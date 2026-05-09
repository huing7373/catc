// WebSocketClientImplTests.swift
// Story 12.2 AC8：WebSocketClientImpl 单元测试（≥5 case + 推荐 case#6 / case#7）.
//
// 测试栈：XCTest only + 手写 fake task handle（通过 @testable import PetApp 拿 internal protocols）.
// 不引 ViewInspector / SnapshotTesting / Mockingbird（与 ADR-0002 §3.1 钦定一致）.
//
// 测试 hook 模式：URLSessionWebSocketTask 不可子类化（init NS_UNAVAILABLE + send/receive 在 extension 内
// 不可 override），因此 WebSocketClientImpl 内引入 internal protocol `WebSocketTaskHandle` + factory
// `WebSocketTaskFactory` —— 测试通过 internal init 注入 FakeWebSocketTaskFactory 接管.
//
// 覆盖 case：
//   #1 happy: connect URL 构造正确（http→ws + path 拼接 + token URL-encode）
//   #2 happy: connect 后 incoming text frame → AsyncStream yield WSMessage（room.snapshot）
//   #3 happy: send(.ping) → underlying task.send 收到 V1 §12.2 ping 信封
//   #4 edge: tokenProvider() 返回 nil → connect 抛 WSError.tokenMissing
//   #5 edge: 未识别 type → AsyncStream yield .unknown(rawType:) + 不破坏 stream
//   #6 (推荐): prepareForReconnect 后 messages 是 fresh stream
//   #7 (推荐): disconnect 后 send 抛 WSError.notConnected

import XCTest
@testable import PetApp

@MainActor
final class WebSocketClientImplTests: XCTestCase {

    // MARK: - case#1: connect URL 构造

    func test_connect_buildsCorrectWSURL_withSchemeConversionAndTokenEncoding() async throws {
        // baseURL "http://localhost:8080"，roomId "1234567"，token "abc+def" (含 +，应被 URL-encode 为 %2B)
        let factory = FakeWebSocketTaskFactory()
        // fix-review round 1 P1：connect() 现在 await first frame —— 注入 minimal snapshot 让 connect 解 latch.
        factory.fakeTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "abc+def" },
            taskFactory: factory
        )

        try await client.connect(roomId: "1234567")

        let request = try XCTUnwrap(factory.lastRequest)
        let urlString = try XCTUnwrap(request.url?.absoluteString)
        XCTAssertEqual(urlString, "ws://localhost:8080/ws/rooms/1234567?token=abc%2Bdef",
                       "URL must convert http→ws + append /ws/rooms/{roomId} + URL-encode token")
        XCTAssertEqual(factory.fakeTask.resumeCallCount, 1, "task.resume() should be called once")
    }

    func test_connect_buildsWSSURL_whenBaseURLIsHTTPS() async throws {
        // https → wss
        let factory = FakeWebSocketTaskFactory()
        // fix-review round 1 P1：connect() 现在 await first frame —— 注入 minimal snapshot 让 connect 解 latch.
        factory.fakeTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]

        let client = WebSocketClientImpl(
            baseURL: URL(string: "https://example.com")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )

        try await client.connect(roomId: "RM01")

        let urlString = try XCTUnwrap(factory.lastRequest?.url?.absoluteString)
        XCTAssertEqual(urlString, "wss://example.com/ws/rooms/RM01?token=tok")
    }

    /// fix-review round 1 P1：通用辅助 —— "握手成功" 信号最小 room.snapshot frame.
    /// 测试场景中只要不关心具体 payload 字段，注入此帧让 connect() latch 解开即可.
    private static let minimalSnapshotJSON = """
    {
      "type": "room.snapshot",
      "requestId": "",
      "payload": {
        "room": {"id": "RM01", "maxMembers": 4, "memberCount": 0},
        "members": []
      },
      "ts": 1
    }
    """

    // MARK: - case#2: incoming text frame → WSMessage

    func test_connect_incomingTextFrame_yieldsRoomSnapshotMessage() async throws {
        let factory = FakeWebSocketTaskFactory()
        // 准备 room.snapshot frame
        let snapshotJSON = """
        {
          "type": "room.snapshot",
          "requestId": "",
          "payload": {
            "room": {"id": "1234567", "maxMembers": 4, "memberCount": 1},
            "members": [
              {"userId": "u1", "nickname": "Alice", "pet": {"petId": "p1", "currentState": 1}}
            ]
          },
          "ts": 1234567890
        }
        """
        factory.fakeTask.scriptedFrames = [.string(snapshotJSON)]

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        try await client.connect(roomId: "1234567")

        // 拿到 messages stream，await 第一条消息
        let stream = client.messages
        let message = try await firstMessage(from: stream, timeout: 2.0)

        guard case .roomSnapshot(let payload) = message else {
            XCTFail("Expected .roomSnapshot, got \(message)")
            return
        }
        XCTAssertEqual(payload.room.id, "1234567")
        XCTAssertEqual(payload.room.maxMembers, 4)
        XCTAssertEqual(payload.members.count, 1)
        XCTAssertEqual(payload.members[0].userId, "u1")
        XCTAssertEqual(payload.members[0].pet?.petId, "p1")
    }

    // MARK: - case#3: send(.ping) → V1 §12.2 ping 信封

    func test_send_ping_writesV1Section122PingEnvelopeToUnderlyingTask() async throws {
        let factory = FakeWebSocketTaskFactory()
        // fix-review round 1 P1：connect() 需要 first frame 解 latch；之后 blockReceiveForever 让 send 路径独立测.
        factory.fakeTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        factory.fakeTask.blockReceiveForever = true

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        try await client.connect(roomId: "RM01")
        try await client.send(.ping(requestId: "ping_001"))

        XCTAssertEqual(factory.fakeTask.sentMessages.count, 1, "task.send 应被调用一次")
        guard case .string(let text) = factory.fakeTask.sentMessages.first else {
            XCTFail("Expected .string frame, got \(String(describing: factory.fakeTask.sentMessages.first))")
            return
        }
        // 解 JSON 验证字段
        let data = try XCTUnwrap(text.data(using: .utf8))
        let json = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
        XCTAssertEqual(json["type"] as? String, "ping")
        XCTAssertEqual(json["requestId"] as? String, "ping_001")
        // payload 应为空对象 {}
        let payload = try XCTUnwrap(json["payload"] as? [String: Any])
        XCTAssertEqual(payload.count, 0, "ping payload 应为空对象")
    }

    // MARK: - case#4: tokenProvider nil → WSError.tokenMissing

    func test_connect_throwsTokenMissing_whenTokenProviderReturnsNil() async {
        let factory = FakeWebSocketTaskFactory()
        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { nil },
            taskFactory: factory
        )

        do {
            try await client.connect(roomId: "RM01")
            XCTFail("Expected WSError.tokenMissing")
        } catch let err as WSError {
            XCTAssertEqual(err, .tokenMissing)
        } catch {
            XCTFail("Expected WSError, got \(error)")
        }

        // 不应发起 underlying task
        XCTAssertNil(factory.lastRequest, "tokenMissing 早退；不应调 makeTask(with:)")
        XCTAssertEqual(factory.fakeTask.resumeCallCount, 0)
    }

    func test_connect_throwsTokenMissing_whenTokenProviderReturnsEmptyString() async {
        let factory = FakeWebSocketTaskFactory()
        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "" },
            taskFactory: factory
        )

        do {
            try await client.connect(roomId: "RM01")
            XCTFail("Expected WSError.tokenMissing")
        } catch let err as WSError {
            XCTAssertEqual(err, .tokenMissing)
        } catch {
            XCTFail("Expected WSError, got \(error)")
        }
    }

    // MARK: - case#5: 未识别 type → .unknown 不破坏 stream

    func test_unknownType_yieldsUnknownCase_streamRemainsAlive() async throws {
        let factory = FakeWebSocketTaskFactory()
        let unknownJSON = """
        {"type": "foo.bar", "requestId": "", "payload": {}}
        """
        let snapshotJSON = """
        {
          "type": "room.snapshot",
          "requestId": "",
          "payload": {
            "room": {"id": "RM01", "maxMembers": 4, "memberCount": 0},
            "members": []
          },
          "ts": 1
        }
        """
        factory.fakeTask.scriptedFrames = [.string(unknownJSON), .string(snapshotJSON)]

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        try await client.connect(roomId: "RM01")

        var collected: [WSMessage] = []
        let collectExpectation = expectation(description: "collect 2 messages")
        let stream = client.messages
        let collectTask = Task {
            for await msg in stream {
                collected.append(msg)
                if collected.count == 2 {
                    collectExpectation.fulfill()
                    return
                }
            }
        }
        await fulfillment(of: [collectExpectation], timeout: 3.0)
        collectTask.cancel()

        XCTAssertEqual(collected.count, 2, "stream 应仍 alive，收到 2 条消息")
        guard case .unknown(let raw) = collected[0] else {
            XCTFail("Expected .unknown, got \(collected[0])")
            return
        }
        XCTAssertEqual(raw, "foo.bar")
        guard case .roomSnapshot = collected[1] else {
            XCTFail("Expected .roomSnapshot as 2nd message, got \(collected[1])")
            return
        }
    }

    // MARK: - case#6: prepareForReconnect → fresh stream

    func test_prepareForReconnect_swapsToFreshStream() async throws {
        let factory = FakeWebSocketTaskFactory()
        // fix-review round 1 P1：first frame 解 connect() latch；之后 blockReceiveForever 保活 stream.
        factory.fakeTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        factory.fakeTask.blockReceiveForever = true

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        try await client.connect(roomId: "RM01")
        let firstStream = client.messages

        // prepareForReconnect 后旧 stream 应 finish；新 stream 是不同 instance
        client.prepareForReconnect()

        // 检查旧 stream 已 finish（for-await 应立即退出）
        let oldStreamFinishedExpectation = expectation(description: "old stream finished")
        let oldStreamTask = Task {
            for await _ in firstStream {
                // 排空（应没有元素）
            }
            oldStreamFinishedExpectation.fulfill()
        }
        await fulfillment(of: [oldStreamFinishedExpectation], timeout: 2.0)
        oldStreamTask.cancel()

        // 新 stream 应可用（重新读 messages）
        let newStream = client.messages
        // 由于 underlyingTask 已 cancel，新 stream 暂无消息（caller 必须再 connect 才有）
        // 这里只验证可以拿到一个 stream（不 finish）
        let newStreamCheckExpectation = expectation(description: "new stream is alive (no quick finish)")
        newStreamCheckExpectation.isInverted = true  // 期望 1 秒内不 finish
        let newStreamTask = Task {
            for await _ in newStream {
                // 不应有消息
            }
            newStreamCheckExpectation.fulfill()  // finish 即失败
        }
        await fulfillment(of: [newStreamCheckExpectation], timeout: 1.0)
        newStreamTask.cancel()
    }

    // MARK: - case#7: disconnect 后 send 抛 WSError.notConnected

    func test_send_throwsNotConnected_afterDisconnect() async throws {
        let factory = FakeWebSocketTaskFactory()
        // fix-review round 1 P1：first frame 解 connect() latch；之后 blockReceiveForever 保 task running 直到 disconnect.
        factory.fakeTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        factory.fakeTask.blockReceiveForever = true

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        try await client.connect(roomId: "RM01")
        client.disconnect()

        do {
            try await client.send(.ping(requestId: "ping_after_disconnect"))
            XCTFail("Expected WSError.notConnected")
        } catch let err as WSError {
            XCTAssertEqual(err, .notConnected)
        } catch {
            XCTFail("Expected WSError, got \(error)")
        }
    }

    // MARK: - case#8 (fix-review round 1 P1): connect 在 handshake 失败时抛 WSError.connectionFailed

    /// 模拟 server / 网络在握手期就 reject —— receive() 第一次就 throw（无 first frame）.
    /// 期望 connect() 抛 WSError.connectionFailed 而非 silently 返回 success.
    func test_connect_throwsConnectionFailed_whenFirstReceiveErrorsBeforeAnyFrame() async {
        let factory = FakeWebSocketTaskFactory()
        // 不注入 scriptedFrames + 不开 blockReceiveForever
        // → FakeWebSocketTaskHandle.receive() 在 100ms 后抛 URLError(.cancelled) (模拟握手期失败)
        factory.fakeTask.scriptedFrames = []
        factory.fakeTask.blockReceiveForever = false

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )

        do {
            try await client.connect(roomId: "RM01")
            XCTFail("Expected connect() to throw WSError.connectionFailed when first receive errors before any frame")
        } catch let err as WSError {
            guard case .connectionFailed = err else {
                XCTFail("Expected WSError.connectionFailed, got \(err)")
                return
            }
            // OK
        } catch {
            XCTFail("Expected WSError.connectionFailed, got \(error)")
        }
    }

    /// 第一帧成功后 receive 错 —— connect() 应已 return；caller 通过 messages stream finish 感知后续断开.
    func test_connect_succeeds_afterFirstFrame_evenIfLaterReceiveErrors() async throws {
        let factory = FakeWebSocketTaskFactory()
        // 一帧 snapshot 后耗尽 → receive 抛 cancelled（"建连后断"路径）
        factory.fakeTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        factory.fakeTask.blockReceiveForever = false

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )

        // connect() 应正常 return（first frame received）
        try await client.connect(roomId: "RM01")

        // stream 应能拿到 first frame，后续 finish
        var collected: [WSMessage] = []
        let streamFinishedExp = expectation(description: "stream finishes after receive error")
        let stream = client.messages
        let streamTask = Task {
            for await msg in stream {
                collected.append(msg)
            }
            streamFinishedExp.fulfill()
        }
        await fulfillment(of: [streamFinishedExp], timeout: 3.0)
        streamTask.cancel()

        XCTAssertEqual(collected.count, 1, "应收到 first frame（snapshot），之后 stream finish")
        guard case .roomSnapshot = collected.first else {
            XCTFail("Expected first message to be .roomSnapshot, got \(String(describing: collected.first))")
            return
        }
    }

    // MARK: - 辅助：从 AsyncStream 拿第一条消息（带超时）

    private func firstMessage<T: Sendable>(
        from stream: AsyncStream<T>,
        timeout: TimeInterval
    ) async throws -> T {
        try await withThrowingTaskGroup(of: T.self) { group in
            group.addTask {
                for await msg in stream {
                    return msg
                }
                throw FirstMessageError.streamFinishedBeforeMessage
            }
            group.addTask {
                try await Task.sleep(nanoseconds: UInt64(timeout * 1_000_000_000))
                throw FirstMessageError.timeout
            }
            let first = try await group.next()
            group.cancelAll()
            return try XCTUnwrap(first, "task group returned nil")
        }
    }

    private enum FirstMessageError: Error {
        case streamFinishedBeforeMessage
        case timeout
    }
}

// MARK: - Fake task factory + handle

/// 手写 fake factory：仅返回预先 wired 的 fakeTask；记录 lastRequest 让单测断言 URL.
final class FakeWebSocketTaskFactory: WebSocketTaskFactory, @unchecked Sendable {
    var lastRequest: URLRequest?
    let fakeTask: FakeWebSocketTaskHandle = FakeWebSocketTaskHandle()

    func makeTask(with request: URLRequest) -> WebSocketTaskHandle {
        self.lastRequest = request
        return fakeTask
    }
}

/// 手写 fake task handle：scriptedFrames 控制 receive 返回；blockReceiveForever 阻塞；sentMessages 记录 send.
final class FakeWebSocketTaskHandle: WebSocketTaskHandle, @unchecked Sendable {
    var resumeCallCount: Int = 0
    var cancelCallCount: Int = 0
    var lastCancelCloseCode: URLSessionWebSocketTask.CloseCode?
    var sentMessages: [URLSessionWebSocketTask.Message] = []
    var scriptedFrames: [URLSessionWebSocketTask.Message] = []
    var blockReceiveForever: Bool = false

    private var receiveIndex: Int = 0
    private var isCancelled: Bool = false
    private let lock = NSLock()

    var isRunning: Bool {
        lock.lock(); defer { lock.unlock() }
        return !isCancelled
    }

    func resume() {
        lock.lock()
        resumeCallCount += 1
        lock.unlock()
    }

    func cancel(with closeCode: URLSessionWebSocketTask.CloseCode, reason: Data?) {
        lock.lock()
        cancelCallCount += 1
        lastCancelCloseCode = closeCode
        isCancelled = true
        lock.unlock()
    }

    func send(_ message: URLSessionWebSocketTask.Message) async throws {
        lock.lock()
        sentMessages.append(message)
        lock.unlock()
    }

    func receive() async throws -> URLSessionWebSocketTask.Message {
        // fix-review round 1 P1：blockReceiveForever 改为"耗尽 scriptedFrames 后才 block"，
        // 这样测试可以先注入 first frame 让 connect() handshake latch 解开，再阻塞后续 receive
        // 用于验证 send / disconnect / prepareForReconnect 路径.
        lock.lock()
        let idx = receiveIndex
        let frames = scriptedFrames
        let cancelled = isCancelled
        let blockAfter = blockReceiveForever
        if idx < frames.count {
            receiveIndex += 1
        }
        lock.unlock()

        if cancelled {
            throw URLError(.cancelled)
        }
        if idx < frames.count {
            return frames[idx]
        }
        if blockAfter {
            // fix-review round 1 P1：耗尽后通过短间隔轮询 isCancelled / Task.isCancelled 实现快速感应,
            // 避免一次 60s Task.sleep 在某些 sim/runtime 配置下 cancellation 传播过慢导致测试超时.
            while !Task.isCancelled {
                lock.lock()
                let nowCancelled = isCancelled
                lock.unlock()
                if nowCancelled { break }
                try await Task.sleep(nanoseconds: 20_000_000)  // 20ms tick
            }
            throw URLError(.cancelled)
        }
        // 耗尽后等一帧时间然后抛错（让 receive loop 自然退出）
        try await Task.sleep(nanoseconds: 100_000_000)  // 100ms
        throw URLError(.cancelled)
    }
}

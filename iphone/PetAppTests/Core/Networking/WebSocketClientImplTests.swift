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

        // 拿到 messages stream，await 第一条非 connectionStateChanged 消息
        // Story 12.5：first frame 触发时 receive loop 先 emit .connectionStateChanged(.connected) 再 yield snapshot；
        // 本 case 验证 server-side payload 解码，因此跳过 .connectionStateChanged 事件取真实 payload.
        let stream = client.messages
        let message = try await firstNonConnectionState(from: stream, timeout: 2.0)

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

        // Story 12.5：first frame 触发时 receive loop 先 emit .connectionStateChanged(.connected) 再 yield 第一帧；
        // 本 case 验证 server-side payload，因此过滤掉 .connectionStateChanged 事件后看 server-side 顺序.
        var collected: [WSMessage] = []
        let collectExpectation = expectation(description: "collect 2 server-side messages")
        let stream = client.messages
        let collectTask = Task {
            for await msg in stream {
                if case .connectionStateChanged = msg { continue }  // skip Story 12.5 emit
                collected.append(msg)
                if collected.count == 2 {
                    collectExpectation.fulfill()
                    return
                }
            }
        }
        await fulfillment(of: [collectExpectation], timeout: 3.0)
        collectTask.cancel()

        XCTAssertEqual(collected.count, 2, "stream 应仍 alive，收到 2 条 server-side 消息（不含 connectionStateChanged）")
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

    /// Story 12.5 后：第一帧成功后 receive 错 + closeCode 是 terminal（如 1000）→ stream finish.
    /// 节点说明：原 Story 12.2 测试用 .invalid（1006）默认 closeCode，pre-12.5 走 finish；12.5 后 1006 是
    /// transient 应触发 reconnect。本 case 显式注入 1000 (.normalClosure) 走 terminal 路径与原意图一致.
    func test_connect_succeeds_afterFirstFrame_evenIfLaterReceiveErrors() async throws {
        let factory = FakeWebSocketTaskFactory()
        // 一帧 snapshot 后耗尽 → receive 抛 cancelled，closeCode=1000 (terminal)
        factory.fakeTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        factory.fakeTask.blockReceiveForever = false
        factory.fakeTask.stubbedCloseCode = .normalClosure  // 1000 = terminal

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )

        // connect() 应正常 return（first frame received）
        try await client.connect(roomId: "RM01")

        // stream 应能拿到 first frame + .connected + .disconnected，后续 finish
        var collected: [WSMessage] = []
        let streamFinishedExp = expectation(description: "stream finishes after receive error (terminal close)")
        let stream = client.messages
        let streamTask = Task {
            for await msg in stream {
                collected.append(msg)
            }
            streamFinishedExp.fulfill()
        }
        await fulfillment(of: [streamFinishedExp], timeout: 3.0)
        streamTask.cancel()

        // Story 12.5：stream 上有 .connected (first frame) + snapshot + .disconnected (terminal close) → finish
        let serverSideMessages = collected.filter {
            if case .connectionStateChanged = $0 { return false }
            return true
        }
        XCTAssertEqual(serverSideMessages.count, 1, "应收到 1 条 server-side 消息（snapshot）")
        guard case .roomSnapshot = serverSideMessages.first else {
            XCTFail("Expected first server-side message to be .roomSnapshot, got \(String(describing: serverSideMessages.first))")
            return
        }
        // 验证 connection state 序列：.connected → .disconnected
        let connStates: [WSConnectionState] = collected.compactMap {
            if case .connectionStateChanged(let s) = $0 { return s }
            return nil
        }
        XCTAssertEqual(connStates.first, .connected)
        XCTAssertEqual(connStates.last, .disconnected, "terminal close → emit .disconnected then finish")
    }

    // MARK: - Story 12.5 reconnect 状态机测试

    /// case#R1 happy: transient close 4005 → schedule reconnect + emit `.reconnecting(attempt: 1)`.
    ///
    /// 时序：fake task receive() 第一帧 snapshot 解 latch → connect() 成功 → emit .connected →
    /// 第二次 receive() 抛错 + closeCode=4005 → reconnect 状态机 emit .reconnecting(attempt: 1) →
    /// schedule reconnect → 第二个 fake task 注入 → reconnect 成功 → emit .connected.
    func test_reconnect_transientClose4005_emitsReconnectingThenReconnects() async throws {
        let factory = FakeReconnectFactory()
        // 第一个 task：发 snapshot + 抛 4005
        let firstTask = factory.scheduleNewTask()
        firstTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        firstTask.errorAfterFrames = true
        firstTask.stubbedCloseCode = .init(rawValue: 4005)!
        // 第二个 task（reconnect attempt 1）：发 snapshot 即可
        let secondTask = factory.scheduleNewTask()
        secondTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        secondTask.blockReceiveForever = true

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        // 用 ms 级 backoff 避免单测真跑 1s
        client.backoffSequence = [0.05, 0.05, 0.05, 0.05, 0.05]

        try await client.connect(roomId: "RM01")

        // 收集 stream 上的 connection state events（≥3：.connected, .reconnecting(1), .connected）
        let states = try await collectConnectionStates(client: client, count: 3, timeout: 3.0)
        XCTAssertEqual(states.count, 3)
        XCTAssertEqual(states[0], .connected, "首次 connect 后 emit .connected")
        if case .reconnecting(let attempt) = states[1] {
            XCTAssertEqual(attempt, 1, "transient close → emit .reconnecting(attempt: 1)")
        } else {
            XCTFail("Expected .reconnecting, got \(states[1])")
        }
        XCTAssertEqual(states[2], .connected, "reconnect 成功后 emit .connected")

        // verify factory 真的发了第二个 task（reconnect 触发）
        XCTAssertEqual(factory.makeTaskCallCount, 2, "应触发 reconnect → 第二次 makeTask")

        client.disconnect()
    }

    /// case#R2 happy: transient close 1006 (.invalid) → schedule reconnect.
    /// 同 R1，但 closeCode 是 .invalid（rawValue=0）—— 验证 V1 §12.1 行 1729 钦定的 1006 客户端合成等价语义.
    func test_reconnect_transientClose1006Invalid_schedulesReconnect() async throws {
        let factory = FakeReconnectFactory()
        let firstTask = factory.scheduleNewTask()
        firstTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        firstTask.errorAfterFrames = true
        firstTask.stubbedCloseCode = .invalid  // rawValue=0 ↔ 1006

        let secondTask = factory.scheduleNewTask()
        secondTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        secondTask.blockReceiveForever = true

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        client.backoffSequence = [0.05, 0.05, 0.05, 0.05, 0.05]

        try await client.connect(roomId: "RM01")
        let states = try await collectConnectionStates(client: client, count: 3, timeout: 3.0)
        XCTAssertEqual(states[0], .connected)
        if case .reconnecting = states[1] {} else {
            XCTFail("Expected .reconnecting for closeCode=.invalid (1006), got \(states[1])")
        }
        XCTAssertEqual(states[2], .connected)
        client.disconnect()
    }

    /// case#R3 terminal: close 4001 → emit .disconnected + finish stream（不重连）.
    func test_reconnect_terminalClose4001_emitsDisconnectedFinishesStream() async throws {
        let factory = FakeReconnectFactory()
        let firstTask = factory.scheduleNewTask()
        firstTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        firstTask.errorAfterFrames = true
        firstTask.stubbedCloseCode = .init(rawValue: 4001)!

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        client.backoffSequence = [0.05, 0.05, 0.05, 0.05, 0.05]

        try await client.connect(roomId: "RM01")
        let states = try await collectConnectionStatesUntilFinish(
            client: client,
            timeout: 3.0
        )
        XCTAssertEqual(states.first, .connected)
        XCTAssertEqual(states.last, .disconnected, "terminal close → 最终 emit .disconnected")
        // 关键：不应触发 reconnect
        XCTAssertEqual(factory.makeTaskCallCount, 1, "terminal close 不应 schedule 第二次 makeTask")
    }

    /// case#R4: 多 terminal close code → 行为一致.
    /// 参数化覆盖 4002 / 4003 / 4004 / 4006 / 4007 / 1000 都走 terminal 路径.
    func test_reconnect_multipleTerminalCloseCodes_allFinishStream() async throws {
        let codes: [Int] = [4002, 4003, 4004, 4006, 4007, 1000]
        for code in codes {
            let factory = FakeReconnectFactory()
            let firstTask = factory.scheduleNewTask()
            firstTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
            firstTask.errorAfterFrames = true
            firstTask.stubbedCloseCode = .init(rawValue: code)!

            let client = WebSocketClientImpl(
                baseURL: URL(string: "http://localhost:8080")!,
                tokenProvider: { "tok" },
                taskFactory: factory
            )
            client.backoffSequence = [0.05, 0.05, 0.05, 0.05, 0.05]

            try await client.connect(roomId: "RM01")
            let states = try await collectConnectionStatesUntilFinish(client: client, timeout: 3.0)
            XCTAssertEqual(states.last, .disconnected,
                           "close code \(code) 必须按 terminal 处理 (最后一条状态应为 .disconnected)")
            XCTAssertEqual(factory.makeTaskCallCount, 1,
                           "close code \(code) 不应 reconnect")
        }
    }

    /// case#R5 happy: 5 次 reconnect 连续失败 → 最终 emit .disconnected + finish stream.
    /// 用短 backoff 跑得快；每次 reconnect 都在第一帧失败.
    func test_reconnect_fiveAttemptsFail_finallyDisconnected() async throws {
        let factory = FakeReconnectFactory()
        // 第一个 task：发 snapshot 解 latch → 立即抛 4005 transient
        let firstTask = factory.scheduleNewTask()
        firstTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        firstTask.errorAfterFrames = true
        firstTask.stubbedCloseCode = .init(rawValue: 4005)!
        // reconnect attempts 1..5：在 receive() 第一次就抛错（pre-handshake）
        for _ in 0..<5 {
            let t = factory.scheduleNewTask()
            t.errorAfterFrames = true
            t.scriptedFrames = []  // 没有 first frame → receive 立即抛错
            t.stubbedCloseCode = .invalid
        }

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        client.backoffSequence = [0.02, 0.02, 0.02, 0.02, 0.02]

        try await client.connect(roomId: "RM01")
        let states = try await collectConnectionStatesUntilFinish(client: client, timeout: 5.0)
        // states: [.connected, .reconnecting(1), .reconnecting(2), .reconnecting(3), .reconnecting(4), .reconnecting(5), .disconnected]
        XCTAssertEqual(states.first, .connected)
        XCTAssertEqual(states.last, .disconnected, "5 次失败后最终 .disconnected")
        let reconnectingCount = states.filter { state in
            if case .reconnecting = state { return true }
            return false
        }.count
        XCTAssertEqual(reconnectingCount, 5, "应 emit 5 次 .reconnecting (attempt 1..5)")
    }

    /// case#R6 happy: disconnect() 必须 cancel in-flight reconnect task.
    /// 测试核心：transient close → schedule reconnect → 在 reconnect attempt 真跑前 disconnect →
    /// 不应再调 makeTask（reconnect task 被 cancel）+ stream 应被 finish.
    func test_reconnect_disconnectCancelsInflightReconnectTask() async throws {
        let factory = FakeReconnectFactory()
        let firstTask = factory.scheduleNewTask()
        firstTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        firstTask.errorAfterFrames = true
        firstTask.stubbedCloseCode = .init(rawValue: 4005)!

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        // 用较长 backoff 让我们有时间在 reconnect attempt 真正发起前 disconnect
        client.backoffSequence = [1.0, 1.0, 1.0, 1.0, 1.0]

        try await client.connect(roomId: "RM01")
        // 等收到 .reconnecting(1)（说明 schedule 已下发但 backoff 还在 sleep）
        var seenReconnecting = false
        let stream = client.messages
        let waitTask = Task {
            for await msg in stream {
                if case .connectionStateChanged(.reconnecting) = msg {
                    seenReconnecting = true
                    return
                }
            }
        }
        // 等 2s 给 schedule 上传 .reconnecting；同时 backoff sleep 还远未结束.
        let timeoutDeadline = Date().addingTimeInterval(2.0)
        while !seenReconnecting && Date() < timeoutDeadline {
            try await Task.sleep(nanoseconds: 50_000_000)
        }
        waitTask.cancel()
        XCTAssertTrue(seenReconnecting, "应 emit .reconnecting(attempt: 1)（schedule 之后，attempt 真跑之前）")

        // disconnect → cancel reconnectTask
        client.disconnect()

        // 等 1.5s 给 reconnect task 充分被 cancel 的时间
        try await Task.sleep(nanoseconds: 1_500_000_000)
        XCTAssertEqual(factory.makeTaskCallCount, 1,
                       "disconnect 应 cancel reconnect task → 不应触发第二次 makeTask")
    }

    /// case#R7 happy: 未知 close code（如 4099）→ 保守 terminal（不重连）.
    func test_reconnect_unknownCloseCode_treatedAsTerminal() async throws {
        let factory = FakeReconnectFactory()
        let firstTask = factory.scheduleNewTask()
        firstTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        firstTask.errorAfterFrames = true
        firstTask.stubbedCloseCode = .init(rawValue: 4099)!  // 未在 V1 §12.1 close code 表中

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        client.backoffSequence = [0.05, 0.05, 0.05, 0.05, 0.05]

        try await client.connect(roomId: "RM01")
        let states = try await collectConnectionStatesUntilFinish(client: client, timeout: 3.0)
        XCTAssertEqual(states.last, .disconnected, "未知 close code 4099 必须保守 terminal")
        XCTAssertEqual(factory.makeTaskCallCount, 1, "未知 close code 不应 reconnect (防野生 server bug 死循环)")
    }

    /// case#R8 fix-review round 2 P1：reconnect 期间 **pre-handshake** terminal close（如 4001）→
    /// 立即 emit .disconnected + finish stream + **不**继续 retry.
    ///
    /// 触发场景：第一次连接后 transient close 4005 → schedule reconnect → reconnect attempt 1 在 first frame
    /// 之前被 server reject（4001 token 过期）—— 旧实装 attemptReconnect catch 无条件 schedule next attempt,
    /// 白白消耗 5 次 backoff，永远不触发 caller 的 re-auth handling path.
    /// 修复后：reconnect catch 按 close code 分类 → 4001 立即 terminal stop.
    func test_reconnect_terminalCloseDuringHandshake_stopsRetrying() async throws {
        let factory = FakeReconnectFactory()
        // 第一个 task：成功握手 → 发 snapshot → transient close 4005 触发 reconnect
        let firstTask = factory.scheduleNewTask()
        firstTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        firstTask.errorAfterFrames = true
        firstTask.stubbedCloseCode = .init(rawValue: 4005)!
        // reconnect attempt 1：**pre-handshake** 失败 + closeCode 4001（terminal）→ 应停 retry
        let reconnectTask = factory.scheduleNewTask()
        reconnectTask.scriptedFrames = []  // 没有 first frame
        reconnectTask.errorAfterFrames = true
        reconnectTask.stubbedCloseCode = .init(rawValue: 4001)!  // token 过期 = terminal

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        client.backoffSequence = [0.02, 0.02, 0.02, 0.02, 0.02]

        try await client.connect(roomId: "RM01")
        let states = try await collectConnectionStatesUntilFinish(client: client, timeout: 3.0)

        // 关键断言 1：states 里只有 1 次 .reconnecting（不是 5 次）
        let reconnectingCount = states.filter { state in
            if case .reconnecting = state { return true }
            return false
        }.count
        XCTAssertEqual(reconnectingCount, 1,
                       "reconnect 期间 pre-handshake 4001（terminal）→ 应只 emit 1 次 .reconnecting，不应继续 retry 5 次")

        // 关键断言 2：最后状态是 .disconnected
        XCTAssertEqual(states.last, .disconnected,
                       "reconnect pre-handshake terminal close → 最终 emit .disconnected")

        // 关键断言 3：只发起了 2 次 makeTask（第一次连接 + 第一次 reconnect）
        XCTAssertEqual(factory.makeTaskCallCount, 2,
                       "reconnect 期间 pre-handshake terminal close 不应继续触发后续 makeTask（旧实装会跑到 5 次）")
    }

    /// case#R9 fix-review round 2 P1：reconnect 期间 pre-handshake **transient** close（如 4005）→
    /// 仍 schedule 下一次 retry（与 R8 终态对照确保不误伤 transient 路径）.
    func test_reconnect_transientCloseDuringHandshake_continuesRetrying() async throws {
        let factory = FakeReconnectFactory()
        // 第一个 task：握手 → snapshot → transient close 4005
        let firstTask = factory.scheduleNewTask()
        firstTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        firstTask.errorAfterFrames = true
        firstTask.stubbedCloseCode = .init(rawValue: 4005)!
        // reconnect attempts 1..4：pre-handshake 失败 + transient closeCode 4005
        for _ in 0..<4 {
            let t = factory.scheduleNewTask()
            t.scriptedFrames = []
            t.errorAfterFrames = true
            t.stubbedCloseCode = .init(rawValue: 4005)!
        }
        // reconnect attempt 5：成功（snapshot OK）—— 验证 5 次内的 transient retry 路径不被误伤
        let finalTask = factory.scheduleNewTask()
        finalTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        client.backoffSequence = [0.02, 0.02, 0.02, 0.02, 0.02]

        try await client.connect(roomId: "RM01")
        // 等待第二次 .connected（reconnect attempt 5 成功）
        let states = try await collectConnectionStates(client: client, count: 7, timeout: 5.0)
        // [.connected, .reconnecting(1), .reconnecting(2), .reconnecting(3), .reconnecting(4), .reconnecting(5), .connected]
        XCTAssertEqual(states.first, .connected)
        XCTAssertEqual(states.last, .connected, "transient retry 5 次内成功 → 最终 .connected")
        XCTAssertEqual(factory.makeTaskCallCount, 6,
                       "transient pre-handshake close → 应继续 retry 直到成功（1 + 5 = 6 次 makeTask）")
    }

    // MARK: - case#R10..R12 fix-review round 2 P1: generation counter race tests

    /// case#R10 fix-review round 2 P1：cancelled reconnect attempt 的 catch path 不再 schedule new retry.
    ///
    /// 触发：
    ///   1. 第一次 connect 成功（snapshot 解 latch）→ transient close 4005 → schedule reconnect (attempt 1)
    ///   2. attempt 1 的 makeTask 启动新 fake handle → 此 fake 的 receive() **永久 block** 模拟"在 connectInternal
    ///      内卡住"
    ///   3. caller 在 backoff sleep + makeTask 后立即 disconnect() —— 这会 cancel 当前 reconnectTask（已经在
    ///      connectInternal 内 await receive()），翻 sessionGeneration += 1
    ///   4. 被 cancel 的 receive() 抛 CancellationError → connectInternal throw → attemptReconnect catch
    ///   5. 旧实装：catch 无 generation check → 把 cancellation 当 transient → schedule next retry → 第三次 makeTask
    ///   6. 修复后：catch 入口看到 sessionGeneration 已被 disconnect 翻动 → silent return → makeTaskCallCount
    ///      停在 2（首次 + reconnect attempt 1）
    func test_reconnect_cancelledAttemptCatchDoesNotScheduleNewRetry() async throws {
        let factory = FakeReconnectFactory()

        // 首次连接：snapshot 解 latch → 立即 transient 4005 触发 reconnect
        let firstTask = factory.scheduleNewTask()
        firstTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        firstTask.errorAfterFrames = true
        firstTask.stubbedCloseCode = .init(rawValue: 4005)!

        // reconnect attempt 1：永久 block 在 receive() —— 模拟"已经在 connectInternal 内"
        let attemptTask = factory.scheduleNewTask()
        attemptTask.scriptedFrames = []
        attemptTask.blockReceiveForever = true

        // 防御兜底：如果旧实装 schedule 了第三次 makeTask（不该发生），让它快速 error
        let bonusTask = factory.scheduleNewTask()
        bonusTask.scriptedFrames = []
        bonusTask.errorAfterFrames = true
        bonusTask.stubbedCloseCode = .invalid

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        // 用极短 backoff 让 attempt 1 的 makeTask 快速发起
        client.backoffSequence = [0.05, 0.05, 0.05, 0.05, 0.05]

        try await client.connect(roomId: "RM01")

        // 等到 attempt 1 的 makeTask 真的发起（即 makeTaskCallCount == 2）
        let attemptStartedDeadline = Date().addingTimeInterval(2.0)
        while factory.makeTaskCallCount < 2, Date() < attemptStartedDeadline {
            try await Task.sleep(nanoseconds: 20_000_000)
        }
        XCTAssertEqual(factory.makeTaskCallCount, 2, "reconnect attempt 1 应该真的发起 makeTask")

        // 此刻 attempt 1 卡在 receive() 永久 block —— disconnect() cancel reconnectTask + 翻 generation
        client.disconnect()

        // 给 cancellation 充分传播 + catch path 跑完的时间
        try await Task.sleep(nanoseconds: 1_500_000_000)

        XCTAssertEqual(factory.makeTaskCallCount, 2,
                       "cancelled attempt 的 catch 在新 generation 下 silent drop → 不应 schedule next retry → makeTaskCallCount 不增")
    }

    /// case#R11 fix-review round 2 P1：prepareForReconnect 后旧 receive-loop 的 catch 不污染新 stream.
    ///
    /// 触发：
    ///   1. 第一次 connect 成功（snapshot 解 latch）—— receive loop 卡在 second receive()
    ///   2. caller 调 prepareForReconnect() —— swap 新 stream + 翻 sessionGeneration
    ///   3. 旧 receive() 因 task.cancel(closeCode:) 抛错 → 走 catch path
    ///   4. 旧实装：catch 用 `currentContinuation`（已是新 stream）emit `.disconnected` / schedule reconnect →
    ///      新 stream 上出现 stale 状态事件
    ///   5. 修复后：catch 看到 sessionGeneration 已被 prepareForReconnect 翻动 → silent return → 新 stream
    ///      不应收到任何 stale connection-state event
    func test_reconnect_staleReceiveLoopCatchDoesNotPolluteFreshStream() async throws {
        let factory = FakeReconnectFactory()

        // 首次连接：snapshot 解 latch + blockReceiveForever（让 receive loop 卡在 second receive）
        let firstTask = factory.scheduleNewTask()
        firstTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        firstTask.blockReceiveForever = true
        // 注：firstTask 的 closeCode 默认 .invalid（rawValue=0）—— 旧实装会把它当 transient → schedule reconnect.
        // 修复后：generation 校验 silent drop.

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        client.backoffSequence = [0.05, 0.05, 0.05, 0.05, 0.05]

        try await client.connect(roomId: "RM01")

        // prepareForReconnect → swap 新 stream + 翻 generation + cancel 旧 task
        client.prepareForReconnect()
        let freshStream = client.messages

        // 收集新 stream 上 1 秒内 emit 的所有 connection-state 事件（应为 0：旧 receive-loop catch silent drop）
        var pollutedStates: [WSConnectionState] = []
        let collectExp = expectation(description: "collect on fresh stream (expecting NO state events)")
        collectExp.isInverted = true  // 期望不被 fulfilled —— 任何 emit 都算污染
        let collectTask = Task {
            for await msg in freshStream {
                if case .connectionStateChanged(let s) = msg {
                    pollutedStates.append(s)
                    collectExp.fulfill()  // emit 即失败
                    return
                }
            }
        }
        await fulfillment(of: [collectExp], timeout: 1.0)
        collectTask.cancel()

        XCTAssertTrue(pollutedStates.isEmpty,
                      "stale receive-loop catch 不应在 fresh stream 上 emit 任何 connection-state；实际 emit: \(pollutedStates)")

        // 二次保险：旧实装会触发第二次 makeTask（schedule reconnect 后 sleep + attemptReconnect）—— 修复后不应
        XCTAssertEqual(factory.makeTaskCallCount, 1,
                       "stale receive-loop catch silent drop → 不应 schedule reconnect → 不应触发第二次 makeTask")
    }

    /// case#R12 fix-review round 2 P1：cancellation 后 fresh connect 不被 stale retry race.
    ///
    /// 触发：
    ///   1. 第一次 connect 成功（snapshot）→ transient close 4005 → schedule reconnect (attempt 1)
    ///   2. attempt 1 的 connectInternal 内卡住（永久 block）
    ///   3. caller disconnect() cancel reconnectTask + 翻 generation
    ///   4. caller 立即 fresh `connect(roomId: "ROOM_B")`（不同 roomId）—— 成功握手
    ///   5. 旧实装：stale attempt 1 catch 跑 → schedule next retry on currentRoomId（旧 roomId 已被 disconnect 清；
    ///      但成功 fresh connect 把 currentRoomId 写回 "ROOM_B"）→ stale retry 复用 ROOM_B 的 currentRoomId →
    ///      makeTask call counts 莫名增加
    ///   6. 修复后：stale catch silent drop；fresh connect 后 makeTaskCallCount == 2 (首次 RM01 + fresh ROOM_B)
    func test_reconnect_freshConnectAfterCancellationNotRacedByStaleRetry() async throws {
        let factory = FakeReconnectFactory()

        // 首次 RM01：snapshot 解 latch + 立即 transient 4005
        let firstTask = factory.scheduleNewTask()
        firstTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        firstTask.errorAfterFrames = true
        firstTask.stubbedCloseCode = .init(rawValue: 4005)!

        // attempt 1（卡住）
        let staleAttemptTask = factory.scheduleNewTask()
        staleAttemptTask.scriptedFrames = []
        staleAttemptTask.blockReceiveForever = true

        // fresh connect ROOM_B：snapshot + block
        let freshTask = factory.scheduleNewTask()
        freshTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        freshTask.blockReceiveForever = true

        // 防御 bonus：stale retry 一旦 schedule 会消耗
        let bonusTask = factory.scheduleNewTask()
        bonusTask.scriptedFrames = []
        bonusTask.errorAfterFrames = true
        bonusTask.stubbedCloseCode = .invalid

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        client.backoffSequence = [0.05, 0.05, 0.05, 0.05, 0.05]

        try await client.connect(roomId: "RM01")

        // 等 attempt 1 makeTask 发起
        let attemptStartedDeadline = Date().addingTimeInterval(2.0)
        while factory.makeTaskCallCount < 2, Date() < attemptStartedDeadline {
            try await Task.sleep(nanoseconds: 20_000_000)
        }
        XCTAssertEqual(factory.makeTaskCallCount, 2)

        // disconnect → cancel attempt 1 + 翻 gen
        client.disconnect()

        // fresh connect ROOM_B（不同 roomId）—— 应直接成功（snapshot 解 latch）
        try await client.connect(roomId: "ROOM_B")

        // fresh connect 后等 2 秒 —— 给 stale catch 充分时间跑（如果还会跑）
        try await Task.sleep(nanoseconds: 2_000_000_000)

        // 关键断言：makeTaskCallCount == 3（RM01 + stale attempt 1 + fresh ROOM_B）
        // 旧实装会 ≥ 4（stale catch 又 schedule next retry，消耗 bonus task）
        XCTAssertEqual(factory.makeTaskCallCount, 3,
                       "fresh connect 后 stale catch 应 silent drop；不应 schedule 后续 retry")

        // fresh connect 的 URL 必须是 ROOM_B（最后一次 makeTask 拿的 request）
        let lastRequest = factory.requests.last
        let urlString = lastRequest?.url?.absoluteString ?? ""
        XCTAssertTrue(urlString.contains("ROOM_B"),
                      "fresh connect 应使用 ROOM_B URL；实际 last URL: \(urlString)")

        client.disconnect()
    }

    /// case#R13 fix-review round 3 P1：stale receive-task defer 的 resolveConnectGate 不污染新 connect 的 gate.
    ///
    /// 触发：
    ///   1. 第一次 connect 成功（snapshot 解 latch）→ transient close 4005 → schedule reconnect (attempt 1)
    ///   2. attempt 1 永久 block 在 receive() —— 模拟 "已经在 connectInternal 内卡住"
    ///   3. caller disconnect() cancel reconnectTask + 翻 generation
    ///   4. caller 立即 fresh `connect(roomId: "ROOM_C")`：fresh fake handle scriptedFrames=空 + blockReceiveForever
    ///      —— 让 fresh connect 阻塞在 first frame 直到 mocked snapshot 出现
    ///   5. 旧 attempt 1 receive task 因为 cancel 抛 URLError(.cancelled) → 走 defer block →
    ///      调 `resolveConnectGate(success: false, ...)`
    ///
    /// 旧实装：resolveConnectGate 没 generation check —— stale defer resolve 新 gate → fresh connect 抛
    ///   `WSError.connectionFailed` ↔ "receive task cancelled before first frame".
    /// 修复后：resolveConnectGate 内部 `mySession != connectGateOwnerSession` → silent drop → fresh
    ///   connect 继续 await（直到我们解开 fresh task 的 first frame）.
    ///
    /// 验证手段：在 stale defer 应当跑完后，我们让 fresh task 的 first frame 出现 —— fresh connect 应
    ///   成功 return（不抛 stale failure）.
    func test_reconnect_staleReceiveTaskDeferDoesNotResolveFreshConnectGate() async throws {
        let factory = FakeReconnectFactory()

        // 首次 RM01：snapshot 解 latch + 立即 transient 4005
        let firstTask = factory.scheduleNewTask()
        firstTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        firstTask.errorAfterFrames = true
        firstTask.stubbedCloseCode = .init(rawValue: 4005)!

        // attempt 1（卡住，永久 block 在 receive；将被 disconnect cancel）
        let staleAttemptTask = factory.scheduleNewTask()
        staleAttemptTask.scriptedFrames = []
        staleAttemptTask.blockReceiveForever = true

        // fresh ROOM_C：scriptedFrames 留空 + blockReceiveForever —— 让 fresh connect 阻塞在 first frame.
        // 测试中我们后续会注入 first frame 让 connect 真正 resolve.
        let freshTask = factory.scheduleNewTask()
        freshTask.scriptedFrames = []
        freshTask.blockReceiveForever = true

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        client.backoffSequence = [0.05, 0.05, 0.05, 0.05, 0.05]

        try await client.connect(roomId: "RM01")

        // 等 attempt 1 makeTask 发起
        let attemptStartedDeadline = Date().addingTimeInterval(2.0)
        while factory.makeTaskCallCount < 2, Date() < attemptStartedDeadline {
            try await Task.sleep(nanoseconds: 20_000_000)
        }
        XCTAssertEqual(factory.makeTaskCallCount, 2)

        // disconnect → cancel attempt 1 + 翻 gen.
        // 关键：disconnect() 自身走 unconditional resolve 路径 fail 当时的 in-flight gate（此时无 in-flight
        // connect，所以 connectGate 是 nil，unconditional resolve no-op）.
        client.disconnect()

        // 立即 fresh connect ROOM_C —— 该 connect 进 connectInternal 后 install 新 gate (owner = new session).
        // stale attempt 1 的 receive-task defer 即将跑（cancel 在 disconnect 内已下达）；
        // 旧实装会 resolve 新 gate → 让本 connect 抛 stale failure.
        // 修复后：stale defer 的 mySession（attempt 1 的 session）与新 gate owner 不匹配 → silent drop.
        //
        // 用 actor-isolated finished flag 跟踪 fresh connect 是否提前 finish（不能用直接 await freshConnectTask.value
        // 因为 fresh task 永久 block 在 receive；测试需要 race timeout 而非 hang）.
        let resultBox = ResultBox()
        let freshConnectTask = Task<Void, Never> {
            do {
                try await client.connect(roomId: "ROOM_C")
                await resultBox.set(.success(()))
            } catch {
                await resultBox.set(.failure(error))
            }
        }

        // 给 stale defer 充分时间跑完（disconnect cancel 信号传播 + receive() throw + defer block + log）.
        // 1.0s 经验值：远大于 cancellation 传播 + Swift Concurrency cooperative yield 的最坏情况.
        try await Task.sleep(nanoseconds: 1_000_000_000)

        // 关键 assertion：fresh connect 不应在 stale defer 跑完后立即 finish（freshTask 永久 block 在 receive,
        // 没有 first frame；唯一能让 connect() return/throw 的路径是 stale defer 错误地 resolve 新 gate）.
        let resultBeforeCleanup = await resultBox.get()

        // cleanup：disconnect 走 unconditional resolve 让 fresh connect throw + freshConnectTask 退出（防泄漏）.
        client.disconnect()
        _ = await freshConnectTask.value

        // 修复后：resultBeforeCleanup == nil（fresh connect 仍在 await）.
        // 旧实装 bug 复现：resultBeforeCleanup == .failure(WSError.connectionFailed("...cancelled before first frame...")).
        if let result = resultBeforeCleanup {
            XCTFail("fresh connect 不应在 stale defer 跑完后 finish；实际: \(result) —— stale resolveConnectGate 污染了新 gate")
        }
    }

    /// case#R13 helper：actor-isolated Result box 让 freshConnectTask 把"是否已 finish + finish 结果"写到此 box,
    /// 主测试 task 通过 `get()` 读取（避免直接 await freshConnectTask.value 阻塞测试 timeout race）.
    private actor ResultBox {
        private var value: Result<Void, Error>?
        func set(_ v: Result<Void, Error>) { self.value = v }
        func get() -> Result<Void, Error>? { self.value }
    }

    // MARK: - case#R14 (fix-review round 4 P2 #1)
    // connect 在已 connected client 上调用复用现 stream → stale receive-task defer 的 finish() 必须 generation-gated.
    //
    // 触发条件：connect(roomId:) 复用现存 currentContinuation（不调 prepareForReconnect），
    //   sessionGeneration += 1 + cancel 旧 receiveTask；旧 receive-loop 落到 defer 跑 continuation.finish().
    // 旧实装：finish() 直写共享 continuation → 新 session 的 stream 立即被 terminate → vm 收不到消息.
    // 修复后：finishStreamIfCurrent 校验 mySession == sessionGeneration → 不匹配 silent skip → 新 stream 仍 alive.

    func test_reconnect_staleReceiveTaskDeferFinishDoesNotTerminateReusedStream() async throws {
        let factory = FakeReconnectFactory()

        // 首个 RM01：snapshot 解 latch + 阻塞后续 receive（让旧 receive-loop 在 connect 复用 path 前一直停在 receive() 内）
        let firstTask = factory.scheduleNewTask()
        firstTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        firstTask.blockReceiveForever = true

        // 第二次 connect 路径：scheduleNewTask 备好；snapshot 解 latch + 持续阻塞（不 finish stream）
        let secondTask = factory.scheduleNewTask()
        secondTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        secondTask.blockReceiveForever = true

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )

        // 拿 stream 引用（vm 视角）—— 关键：跨第二次 connect 不应被 finish.
        let sharedStream = client.messages

        // 1. 首次 connect RM01 成功
        try await client.connect(roomId: "RM01")

        // 2. 立即在已 connected client 上 connect ROOM_B —— 复用现存 currentContinuation；旧 receiveTask 被 cancel.
        //    cancel 信号 + 旧 receive-loop defer 跑 finish() —— 旧实装会终结被 secondTask 复用的同一 stream.
        try await client.connect(roomId: "ROOM_B")

        // 3. 给 stale defer 充分时间跑完（cancel 信号传播 + receive() throw + defer block）.
        try await Task.sleep(nanoseconds: 800_000_000)

        // 4. 关键 assertion：sharedStream 仍 alive —— 用一个 task 收第一条非 connectionState 消息（应该是
        //    第二次 connect 收到的 ROOM_B snapshot；如果 stream 被 stale defer finish，task 会立即拿不到任何消息且 stream 早已结束）.
        let firstNonStateMessage = try await firstNonConnectionState(from: sharedStream, timeout: 1.5)
        // ROOM_B 的 snapshot 解码应是 .roomSnapshot；至少不能是 streamFinishedBeforeMessage（throw）.
        if case .unknown = firstNonStateMessage {
            XCTFail("expected room.snapshot non-unknown message, got .unknown — stale defer 可能污染了消息序列")
        }

        client.disconnect()
    }

    // MARK: - case#R15 (fix-review round 4 P2 #2)
    // connect 在已 connected client 上调用 → stale receive-task 已从 task.receive() 拿到的旧房间 frame
    // 不能在 sessionGeneration 翻新之后 yield 到复用的 stream.
    //
    // 触发条件：connect(roomId:) 翻 sessionGeneration 后，旧 receive-loop 仍可能从 await task.receive()
    //   返回（cancel 信号传播延迟），随即 yield；旧实装无 generation gate → 旧房间 frame 漏到新连接.
    // 测试构造：用 `gateUntilCancelled` 模式让首个 task 的"第二帧 receive() 调用"挂着，直到 cancel 才抛错；
    //   这样 cancel 信号到达时 receive() 返回（throw cancelled）而**不是** dequeue 一帧；
    //   验证 stale receive-loop 的 catch path 不会把任何 frame 漏到 fresh stream.
    // 严格 yield-leak race（receive 已 dequeue 但 yield 未跑 + 翻 gen + yield）窗口极小，
    // 直接构造跨 await 的"已 dequeue 未 yield" 窗口需要 patch fake task 内部信号；
    // 在 R14 已 cover finish path 的修复，本测试用稳定的"第二次 connect 后旧房间无 stale frame 漏出" assertion.

    func test_reconnect_staleReceiveLoopAfterConnectReplaceLeavesNewStreamCleanForNewSnapshot() async throws {
        let factory = FakeReconnectFactory()

        // 首个 RM01：snapshot 解 latch + 之后 block 等 cancel
        let firstTask = factory.scheduleNewTask()
        firstTask.scriptedFrames = [.string(Self.minimalSnapshotJSON)]
        firstTask.blockReceiveForever = true

        // 第二次 ROOM_B：snapshot 解 latch + 之后 block
        let secondTask = factory.scheduleNewTask()
        let roomBSnapshotJSON = """
        {
          "type": "room.snapshot",
          "requestId": "",
          "payload": {
            "room": {"id": "ROOM_B", "maxMembers": 4, "memberCount": 0},
            "members": []
          },
          "ts": 2
        }
        """
        secondTask.scriptedFrames = [.string(roomBSnapshotJSON)]
        secondTask.blockReceiveForever = true

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )

        let sharedStream = client.messages

        // 关键：在 connect 之前就起 collector（unbounded buffer 但避免任何 stream-iteration 异常）.
        let collectTask = Task<[WSMessage], Never> {
            var local: [WSMessage] = []
            for await msg in sharedStream {
                if case .connectionStateChanged = msg { continue }
                local.append(msg)
                // 收够 2 条 non-state 消息（RM01 + ROOM_B 两个 snapshot）就退出，避免 hang
                if local.count >= 2 { return local }
            }
            return local
        }

        try await client.connect(roomId: "RM01")
        // 给 RM01 first frame 的 yield 跑完时间（connect() return 后 receive-loop 仍在 process first frame
        // 路径，yield 在 resolve-gate 之后但 caller 已 wake；不 sleep 直接 chain connect 会与 yield race）.
        try await Task.sleep(nanoseconds: 50_000_000)

        // 立即 connect ROOM_B —— 翻 sessionGeneration + cancel 旧 receiveTask + 复用现存 currentContinuation.
        try await client.connect(roomId: "ROOM_B")

        // 给 stale receive-loop 充分时间走完 cancel + defer + 任何 catch path（不应 yield 任何 stale frame）.
        try await Task.sleep(nanoseconds: 1_000_000_000)
        client.disconnect()
        let collected = await collectTask.value

        // 至少有两条 snapshot：RM01（首次 connect）+ ROOM_B（第二次 connect）.
        // stale leak 表征：在 ROOM_B snapshot **之后**还有 RM01 的消息（roomId == "RM01"）.
        let snapshots: [(roomId: String, idx: Int)] = collected.enumerated().compactMap { idx, msg in
            if case .roomSnapshot(let payload) = msg {
                return (roomId: payload.room.id, idx: idx)
            }
            return nil
        }
        XCTAssertGreaterThanOrEqual(snapshots.count, 2, "expected RM01 + ROOM_B snapshots; collected=\(collected)")
        let lastSnapshot = snapshots.last!
        XCTAssertEqual(lastSnapshot.roomId, "ROOM_B",
            "last snapshot on reused stream should be ROOM_B, not stale RM01 (stale yield leak); collected=\(collected)")
    }

    // MARK: - case#R16 (fix-review round 5 P1)
    // connect(roomId:) 在 token nil/空 或 makeWSURL throw 时**绝不**翻 sessionGeneration ——
    // 否则现存活的 receive-loop 立即被 stale 化（mySession 落后），still-open connection 的所有
    // 后续 frame 走 yieldIfCurrent / emitConnectionStateIfCurrent 全部 silent drop → 用户不可见 wedge.
    //
    // 触发条件：room switch 时 auth 暂时不可用（tokenProvider 返回 nil）—— 切到新 room 失败，
    //   但原 in-room session 必须仍可正常收消息.
    // 测试设计：
    //   1. 用一个动态 tokenProvider —— 首次 connect 返回 valid token；第二次（token-fail 注入）返回 nil.
    //   2. 首次 connect RM01 成功 + 收到 snapshot → 然后注入 nil token + 调 connect("RM02") 期望 throws.
    //   3. 失败 connect 之后再让 firstTask 推一帧（模拟 RM01 still-open 的后续 server push）—— 验证仍能 yield.
    //
    // 旧实装：第二步 connect 入口直接翻 gen → 第三步的 firstTask frame yield 时 mySession 落后 →
    //   silent drop → 测试观察到 stream 上拿不到第二帧 RM01 message → wedge.
    // 修复后：connect 入口先 dry-run 校验 token + URL；token nil → throw 时 gen 未翻 → 第二帧仍 yield.

    func test_connect_tokenNilDoesNotInvalidateLiveSession_round5_P1() async throws {
        let factory = FakeReconnectFactory()

        // RM01：snapshot 解 latch + 之后 blockReceiveForever 保活；测试通过 enqueueFrame 在 connect 失败后再 push.
        let firstTask = factory.scheduleNewTask()
        let rm01Snapshot = """
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
        let rm01PostFailMessage = """
        {
          "type": "member.left",
          "requestId": "",
          "payload": {"userId": "U1"},
          "ts": 2
        }
        """
        // 两帧：snapshot 解 latch；第二帧（heartbeat）会在第二次 connect 失败之后被 receive() 返回.
        firstTask.scriptedFrames = [.string(rm01Snapshot), .string(rm01PostFailMessage)]
        firstTask.blockReceiveForever = true

        // 动态 token：用 NSLock 保护的 toggle.
        let tokenLock = NSLock()
        var tokenValid = true
        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: {
                tokenLock.lock()
                defer { tokenLock.unlock() }
                return tokenValid ? "tok" : nil
            },
            taskFactory: factory
        )

        let stream = client.messages
        let collectTask = Task<[WSMessage], Never> {
            var local: [WSMessage] = []
            for await msg in stream {
                if case .connectionStateChanged = msg { continue }
                local.append(msg)
                if local.count >= 2 { return local }  // snapshot + heartbeat
            }
            return local
        }

        // 1. 首次 connect RM01 成功
        try await client.connect(roomId: "RM01")

        // 2. 注入 nil token + 调 connect("RM02") 期望抛 tokenMissing.
        tokenLock.lock(); tokenValid = false; tokenLock.unlock()
        do {
            try await client.connect(roomId: "RM02")
            XCTFail("Expected WSError.tokenMissing")
        } catch let err as WSError {
            XCTAssertEqual(err, .tokenMissing)
        } catch {
            XCTFail("Expected WSError.tokenMissing, got \(error)")
        }

        // 3. 关键 assertion：firstTask 仍是当前 underlying；其第二帧 (rm01PostFailMessage) 应能正常 yield.
        //    旧实装：connect 入口已翻 gen → mySession 落后 → yieldIfCurrent silent drop → collectTask 拿不到第二帧.
        let collected = await withTaskGroup(of: [WSMessage].self) { group in
            group.addTask { await collectTask.value }
            group.addTask {
                try? await Task.sleep(nanoseconds: 2_000_000_000)
                return []
            }
            let first = await group.next() ?? []
            group.cancelAll()
            return first
        }

        // 必须收到 2 条（snapshot + heartbeat）；只收到 1 条说明 silent drop.
        XCTAssertEqual(collected.count, 2,
            "still-open RM01 connection 必须仍可 yield；只收到 \(collected.count) 条 → 旧 P1 wedge 复现")
        if collected.count >= 2 {
            // 第二条应该是 .memberLeft（不是 .unknown）
            if case .unknown = collected[1] {
                XCTFail("expected member.left as 2nd message; got .unknown — 解码失败")
            }
        }

        client.disconnect()
    }

    // MARK: - case#R17 (fix-review round 5 P2)
    // connect(roomId:) 在 reconnect-attempt 已在 `connectInternal` 内 await connectGate 时被调用 ——
    // 旧实装直接覆盖 connectGate 字段，被覆盖的旧 continuation 永远 hang（其后续 resolve 因 session
    // 不匹配 silent drop）→ 旧 reconnect 的 await 永久 suspend，task 泄漏.
    //
    // 触发条件：reconnect backoff 期间 caller 主动 connect("ROOM_B")；或两个 caller 极端 race.
    // 测试设计：
    //   1. 起一个 manual `connectInternal` await（用 fresh client + 永不返回 frame 的 firstTask）—— 这个
    //      `connect()` Task 会一直 stuck 在 `withCheckedThrowingContinuation`（即 connectGate）直到
    //      被显式 resume.
    //   2. 同步 connect("ROOM_B") —— install 新 connectGate；旧实装会丢弃旧 gate 让旧 connect Task hang.
    //   3. 验证：旧 connect Task 在合理时间内拿到 thrown error（不再 hang），且新 connect 成功.

    func test_connect_supersededInflightGateMustBeResolved_round5_P2() async throws {
        let factory = FakeReconnectFactory()

        // 首个 task：永不返回任何 frame —— 让 first connect 永远 stuck 在 connectGate await（在 P2 修复前
        // 模拟"被覆盖且永远 hang"的场景；修复后此 connect 应被 superseded 抛出错误）.
        let firstTask = factory.scheduleNewTask()
        firstTask.scriptedFrames = []
        firstTask.blockReceiveForever = true  // receive() 永远阻塞

        // 第二次 connect ROOM_B：snapshot 解 latch.
        let secondTask = factory.scheduleNewTask()
        let roomBSnapshot = """
        {
          "type": "room.snapshot",
          "requestId": "",
          "payload": {
            "room": {"id": "ROOM_B", "maxMembers": 4, "memberCount": 0},
            "members": []
          },
          "ts": 2
        }
        """
        secondTask.scriptedFrames = [.string(roomBSnapshot)]
        secondTask.blockReceiveForever = true

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )

        // 1. 起首个 connect Task —— 它会 stuck 在 connectGate 因为 firstTask 不返回任何 frame.
        let firstConnectExpectation = expectation(description: "first connect throws or returns")
        let firstConnectResult = SendableBox<Result<Void, Error>?>(value: nil)
        let firstConnectTask = Task {
            do {
                try await client.connect(roomId: "RM01")
                firstConnectResult.set(.success(()))
            } catch {
                firstConnectResult.set(.failure(error))
            }
            firstConnectExpectation.fulfill()
        }

        // 给 firstConnectTask 充分时间进入 await connectGate（首帧 task 已 install 但 receive() block）.
        try await Task.sleep(nanoseconds: 100_000_000)  // 100ms

        // 2. 同步发起第二次 connect ROOM_B —— P2 修复后会 resolve 旧 gate（throw connectionFailed），
        //    然后 install 自己的 gate；secondTask snapshot yield → 解第二个 gate.
        try await client.connect(roomId: "ROOM_B")

        // 3. 旧 connect Task 必须在合理时间内拿到 thrown error（不能 hang）—— P2 旧实装会 hang 至超时.
        await fulfillment(of: [firstConnectExpectation], timeout: 2.0)
        firstConnectTask.cancel()  // 防御性

        let firstResult = firstConnectResult.value
        XCTAssertNotNil(firstResult, "first connect 必须 settle（不能永久 suspended）")
        guard case .failure(let err)? = firstResult else {
            XCTFail("first connect 应抛错（被新 connect superseded）；实际 result=\(String(describing: firstResult))")
            return
        }
        // 期望 WSError.connectionFailed —— underlyingDescription 包含 "superseded".
        guard case WSError.connectionFailed(let desc) = err else {
            XCTFail("expected WSError.connectionFailed (superseded), got \(err)")
            return
        }
        XCTAssertTrue(desc.contains("superseded"),
            "underlyingDescription 应明示 superseded；实际=\(desc)")

        client.disconnect()
    }

    // MARK: - Story 12.6 heartbeat 测试

    /// "握手 OK" 信号最小 room.snapshot 用于 heartbeat 测试.
    private static let hbSnapshotJSON = """
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

    /// case#HB1 happy: 心跳间隔到 → 发 ping → 收到 pong → 继续下一轮（task 仍 running，不进 timeout）.
    func test_heartbeat_intervalElapsed_sendsPingAndReceivesPongContinues() async throws {
        let factory = FakeWebSocketTaskFactory()
        // scriptedFrames：第一帧 snapshot（解 connect latch + 启 heartbeat task） + 第二帧 pong（让 heartbeat 不超时）
        let pongJSON = """
        {"type":"pong","requestId":"ping_1","payload":{},"ts":2}
        """
        factory.fakeTask.scriptedFrames = [.string(Self.hbSnapshotJSON), .string(pongJSON)]
        factory.fakeTask.blockReceiveForever = true

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        // 短间隔 + 长 pongTimeout（确保 pong 在超时窗口内到达）.
        client.heartbeatInterval = 0.05
        client.pongTimeout = 1.0

        try await client.connect(roomId: "RM01")

        // 等 0.2s（≈4 个心跳间隔）让首次 ping 发出 + pong 在 0.05s 后到达.
        try await Task.sleep(nanoseconds: 200_000_000)

        // 断言：sentMessages 包含至少 1 次 ping
        let sent = factory.fakeTask.sentMessages
        XCTAssertGreaterThanOrEqual(sent.count, 1, "heartbeat task 至少发了一次 ping")
        guard case .string(let text) = sent.first else {
            XCTFail("first sent message should be string frame, got \(String(describing: sent.first))")
            return
        }
        let data = try XCTUnwrap(text.data(using: .utf8))
        let json = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
        XCTAssertEqual(json["type"] as? String, "ping", "heartbeat 发的是 ping")
        XCTAssertEqual(json["requestId"] as? String, "ping_1", "首次 ping requestId == ping_1")

        // 断言：pong 已识别 → underlying task 不应被 cancel(.goingAway).
        XCTAssertNotEqual(factory.fakeTask.lastCancelCloseCode, .goingAway,
                          "pong 已正常到达，不应触发 .goingAway cancel")

        client.disconnect()
    }

    /// case#HB2 edge: 5s 未收到 pong → cancel underlying task with .goingAway → 触发 12.5 transient reconnect.
    func test_heartbeat_pongTimeout_cancelsUnderlyingWithGoingAwayTriggersReconnect() async throws {
        let factory = FakeReconnectFactory()
        // 第一个 task：发 snapshot 解 latch；之后没有 pong 让 heartbeat 5s 超时.
        // pong-timeout cancel(.goingAway) 后 stubbedCloseCode 必须 = .goingAway 让 receive-loop 走 transient (1001).
        let firstTask = factory.scheduleNewTask()
        firstTask.scriptedFrames = [.string(Self.hbSnapshotJSON)]
        firstTask.blockReceiveForever = true
        // 注意：cancel(with:) 在 fake handle 内已记录 lastCancelCloseCode；但 closeCode getter 返回的是 stubbedCloseCode.
        // 为了让 receive-loop 在 cancel 后抛错时拿到 1001 = .goingAway 走 transient，pre-set stubbedCloseCode.
        firstTask.stubbedCloseCode = .goingAway

        // 第二个 task：reconnect attempt → 发 snapshot 让 .reconnecting → .connected 链跑通.
        let secondTask = factory.scheduleNewTask()
        secondTask.scriptedFrames = [.string(Self.hbSnapshotJSON)]
        secondTask.blockReceiveForever = true

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        client.heartbeatInterval = 0.05
        client.pongTimeout = 0.05  // 短超时让测试快
        client.backoffSequence = [0.05, 0.05, 0.05, 0.05, 0.05]

        try await client.connect(roomId: "RM01")

        // 收集 stream 上的 connection states 至 3：[.connected, .reconnecting(1), .connected].
        let states = try await collectConnectionStates(client: client, count: 3, timeout: 5.0)
        XCTAssertGreaterThanOrEqual(states.count, 3, "应至少收到 3 个 state events")
        XCTAssertEqual(states[0], .connected, "首次 connect 后 emit .connected")
        if case .reconnecting(let attempt) = states[1] {
            XCTAssertEqual(attempt, 1, "pong 超时触发 .reconnecting(attempt: 1)")
        } else {
            XCTFail("Expected .reconnecting(attempt: 1), got \(states[1])")
        }
        XCTAssertEqual(states[2], .connected, "reconnect 成功后 emit .connected")

        // 断言：firstTask 收到了 .goingAway cancel.
        XCTAssertEqual(firstTask.lastCancelCloseCode, .goingAway,
                       "pong 超时应 cancel underlying with .goingAway")
        // 断言：factory 真发了第二次 makeTask（reconnect 触发）.
        XCTAssertEqual(factory.makeTaskCallCount, 2, "应触发 reconnect → 第二次 makeTask")

        client.disconnect()
    }

    /// case#HB2b edge (round 3 P1): heartbeat send(.ping) 抛错（locally broken socket）→
    /// 强制走与 pong timeout 相同的 fallback —— cancel underlying with .goingAway → receive-loop catch
    /// transient → schedule reconnect. 不能 silent return（旧 bug：客户端卡死无 heartbeat 无 reconnect）.
    func test_heartbeat_pingSendThrows_cancelsUnderlyingWithGoingAwayTriggersReconnect() async throws {
        let factory = FakeReconnectFactory()
        // 第一个 task：发 snapshot 解 latch；之后 receive 阻塞（模拟 "locally broken socket，receive 仍 blocked"）.
        let firstTask = factory.scheduleNewTask()
        firstTask.scriptedFrames = [.string(Self.hbSnapshotJSON)]
        firstTask.blockReceiveForever = true
        // sendThrowsError 让 heartbeat 第一次 send(.ping) 立即抛错（snapshot 帧已 receive，handshake latch 已解）.
        firstTask.sendThrowsError = URLError(.notConnectedToInternet)
        // cancel(.goingAway) 后 closeCode 必须 = .goingAway 让 receive-loop 走 transient (1001).
        firstTask.stubbedCloseCode = .goingAway

        // 第二个 task：reconnect attempt → 发 snapshot 让 .reconnecting → .connected 链跑通.
        let secondTask = factory.scheduleNewTask()
        secondTask.scriptedFrames = [.string(Self.hbSnapshotJSON)]
        secondTask.blockReceiveForever = true

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        client.heartbeatInterval = 0.05
        client.pongTimeout = 1.0  // 长 pongTimeout 排除 "pong timeout 触发 reconnect"路径，确认是 send 抛错触发
        client.backoffSequence = [0.05, 0.05, 0.05, 0.05, 0.05]

        try await client.connect(roomId: "RM01")

        // 收集 stream 上的 connection states 至 3：[.connected, .reconnecting(1), .connected].
        let states = try await collectConnectionStates(client: client, count: 3, timeout: 5.0)
        XCTAssertGreaterThanOrEqual(states.count, 3, "应至少收到 3 个 state events")
        XCTAssertEqual(states[0], .connected, "首次 connect 后 emit .connected")
        if case .reconnecting(let attempt) = states[1] {
            XCTAssertEqual(attempt, 1, "ping send 抛错触发 .reconnecting(attempt: 1)")
        } else {
            XCTFail("Expected .reconnecting(attempt: 1), got \(states[1])")
        }
        XCTAssertEqual(states[2], .connected, "reconnect 成功后 emit .connected")

        // 断言：firstTask 收到了 .goingAway cancel（验证走的是 cancelUnderlyingTaskWithGoingAwayIfCurrent 路径）.
        XCTAssertEqual(firstTask.lastCancelCloseCode, .goingAway,
                       "ping send 抛错应 cancel underlying with .goingAway")
        // 断言：firstTask 至少 "尝试" send 过一次 ping（fake handle 在抛错前已 append 到 sentMessages）.
        XCTAssertGreaterThanOrEqual(firstTask.sentMessages.count, 1,
                                    "send(.ping) 至少被调用一次（fake handle 抛错前已 append）")
        // 断言：factory 真发了第二次 makeTask（reconnect 触发）.
        XCTAssertEqual(factory.makeTaskCallCount, 2, "应触发 reconnect → 第二次 makeTask")

        client.disconnect()
    }

    /// case#HB3 happy: disconnect() 后 heartbeat task 停止（不再发 ping）.
    func test_heartbeat_disconnectStopsHeartbeatTaskNoMorePing() async throws {
        let factory = FakeWebSocketTaskFactory()
        let pongJSON = """
        {"type":"pong","requestId":"ping_1","payload":{},"ts":2}
        """
        factory.fakeTask.scriptedFrames = [.string(Self.hbSnapshotJSON), .string(pongJSON)]
        factory.fakeTask.blockReceiveForever = true

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        client.heartbeatInterval = 0.05
        client.pongTimeout = 1.0

        try await client.connect(roomId: "RM01")

        // 等 0.2s 让首次 ping 发出.
        try await Task.sleep(nanoseconds: 200_000_000)
        let countBefore = factory.fakeTask.sentMessages.count
        XCTAssertGreaterThanOrEqual(countBefore, 1, "disconnect 前已发出 ping")

        // 调 disconnect → cancel heartbeatTask.
        client.disconnect()

        // 等 0.3s（≥6 个心跳间隔）确认不再增长.
        try await Task.sleep(nanoseconds: 300_000_000)
        let countAfter = factory.fakeTask.sentMessages.count
        XCTAssertEqual(countAfter, countBefore,
                       "disconnect 后 heartbeat task 必须停止发 ping; before=\(countBefore) after=\(countAfter)")
    }

    /// case#HB4 happy: reconnect 成功后 heartbeat task 重启（second fake task 也收到 ping）.
    func test_heartbeat_taskRestartsAfterReconnectSuccess() async throws {
        let factory = FakeReconnectFactory()
        // 第一个 task：发 snapshot → 没有 pong → heartbeat 5s timeout → cancel(.goingAway) → reconnect.
        let firstTask = factory.scheduleNewTask()
        firstTask.scriptedFrames = [.string(Self.hbSnapshotJSON)]
        firstTask.blockReceiveForever = true
        firstTask.stubbedCloseCode = .goingAway

        // 第二个 task（reconnect 后）：发 snapshot + pong（让 heartbeat 不再超时）.
        let secondTask = factory.scheduleNewTask()
        let pongJSON = """
        {"type":"pong","requestId":"ping_1","payload":{},"ts":3}
        """
        secondTask.scriptedFrames = [.string(Self.hbSnapshotJSON), .string(pongJSON)]
        secondTask.blockReceiveForever = true

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        client.heartbeatInterval = 0.05
        client.pongTimeout = 0.05
        client.backoffSequence = [0.05, 0.05, 0.05, 0.05, 0.05]

        try await client.connect(roomId: "RM01")

        // 等 0.5s 给：firstTask 超时（~0.1s）→ reconnect 触发（~0.05s backoff）→ secondTask connect → secondTask 心跳 ping.
        try await Task.sleep(nanoseconds: 500_000_000)

        // 断言：second fake task 收到至少一次 ping —— 证明 heartbeatTask 在 reconnect 后已重启.
        // 注意：fake handle 的 scriptedFrames 静态调度无法精准模拟"ping 发出后 server 回 pong" 的真实时序
        // —— pong frame 在 heartbeat 第一次发 ping 之前就被 receive-loop 消费掉，pendingPongContinuation 还
        // 没创建，notify silent drop。所以本 case 的核心断言在"heartbeat 重启 → 第二个 task 收到 ping"，
        // 不强求 second task 不被 .goingAway cancel（那是 fake 调度限制下的副作用，非 production 行为）.
        XCTAssertGreaterThanOrEqual(secondTask.sentMessages.count, 1,
                                    "reconnect 成功后 heartbeat 必须重启 → secondTask 应收到 ping")

        client.disconnect()
    }

    /// fix-review round 1 P1：transient close 触发自动 reconnect 时，旧 heartbeat 的 in-flight pong timer
    /// **不能**在新 underlyingTask install 之后 fire 并错杀新 socket.
    ///
    /// 时序设计（pongTimeout=500ms / heartbeatInterval=50ms）：
    ///   T0     : connect → firstTask snapshot → heartbeat 启动
    ///   T0+50  : heartbeat 第一次 ping firstTask → 进 awaitPongOrTimeout（pongTimeout=500ms）
    ///   T0+150 : 测试主动 cancel firstTask（stubbedCloseCode=.goingAway → 1001 transient）
    ///   T0+170 : receive-loop catch transient → cancelHeartbeatStateForReconnectIfCurrent（**修复点**）
    ///            + scheduleReconnect（backoff=50ms）
    ///   T0+220 : attemptReconnect → secondTask install + 重启 heartbeat
    ///   T0+550 : **修复未生效时**：firstTask 的旧 pong timer fire → cancel(.goingAway, secondTask) → race bug
    ///            **修复生效时**：旧 heartbeatTask 已被 cancel + pongCont 已 finish → timer 不 fire
    ///   T0+700 : 测试断言时间点（介于 T0+550 与 secondTask 自己 heartbeat 超时 ~T0+790 之间）
    ///
    /// 选取 T_assert=700ms 的边界推导：
    ///   下限 550ms ：旧 pong timer fire 时间（必须等过；早断言修复前/后无差异）
    ///   上限 ~790ms：secondTask 自己 heartbeat 第一次 ping (~T0+270) + pongTimeout(500ms) ≈ T0+790
    ///                早于此时点断言可避免被 secondTask 自己的合法 timeout 干扰.
    ///   700ms 居中 + 留 ~90ms 时序抖动余量.
    ///
    /// 断言：secondTask.lastCancelCloseCode != .goingAway（修复前 == .goingAway 即失败）.
    func test_heartbeat_oldPongTimeoutDoesNotCancelInflightReconnectSocket_round1_P1() async throws {
        let factory = FakeReconnectFactory()

        // firstTask：发 snapshot（启动 heartbeat）→ blockReceiveForever
        // 测试主动调 cancel(with:.goingAway) 模拟 server transient close.
        // closeCode getter 在 cancel 后仍读 stubbedCloseCode → .goingAway → classify = transient (1001).
        let firstTask = factory.scheduleNewTask()
        firstTask.scriptedFrames = [.string(Self.hbSnapshotJSON)]
        firstTask.blockReceiveForever = true
        firstTask.stubbedCloseCode = .goingAway

        // secondTask：reconnect 后的新 socket，发 snapshot 让 connect 路径完成.
        // 关键：blockReceiveForever 保活 → 任何 .goingAway cancel 必定来自外部错杀路径（race bug）
        //       或 secondTask 自己 heartbeat timeout（在 T0+790 后才 fire，断言在 T0+700 不会撞上）.
        let secondTask = factory.scheduleNewTask()
        secondTask.scriptedFrames = [.string(Self.hbSnapshotJSON)]
        secondTask.blockReceiveForever = true
        secondTask.stubbedCloseCode = .invalid

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        // heartbeatInterval 50ms：保证 ping 在 transient close 之前发出 → heartbeat 进 awaitPongOrTimeout.
        // pongTimeout 500ms：足够长让 reconnect install 新 socket 在旧 pong timer fire 之前完成.
        client.heartbeatInterval = 0.05
        client.pongTimeout = 0.5
        client.backoffSequence = [0.05, 0.05, 0.05, 0.05, 0.05]

        try await client.connect(roomId: "RM01")

        // T0+150ms：heartbeat 已发 ping (T0+50ms)，仍在 awaitPongOrTimeout 等 pong（T0+550ms 才超时）.
        try await Task.sleep(nanoseconds: 150_000_000)
        XCTAssertGreaterThanOrEqual(firstTask.sentMessages.count, 1,
                                    "前置：heartbeat 应已发 ping 进入 awaitPongOrTimeout")

        // 主动 cancel firstTask 模拟 server transient close（rawValue 1001）.
        // receive-loop catch → transient → 修复点 cancelHeartbeatStateForReconnectIfCurrent → schedule reconnect.
        firstTask.cancel(with: .goingAway, reason: nil)

        // sleep 到 T0+700ms（再睡 550ms）：
        //   - 已过 T0+550ms，旧 pong timer 应已 fire / 被 cancel；
        //   - 还没到 secondTask 自己 heartbeat 超时点（~T0+790ms），不会被合法 timeout 干扰断言.
        try await Task.sleep(nanoseconds: 550_000_000)

        // 关键断言：secondTask **不应**被 .goingAway cancel.
        // 修复前 race：旧 heartbeat 仍活 → T0+550ms pong timer fire → cancelUnderlyingTaskWithGoingAwayIfCurrent
        // 持锁查 underlyingTask（此刻 = secondTask）→ cancel(.goingAway, secondTask) → 此断言失败.
        XCTAssertNotEqual(secondTask.lastCancelCloseCode, .goingAway,
                          "旧 heartbeat 的 pong timeout 不应 cancel reconnect 后的新 socket（race bug 修复验证）")

        // 同时验证：reconnect 真发生过（factory 真发了 2 次 makeTask）.
        XCTAssertGreaterThanOrEqual(factory.makeTaskCallCount, 2,
                                    "transient close 应触发 reconnect")

        client.disconnect()
    }

    /// fix-review round 2 P2：heartbeat .pong 必须按 requestId 配对当前 in-flight ping.
    /// 旧实装无条件 ack 任何 .pong → server 推 stale pong（旧 ping 的迟到 / 重复 pong）会
    /// 错误 ack 当前 in-flight ping → miss 当前 ping 实际未被 ack → 推迟一整个 heartbeat
    /// interval 才检测到 reconnect 需要.
    ///
    /// 时序设计（heartbeatInterval=50ms / pongTimeout=300ms / framedelay=120ms）：
    ///   T0     : connect → snapshot 解 latch → heartbeat 启动
    ///   T0+50  : heartbeat 第一次 ping (requestId="ping_1") → pendingPongRequestId="ping_1" → 进 awaitPongOrTimeout
    ///   T0+120 : fake handle frame[1] = stale pong (requestId="stale_id") 到达 receive-loop
    ///            → notifyPongReceivedIfCurrent("stale_id", mySession) 校验 "stale_id" != "ping_1" → silent drop
    ///   T0+350 : pongTimeout fire → cancel underlying with .goingAway (1001 transient)
    ///   T0+400 : 测试断言时间点
    ///
    /// 修复前断言失败：notify 无 requestId 校验 → stale pong 错配 ack ping_1 → awaitPongOrTimeout
    ///   return false → continue 下一轮 sleep；underlying 不会被 cancel(.goingAway)。
    /// 修复后断言成立：requestId 校验 silent drop stale → pongTimeout 真正 fire → cancel(.goingAway).
    func test_heartbeat_stalePongMismatchedRequestIdDoesNotAckInflightPing_round2_P2() async throws {
        let factory = FakeWebSocketTaskFactory()
        // frame[0]: snapshot 解 connect latch + 启 heartbeat
        // frame[1]: stale pong with requestId="stale_id"（不匹配 ping_1）
        let stalePongJSON = """
        {"type":"pong","requestId":"stale_id","payload":{},"ts":99}
        """
        factory.fakeTask.scriptedFrames = [.string(Self.hbSnapshotJSON), .string(stalePongJSON)]
        // frame[1] 注入 120ms 延迟 → ping_1 在 T0+50 已发出（pendingPongRequestId="ping_1"）→ T0+120 stale pong 到达.
        factory.fakeTask.frameDelaysSec = [0.0, 0.12]
        factory.fakeTask.blockReceiveForever = true

        let client = WebSocketClientImpl(
            baseURL: URL(string: "http://localhost:8080")!,
            tokenProvider: { "tok" },
            taskFactory: factory
        )
        client.heartbeatInterval = 0.05
        client.pongTimeout = 0.3   // T0+50 ping → T0+350 pong timeout
        client.backoffSequence = [10.0]  // 防 reconnect attempt 干扰断言（pongTimeout 触发后我们不需要 reconnect 跑通）

        try await client.connect(roomId: "RM01")

        // 等 ~T0+400ms：足够让 stale pong 到达 + pongTimeout fire + cancelUnderlyingTaskWithGoingAwayIfCurrent.
        try await Task.sleep(nanoseconds: 400_000_000)

        // 修复后核心断言：stale pong 被 silent drop → pongTimeout fire → cancel(.goingAway).
        XCTAssertEqual(factory.fakeTask.lastCancelCloseCode, .goingAway,
                       "stale pong (requestId mismatch) 必须被 silent drop → pongTimeout 真触发 cancel(.goingAway)")

        // 顺带验证：ping 真发出过（前置条件，否则断言无意义）.
        let sent = factory.fakeTask.sentMessages
        XCTAssertGreaterThanOrEqual(sent.count, 1, "前置：heartbeat task 应已发出至少一次 ping")
        guard case .string(let text) = sent.first else {
            XCTFail("first sent message should be string frame")
            return
        }
        let data = try XCTUnwrap(text.data(using: .utf8))
        let json = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
        XCTAssertEqual(json["requestId"] as? String, "ping_1", "首次 ping requestId == ping_1（与 stale pong requestId='stale_id' 必然不匹配）")

        client.disconnect()
    }

    // MARK: - reconnect 测试 helpers

    /// 收集 stream 上的 connection state events 直到拿到指定数量（带超时）.
    private func collectConnectionStates(
        client: WebSocketClientImpl,
        count: Int,
        timeout: TimeInterval
    ) async throws -> [WSConnectionState] {
        let stream = client.messages
        var collected: [WSConnectionState] = []
        let exp = expectation(description: "collected \(count) connection states")
        let task = Task {
            for await msg in stream {
                if case .connectionStateChanged(let state) = msg {
                    collected.append(state)
                    if collected.count >= count {
                        exp.fulfill()
                        return
                    }
                }
            }
        }
        await fulfillment(of: [exp], timeout: timeout)
        task.cancel()
        return collected
    }

    /// 收集 stream 上 connection state events 直到 stream finish（用于 terminal 路径验证）.
    private func collectConnectionStatesUntilFinish(
        client: WebSocketClientImpl,
        timeout: TimeInterval
    ) async throws -> [WSConnectionState] {
        let stream = client.messages
        var collected: [WSConnectionState] = []
        let exp = expectation(description: "stream finished")
        let task = Task {
            for await msg in stream {
                if case .connectionStateChanged(let state) = msg {
                    collected.append(state)
                }
            }
            exp.fulfill()
        }
        await fulfillment(of: [exp], timeout: timeout)
        task.cancel()
        return collected
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

    /// Story 12.5：取 stream 上第一条**非** connectionStateChanged 消息（带超时）.
    /// connectionStateChanged 是 client-internal emit，server-side payload 测试想跳过这类事件.
    private func firstNonConnectionState(
        from stream: AsyncStream<WSMessage>,
        timeout: TimeInterval
    ) async throws -> WSMessage {
        try await withThrowingTaskGroup(of: WSMessage.self) { group in
            group.addTask {
                for await msg in stream {
                    if case .connectionStateChanged = msg { continue }
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

    /// Story 12.5：模拟 server emit 的 close code —— 在 `receiveErrorCloseCode` 模式下,
    /// receive() 抛错时让 closeCode getter 返回此值.
    /// 默认 `.invalid`（rawValue=0 ↔ 1006 client 本地合成；与 production URLSessionWebSocketTask 默认一致）.
    var stubbedCloseCode: URLSessionWebSocketTask.CloseCode = .invalid

    /// Story 12.5：scriptedFrames 耗尽后立即抛错（不 sleep 100ms）—— 模拟 transient/terminal close 触发链.
    /// 与 blockReceiveForever 互斥：blockReceiveForever=true → 阻塞；errorAfterFrames=true → 立即抛错.
    var errorAfterFrames: Bool = false

    /// Story 12.6 fix-review round 2 P2：每个 scriptedFrames index 对应的 pre-receive 延迟（秒）.
    /// 数组与 scriptedFrames 同 index 配对；超过 scriptedFrames 长度的 entry 忽略.
    /// 用于让 stale pong 等场景在 heartbeat 已发 ping 之后才到达 receive-loop（精准重现 race 时序）.
    /// 默认 nil = 不 sleep（与原行为一致）.
    var frameDelaysSec: [TimeInterval]?

    /// Story 12.6 fix-review round 3 P1：send 抛错开关 —— 模拟 "locally broken socket"
    /// （URLSessionWebSocketTask.send 失败但 receive() 仍 blocked，没观察到 close）.
    /// 默认 nil = 不抛（与原行为一致）；非 nil = 每次 send 都抛此错误.
    var sendThrowsError: Error?

    private var receiveIndex: Int = 0
    private var isCancelled: Bool = false
    private let lock = NSLock()

    var isRunning: Bool {
        lock.lock(); defer { lock.unlock() }
        return !isCancelled
    }

    /// Story 12.5：暴露 stubbedCloseCode 让 reconnect 状态机分类决策.
    var closeCode: URLSessionWebSocketTask.CloseCode {
        lock.lock(); defer { lock.unlock() }
        return stubbedCloseCode
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
        let throwErr = sendThrowsError
        lock.unlock()
        if let throwErr = throwErr {
            // round 3 P1：先 append 再抛，让测试可以验证 "送出 ping 帧但 send 失败"路径.
            throw throwErr
        }
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
        let errorAfter = errorAfterFrames
        let delays = frameDelaysSec
        if idx < frames.count {
            receiveIndex += 1
        }
        lock.unlock()

        if cancelled {
            throw URLError(.cancelled)
        }
        if idx < frames.count {
            // round 2 P2：按 index 查 delay；让 stale pong 等场景能在 heartbeat ping 已发出之后才到达.
            if let ds = delays, idx < ds.count, ds[idx] > 0 {
                try await Task.sleep(nanoseconds: UInt64(ds[idx] * 1_000_000_000))
            }
            return frames[idx]
        }
        if errorAfter {
            // Story 12.5：耗尽后立即抛错（不 sleep）—— 模拟 server 主动 close（带 stubbedCloseCode）.
            // 给 receive loop 极短时间消化 first frame（让 firstFrameReceived 在抛错前 set 为 true）.
            try await Task.sleep(nanoseconds: 5_000_000)  // 5ms
            throw URLError(.cancelled)
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

// MARK: - Story 12.5 reconnect-friendly factory

/// Story 12.5 reconnect 状态机测试用 factory：每次 `makeTask` 返回**新**的 fake handle，
/// 让单测可独立配置每次 reconnect attempt 的行为（首帧 / 抛错 / closeCode）.
final class FakeReconnectFactory: WebSocketTaskFactory, @unchecked Sendable {
    var requests: [URLRequest] = []
    /// 预先 schedule 好的 handles 队列；按 makeTask 顺序消费.
    var handles: [FakeWebSocketTaskHandle] = []
    /// 已 makeTask 调用次数（断言 reconnect 是否真的触发了第二次 makeTask）.
    var makeTaskCallCount: Int = 0

    private let lock = NSLock()

    /// 提前 schedule 一个新 fake handle 进队列；返回引用让测试 case 配置 scriptedFrames / closeCode.
    func scheduleNewTask() -> FakeWebSocketTaskHandle {
        let h = FakeWebSocketTaskHandle()
        lock.lock()
        handles.append(h)
        lock.unlock()
        return h
    }

    func makeTask(with request: URLRequest) -> WebSocketTaskHandle {
        lock.lock()
        requests.append(request)
        makeTaskCallCount += 1
        let h: FakeWebSocketTaskHandle
        if !handles.isEmpty {
            h = handles.removeFirst()
        } else {
            // 队列耗尽 fallback：返回一个 immediately-error fake（防测试漏配置卡住）
            let stub = FakeWebSocketTaskHandle()
            stub.scriptedFrames = []
            stub.errorAfterFrames = true
            stub.stubbedCloseCode = .invalid
            h = stub
        }
        lock.unlock()
        return h
    }
}

// MARK: - SendableBox helper (fix-review round 5 P2 test)

/// 跨 task 边界传递可变值的最简 box —— `@unchecked Sendable` 通过 NSLock 保护内部 var.
/// 仅供测试用，不进 production 代码.
final class SendableBox<T>: @unchecked Sendable {
    private var _value: T
    private let lock = NSLock()

    init(value: T) {
        self._value = value
    }

    var value: T {
        lock.lock(); defer { lock.unlock() }
        return _value
    }

    func set(_ newValue: T) {
        lock.lock(); defer { lock.unlock() }
        _value = newValue
    }
}

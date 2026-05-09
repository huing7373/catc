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
        let errorAfter = errorAfterFrames
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

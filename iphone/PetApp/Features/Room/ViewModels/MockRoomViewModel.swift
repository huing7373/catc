// MockRoomViewModel.swift
// Story 37.8 AC2: RoomViewModel mock 子类，用于 #Preview / UITest skip-guest-login / Scaffold 单元测试.
//
// 设计：
//   - 默认数据来自 RoomScaffoldDefaults（与 RealRoomViewModel 共享，避免双源；详见 round 1 P2 fix lesson）
//   - override 2 个 abstract method 仅 print log + 记录 invocations 数组
//   - **不**依赖 AppState（Mock 路径走纯 ViewModel-only 数据）
//
// 与 MockHomeViewModel Story 37.7 同模式（invocations 数组而非 closure spy；显式 import Combine 防 transitive 漂移）.
//
// round 1 P2 fix：默认值不再 hardcode 在本文件，统一走 RoomScaffoldDefaults；
// `fourMembersMock` 转发到 `RoomScaffoldDefaults.members` 仅作为 Mock 子类的对外 alias 保留
// （Preview / Tests 旧引用 `MockRoomViewModel.fourMembersMock` 字面量保持兼容）.

import Foundation
import Combine
import os.log

@MainActor
public final class MockRoomViewModel: RoomViewModel {
    /// 单元测试用：记录所有方法调用
    public enum Invocation: Equatable {
        case leaveTap
        case copyTap
    }

    @Published public var invocations: [Invocation] = []

    /// 默认构造 — 4 成员（房主 + 3 普通成员）满员场景，对齐 ui_design room.jsx 默认 demo 数据.
    /// 全部默认值走 RoomScaffoldDefaults（与 RealRoomViewModel init seed 同源）.
    public override init() {
        super.init()
        self.roomCodeForCopy = RoomScaffoldDefaults.roomCodeForCopy
        self.hostCatName = RoomScaffoldDefaults.hostCatName
        self.members = RoomScaffoldDefaults.members
        self.userIsHost = RoomScaffoldDefaults.userIsHost
    }

    /// 测试 / Preview 灵活构造 — 可注入任意 members 数（0/1/2/3/4）+ 自定 roomCode + hostCatName.
    /// 用于单元测试 case#1 (4 成员)、case#2 (2 成员) 等；与 HomeViewScaffoldTests 同模式.
    /// 默认形参全部从 RoomScaffoldDefaults 取（与无参 init() 同源）.
    public init(
        roomCodeForCopy: String = RoomScaffoldDefaults.roomCodeForCopy,
        hostCatName: String = RoomScaffoldDefaults.hostCatName,
        members: [RoomMember] = RoomScaffoldDefaults.members,
        userIsHost: Bool = RoomScaffoldDefaults.userIsHost
    ) {
        super.init()
        self.roomCodeForCopy = roomCodeForCopy
        self.hostCatName = hostCatName
        self.members = members
        self.userIsHost = userIsHost
    }

    // MARK: - override abstract methods

    public override func onLeaveTap() {
        os_log(.debug, "MockRoomViewModel.onLeaveTap")
        invocations.append(.leaveTap)
    }

    public override func onCopyTap() {
        os_log(.debug, "MockRoomViewModel.onCopyTap")
        invocations.append(.copyTap)
    }

    // MARK: - mock 数据

    /// 4 成员满员 mock（房主 + 3 普通）—— 转发到 RoomScaffoldDefaults.members 保持 API 兼容.
    /// round 1 P2 fix 之后单源真值在 RoomScaffoldDefaults；此处保留作 Preview / Tests 引用入口.
    public static var fourMembersMock: [RoomMember] { RoomScaffoldDefaults.members }

    /// 2 成员场景 mock（房主 + 1）；用于单元测试 case#2 验证空位渲染.
    public static let twoMembersMock: [RoomMember] = [
        RoomMember(id: "u1", name: "小花", level: 8, status: "在玩耍", isHost: true),
        RoomMember(id: "u2", name: "Mocha", level: 7, status: "在散步", isHost: false),
    ]
}

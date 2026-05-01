// RoomScaffoldDefaults.swift
// Story 37.8 round 1 P2 fix: Mock 与 Real RoomViewModel 共享 scaffold 占位数据.
//
// 背景（codex review round 1 P2 finding）：
//   原 RealRoomViewModel.init() 仅 set roomCodeForCopy="" / hostCatName="默认小猫"，**不** seed members / userIsHost.
//   后果：UITEST_FORCE_IN_ROOM 路径 / Story 37.12 后 JoinRoomModal 落地 / 任何 in-room 走 Real path 的场景，
//   RoomScaffoldView 渲染近乎空房间（4 个 mock member 占位、host cat 占位都缺失）→
//   Story 37.8 "RoomScaffoldView 替代 RoomViewPlaceholder" 的 deliverable 在 Real 路径形同未交付.
//
// 设计决议（与 user review summary "option A" 对齐）：
//   抽 shared defaults 而非 hardcode 在两个 ViewModel —— 避免 Mock/Real 重复定义 mock 数据，
//   未来 Story 12.1 接 WS room.snapshot 时只需在 RealRoomViewModel sink 内覆盖即可，**不**动 Mock.
//
// 边界：本 defaults 与 MockRoomViewModel.fourMembersMock 字段值完全一致 —— 后者保留作为 Mock 子类的对外 API
// （Preview / 测试场景仍直接读 `MockRoomViewModel.fourMembersMock`），内部 init 走共享 defaults，
// 双方共指同一组数据（fourMembersMock 直接转发到 RoomScaffoldDefaults.members 即可，避免双源）.

import Foundation

/// Mock 与 Real RoomViewModel 启动占位数据（in-room state UI scaffold defaults）.
///
/// **使用规则**（务必读）：
/// - Mock：直接用 RoomScaffoldDefaults 字段初始化 4 个 @Published（参见 MockRoomViewModel.init()）.
/// - Real：init() / init(appState:) 都用 RoomScaffoldDefaults seed 起手；sink 路径
///   （subscribeRoomCode / subscribeHostCatName）作为 override —— currentRoomId 来 → 派生 roomCodeForCopy；
///   currentPet 来 → 派生 hostCatName；都 fallback 到 RoomScaffoldDefaults 占位.
/// - Story 12.1 后：RealRoomViewModel 接 WS room.snapshot → snapshot 来到时覆盖 members / userIsHost；
///   覆盖前仍用 RoomScaffoldDefaults 不让 RoomScaffoldView 渲染空房间.
public enum RoomScaffoldDefaults {
    /// 房间号占位（mock "1234567"；Story 12.1 RealRoomViewModel sink 派生覆盖）.
    public static let roomCodeForCopy: String = "1234567"

    /// 房主猫名占位（mock "小花"；Story 12.1 RealRoomViewModel sink 派生覆盖）.
    public static let hostCatName: String = "小花"

    /// 用户是否房主（mock true —— UITEST_FORCE_IN_ROOM / 节点 1 占位语境下让自身可见为房主）.
    /// Story 12.1 后由 user.id == hostMember.id 派生覆盖.
    public static let userIsHost: Bool = true

    /// 4 成员满员 mock（房主 + 3 普通；level / status 与 ui_design room.jsx demo 一致）.
    /// 与 MockRoomViewModel.fourMembersMock 同源 —— 后者直接转发到本字段（避免双源数据漂移）.
    public static let members: [RoomMember] = [
        RoomMember(id: "u1", name: "小花", level: 8, status: "在玩耍", isHost: true),
        RoomMember(id: "u2", name: "Mocha", level: 7, status: "在散步", isHost: false),
        RoomMember(id: "u3", name: "Latte", level: 9, status: "在玩耍", isHost: false),
        RoomMember(id: "u4", name: "Espresso", level: 6, status: "在休息", isHost: false),
    ]
}

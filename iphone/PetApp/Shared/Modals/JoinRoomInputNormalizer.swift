// JoinRoomInputNormalizer.swift
// Story 37.12 r2: JoinRoomModal 输入归一化纯函数 helper.
//
// 设计动机（review r2 [P2] fix）：
//   原版 JoinRoomModalTests 直接调 `modal.onConfirm(...)` 闭包并**本地复刻** trim / 64-char 截断
//   规则，没有真正触发 view body 内的 `action: { onConfirm(trimmed) }` / `trimmedIsEmpty` /
//   `.onChange` 逻辑。如果 view body 回归（例如 onChange 漏 trim、`.disabled` 漏 trimmedIsEmpty
//   检查、长度截断改成不截断），现有测试仍 pass，feature regression 不抓。
//
// ADR-0002 §3.1 禁用 ViewInspector / SnapshotTesting，所以不能直接断言 SwiftUI body。
// 修法：抽 input normalization 纯函数 helper，view + tests 共用同一函数 → 测试断言此函数
// 等价于断言 view 行为（因为 view 直接调用此 helper）.
//
// 与 `HomeRoomDispatcher.shouldShowRoom(currentRoomId:)` /
// `HomePetNameResolver.resolve(pet:hasHydrated:)` 同精神：
//   抽纯函数让单测直接覆盖；fileprivate / private SwiftUI 子视图 body 难直接断言时的标准模式.
//
// 使用方：
//   - JoinRoomModal `.onChange(of: roomIdInput) { _, newValue in
//         roomIdInput = JoinRoomInputNormalizer.normalize(newValue)
//     }`
//   - JoinRoomModal confirm button `action: { onConfirm(JoinRoomInputNormalizer.normalize(roomIdInput)) }`
//   - JoinRoomModal confirm button `.disabled(JoinRoomInputNormalizer.isSubmitDisabled(roomIdInput))`
//
// 显式 import Foundation（trimmingCharacters / CharacterSet 来自 Foundation；防 transitive import
// 走丢；与 MockHomeViewModel round 4 [P0] hardening 同精神）.

import Foundation

/// JoinRoomModal 输入归一化纯函数 helper（review r2 [P2] fix）.
///
/// **职责**：trim 前后空白 + 截断到 64 字符；判定提交按钮 disabled state.
/// **不**做客户端格式校验（仅 trim + 限长；server 决定合法性，AR21 + epic AC line 4856 钦定）.
public enum JoinRoomInputNormalizer {

    /// 与 JoinRoomModal `.onChange(of: roomIdInput)` 配合：去除前后空白 + 截断到 64 字符.
    ///
    /// - Parameter raw: 原始输入字符串（可能含前后空白 / 超 64 字符）.
    /// - Returns: trim 后 + prefix(64) 后字符串.
    ///
    /// 设计：
    ///   - 先 trim 再 prefix —— 防 64 字符全是空白时 prefix 拿到空白后被 trim 反而 < 64
    ///     的尴尬（trim 后用户可见非空白部分一定 ≤ 64）.
    ///   - 内部空白保留（"abc 123" → "abc 123"，server 决定是否合法）.
    public static func normalize(_ raw: String) -> String {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        return String(trimmed.prefix(64))
    }

    /// 与 JoinRoomModal confirm button `.disabled(...)` 配合：trim 后空字符串 → disabled = true.
    ///
    /// - Parameter raw: 原始输入字符串（含 normalize 前的形态）.
    /// - Returns: true → confirm button disabled（trim 后空 / 仅空白）；false → enabled.
    public static func isSubmitDisabled(_ raw: String) -> Bool {
        normalize(raw).isEmpty
    }
}

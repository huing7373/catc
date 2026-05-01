// JoinRoomModalTests.swift
// Story 37.12 AC5: JoinRoomModal + HomeViewModel.onJoinRoomConfirm 单元测试.
//
// 测试基础设施约束（与 Story 2.7 + ADR-0002 §3.1 衔接）：
//   - 仅依赖 stdlib（XCTest + @testable import PetApp）.
//   - 不引 ViewInspector / SnapshotTesting.
//   - 走 ViewModel 行为 + closure invocation 断言；不走 SwiftUI body 内省.
//   - JoinRoomModal 是纯 presentation View 不持 ViewModel —— 单元测试断言 closure 收到的 trim 后字符串.
//
// review r2 [P2] fix：trim / 64-char 截断 / 提交 disabled 规则下沉到 `JoinRoomInputNormalizer`
// 纯函数 helper，view body 内 `.onChange` / `.disabled` / `action: { onConfirm(normalize(...)) }`
// 三处全部直接调用 helper. 测试断言 helper = 断言 view 行为（共享同一函数源），不再本地复刻规则,
// 抓得住 view body 的 trim / 截断 / disabled 回归.
// 与 `HomePetNameResolverTests` / `HomeContainerViewTests` (HomeRoomDispatcher) 同精神.

import XCTest
import SwiftUI
@testable import PetApp

@MainActor
final class JoinRoomModalTests: XCTestCase {

    // MARK: - JoinRoomInputNormalizer.normalize(_:) — review r2 [P2] fix 覆盖 view body 共享规则

    /// case#N1 happy: 不含空白 + 长度 ≤ 64 → 原样返回（不破坏 happy path 输入）.
    func testNormalizePreservesShortNonWhitespaceInput() {
        XCTAssertEqual(
            JoinRoomInputNormalizer.normalize("1234567"),
            "1234567",
            "短字符串无空白 → 原样返回（最常见 happy path）"
        )
    }

    /// case#N2 happy: 前后空白被去除（驱动 view body confirm `action: { onConfirm(normalize(...)) }`
    /// + `.onChange` 同源 trim 行为）.
    func testNormalizeTrimsLeadingAndTrailingWhitespace() {
        XCTAssertEqual(
            JoinRoomInputNormalizer.normalize("  abc-123  "),
            "abc-123",
            "前后空白必须去除 —— 与 view body confirm closure / .onChange 同源规则"
        )
    }

    /// case#N3 edge: 内部空白保留（"abc 123" → "abc 123"，server 决定是否合法）.
    func testNormalizePreservesInternalWhitespace() {
        XCTAssertEqual(
            JoinRoomInputNormalizer.normalize("abc 123"),
            "abc 123",
            "内部空白必须保留（仅 trim 前后；server 决定 roomId 合法性）"
        )
    }

    /// case#N4 edge: 含换行 / Tab 的前后空白也被 trim（whitespacesAndNewlines 覆盖范围）.
    func testNormalizeTrimsNewlinesAndTabs() {
        XCTAssertEqual(
            JoinRoomInputNormalizer.normalize("\n\tabc\t\n"),
            "abc",
            "whitespacesAndNewlines 必须覆盖 \\n / \\t（防 paste 多行输入）"
        )
    }

    /// case#N5 happy: 长度 == 64 → 原样保留（不截断；边界值守护）.
    func testNormalizePreservesExactly64Chars() {
        let exact64 = String(repeating: "X", count: 64)
        XCTAssertEqual(
            JoinRoomInputNormalizer.normalize(exact64).count,
            64,
            "长度 == 64 应原样保留（< / == / > 三段中的 == 边界）"
        )
    }

    /// case#N6 edge: 长度 > 64 → 截断到 64（驱动 view body `.onChange` 截断行为）.
    func testNormalizeTruncatesLongInputTo64Chars() {
        let longInput = String(repeating: "X", count: 100)
        let normalized = JoinRoomInputNormalizer.normalize(longInput)
        XCTAssertEqual(
            normalized.count,
            64,
            "长度 > 64 必须截断到 64 字符（与 view body `.onChange` 同源规则）"
        )
        XCTAssertEqual(
            normalized,
            String(repeating: "X", count: 64),
            "截断后保留前 64 字符（prefix 语义）"
        )
    }

    /// case#N7 edge: 前后空白 + 超长 → 先 trim 再 prefix（"  " + 100×"X" + "  "
    /// → trim 拿 100×"X" → prefix(64) 拿 64×"X"，而不是先 prefix 再 trim 拿到 < 64 的尴尬）.
    func testNormalizeTrimBeforeTruncate() {
        let messy = "  " + String(repeating: "X", count: 100) + "  "
        let normalized = JoinRoomInputNormalizer.normalize(messy)
        XCTAssertEqual(
            normalized,
            String(repeating: "X", count: 64),
            "前后空白 + 超长输入必须先 trim 再 prefix —— 防 prefix(64) 后还残留 leading 空白"
        )
    }

    // MARK: - JoinRoomInputNormalizer.isSubmitDisabled(_:) — 驱动 view body confirm `.disabled(...)`

    /// case#D1 edge: 空字符串 → submit disabled（epic AC line 4863）.
    func testIsSubmitDisabledOnEmptyString() {
        XCTAssertTrue(
            JoinRoomInputNormalizer.isSubmitDisabled(""),
            "空字符串 → confirm button disabled"
        )
    }

    /// case#D2 edge: 全空白 → submit disabled（epic AC line 4864）.
    func testIsSubmitDisabledOnWhitespaceOnly() {
        XCTAssertTrue(
            JoinRoomInputNormalizer.isSubmitDisabled("     "),
            "仅空格输入 trim 后空 → confirm button disabled"
        )
    }

    /// case#D3 edge: 含换行 / Tab 的全空白 → submit disabled.
    func testIsSubmitDisabledOnNewlineAndTabOnly() {
        XCTAssertTrue(
            JoinRoomInputNormalizer.isSubmitDisabled("\n\t \n"),
            "全 whitespacesAndNewlines 字符 → confirm button disabled"
        )
    }

    /// case#D4 happy: 非空 trim 后非空 → submit enabled.
    func testIsSubmitDisabledFalseOnNonEmptyTrimmed() {
        XCTAssertFalse(
            JoinRoomInputNormalizer.isSubmitDisabled("1234567"),
            "非空 trim 后非空 → confirm button enabled"
        )
    }

    /// case#D5 happy: 含前后空白但 trim 后非空 → submit enabled（边界，防把 normalize 与 isEmpty 颠倒判断）.
    func testIsSubmitDisabledFalseOnPaddedNonEmpty() {
        XCTAssertFalse(
            JoinRoomInputNormalizer.isSubmitDisabled("  abc  "),
            "前后空白但 trim 后非空 → confirm button enabled"
        )
    }

    // MARK: - JoinRoomModal closure surrogate — 守护 closure 持有 + 调用语义

    /// case#M1 happy: JoinRoomModal struct 持 onConfirm closure，调用时正确传 String 参数.
    /// 守护：未来 Claude 误把 `onConfirm: (String) -> Void` 改成 `() -> Void` 不参数化 → 此 case fail.
    func testJoinRoomModalForwardsStringToOnConfirmClosure() {
        var captured: String?
        let modal = JoinRoomModal(
            roomIdInput: .constant("1234567"),
            onConfirm: { captured = $0 },
            onCancel: {}
        )
        modal.onConfirm("1234567")
        XCTAssertEqual(captured, "1234567", "JoinRoomModal.onConfirm 必须接受 String 参数 + 透传")
    }

    /// case#M2 happy: JoinRoomModal struct 持 onCancel closure，调用时无参 trigger.
    func testJoinRoomModalInvokesOnCancelClosure() {
        var cancelInvoked = false
        let modal = JoinRoomModal(
            roomIdInput: .constant(""),
            onConfirm: { _ in },
            onCancel: { cancelInvoked = true }
        )
        modal.onCancel()
        XCTAssertTrue(cancelInvoked, "JoinRoomModal.onCancel closure 必须可被 invoke")
    }

    // MARK: - HomeViewModel onJoinRoomConfirm 守护（保留原 case 6/7/8）

    /// case#H1 (旧 case#6) 守护: HomeViewModel onJoinRoomConfirm Mock override 行为（关 sheet + 记录 invocation）.
    func testMockHomeViewModelOnJoinRoomConfirmClosesSheetAndRecordsInvocation() {
        let vm = MockHomeViewModel()
        vm.showJoinModal = true   // 模拟 user 已点 "加入队伍" 触发 modal 显示

        vm.onJoinRoomConfirm(roomId: "1234567")
        XCTAssertFalse(vm.showJoinModal, "Mock onJoinRoomConfirm 必须关 sheet")
        XCTAssertTrue(
            vm.invocations.contains(.joinRoomConfirm(roomId: "1234567")),
            "Mock 必须记录 invocation 含 roomId"
        )
    }

    /// case#H2 (旧 case#7) 守护: RealHomeViewModel onJoinRoomConfirm 必走 appState.setCurrentRoomId 入口
    /// （lesson 6 + lesson 7 守护）.
    /// 防未来 Claude 重构时把 onJoinRoomConfirm 改成只 log（lesson 6 复犯）或绕过 appState 直接写
    /// self.currentRoomId（lesson 7 复犯）.
    /// lesson 6: 2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md
    /// lesson 7: View 不要绕过 ViewModel seam 直接 mutate state（本测试反向验证 ViewModel 必走 appState 入口）.
    func testRealHomeViewModelOnJoinRoomConfirmGoesThroughAppState() {
        let appState = AppState()
        let vm = RealHomeViewModel(appState: appState)
        vm.showJoinModal = true   // 模拟 modal 已弹起

        vm.onJoinRoomConfirm(roomId: "1234567")
        XCTAssertFalse(vm.showJoinModal, "Real onJoinRoomConfirm 必须关 sheet")
        XCTAssertEqual(
            appState.currentRoomId,
            "1234567",
            "Real onJoinRoomConfirm 必须通过 appState.setCurrentRoomId 写入 currentRoomId（守护 lesson 6 + 7）"
        )
    }

    /// case#H3 (旧 case#8) 守护: RealHomeViewModel onJoinRoomConfirm appState=nil 不 crash（防 launch-time race）.
    /// 防 launch-time race —— RealHomeViewModel parameterless init 走 appState=nil 路径,
    /// 用户在 bind(appState:) 之前点 "加入队伍" + 输入 + 确认 → 不应 crash, 仅 mutate showJoinModal + log.
    /// 与 RealHomeViewModel.onCreateTap 同精神（不依赖 self.appState）.
    func testRealHomeViewModelOnJoinRoomConfirmDoesNotCrashWithoutAppState() {
        let vm = RealHomeViewModel()   // parameterless init 路径
        vm.showJoinModal = true

        // 不 crash，仅 mutate showJoinModal.
        vm.onJoinRoomConfirm(roomId: "1234567")
        XCTAssertFalse(vm.showJoinModal, "appState=nil 时仍应关 sheet（防 race）")
        // appState 不可访问，无 currentRoomId 断言.
    }

    /// case#H4 (旧 case#9) 守护: FriendsView "加入" 入口不破（Story 37.10 落地路径回归）.
    /// 回归确认：FriendsView FriendRow "加入" 按钮路径在本 story 范围内**不动** ——
    /// epic AC line 4859 钦定 FriendsScreen 直接调 JoinRoomUseCase / appState.setCurrentRoomId（Story 37.10 落地占位）,
    /// **不**弹 modal. 本 story 仅守护该路径在 onJoinRoomConfirm abstract method 加入后**不破**.
    func testRealFriendsViewModelOnJoinFriendTapStillBypassesModal() {
        let appState = AppState()
        let vm = RealFriendsViewModel(appState: appState)
        let friend = Friend(
            id: "u1",
            name: "夏夏",
            online: true,
            status: .inRoom,
            statusText: "在房间",
            currentRoomId: "1234567"
        )

        vm.onJoinFriendTap(friend: friend)
        XCTAssertEqual(
            appState.currentRoomId,
            "1234567",
            "FriendsView 入口必须直接走 appState.setCurrentRoomId, 不弹 modal（epic AC line 4859）"
        )
    }
}

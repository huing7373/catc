// KeychainUITestHookView.swift
// Story 5.1 AC5: KeychainPersistenceUITests 配套 hook view（**仅 #if DEBUG**）。
//
// 流程契约（与 KeychainPersistenceUITests 对齐）：
// - launchEnvironment["KEYCHAIN_TEST_SEED"] = <some-uid>:
//     在 onAppear 时调 keychainStore.set(<some-uid>, forKey: KeychainKey.guestUid.rawValue)
//     **完成后**才渲染 `accessibilityIdentifier = "uitest_keychain_seed_done"` 的 hidden Text，
//     给 XCUIApplication 通过 staticTexts 探测"种入完成" 信号
// - launchEnvironment["KEYCHAIN_TEST_READBACK"] = "1":
//     在 onAppear 时读 keychainStore.get(forKey: KeychainKey.guestUid.rawValue)，
//     **拿到结果后**才渲染 `accessibilityIdentifier = "uitest_keychain_readback_value"` 的 hidden Text，
//     label 即读到的字符串；给 XCUIApplication 通过 staticTexts[...].label 比对断言
//
// 设计要点：
// - opacity(0.001)：视觉不可见但 accessibility tree 仍可见 —— XCUIApplication.staticTexts 能定位
// - **副作用完成 → flag 翻 true → Text 才进入 view tree**（codex round 1 [P2] fix）：
//   早期实装把 Text 永远渲染、`.onAppear` 异步执行 keychain；UITest 的 waitForExistence
//   只要看到 element 就视为完成，但此时 keychain.set/get 尚未返回 → 第一次 launch 可能在
//   seed 未真正写入前就 terminate；第二次 launch 可能在 readbackValue 仍是 ""（initial state）
//   时被读到 label。真机/busy simulator 上是间歇性 flake。
//   修复：把 Text 包在 `if seedDone` / `if readbackDone` 后面 —— UITest 看到 element 出现，
//   即等价于"keychain 副作用 returned"，断言时序才严格对齐。
//   详见 docs/lessons/2026-04-27-swiftui-uitest-marker-after-side-effect.md。
// - 用 @State 缓存 readback + done 标志：保证 body 重渲染时不重复触发副作用
// - hook 触发逻辑放在 .onAppear（而不是 .task）：UITest 等待 element 出现走的是 a11y 树轮询，
//   不依赖 .task 的并发时序；.onAppear 同步触发更确定
// - 不引入业务路径耦合：hook view 完全独立挂在 RootView ZStack 末尾，与 launchStateMachine /
//   coordinator / homeViewModel 全部无交叉

#if DEBUG

import SwiftUI

struct KeychainUITestHookView: View {
    let container: AppContainer

    /// 种入完成标志：true 时才把 seed-done Text 加入 view tree → a11y 暴露给 UITest。
    @State private var seedDone: Bool = false
    /// 读回完成标志：true 时才把 readback Text 加入 view tree（label = readbackValue）。
    @State private var readbackDone: Bool = false
    /// 实际读到的 keychain 值（or 空字符串若 keychain 没有此 key）。
    @State private var readbackValue: String = ""

    private var seedUid: String? {
        ProcessInfo.processInfo.environment["KEYCHAIN_TEST_SEED"]
    }

    private var shouldReadback: Bool {
        ProcessInfo.processInfo.environment["KEYCHAIN_TEST_READBACK"] == "1"
    }

    var body: some View {
        ZStack {
            // seed 触发器：占位 Color.clear 承接 .onAppear；副作用完成后翻 seedDone flag
            if let seedUid = seedUid {
                Color.clear
                    .frame(width: 0, height: 0)
                    .onAppear {
                        try? container.keychainStore.set(seedUid, forKey: KeychainKey.guestUid.rawValue)
                        // 同步 set 已 return → 翻 flag → 下一帧 if seedDone 分支生效，渲染 marker
                        seedDone = true
                    }
            }
            // seed marker：仅在 seedDone == true 时进入 view tree
            if seedDone {
                Text("seed-done")
                    .accessibilityIdentifier("uitest_keychain_seed_done")
                    .opacity(0.001)
            }

            // readback 触发器：占位 Color.clear 承接 .onAppear；读完后赋值并翻 readbackDone flag
            if shouldReadback {
                Color.clear
                    .frame(width: 0, height: 0)
                    .onAppear {
                        readbackValue = (try? container.keychainStore.get(forKey: KeychainKey.guestUid.rawValue)) ?? ""
                        readbackDone = true
                    }
            }
            // readback marker：仅在 readbackDone == true 时进入 view tree（label = 读到的值）
            if readbackDone {
                Text(readbackValue)
                    .accessibilityIdentifier("uitest_keychain_readback_value")
                    .opacity(0.001)
            }
        }
    }
}

#endif

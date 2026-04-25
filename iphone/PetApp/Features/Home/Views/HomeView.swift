// HomeView.swift
// Story 2.2 主界面骨架：6 大占位区块
//   ① 用户昵称 + 头像位（顶部）
//   ② 猫展示区（中间，屏幕中心区）
//   ③ 步数显示位（中间下方）
//   ④ 宝箱位（中间右侧）
//   ⑤ 三个主按钮（底部）：进入房间 / 仓库 / 合成
//   ⑥ 版本号小字（右下角）
//
// 后续 story 范围红线：
// - 不实装 Sheet / 路由（→ Story 2.3）
// - 不实装 APIClient 调用（→ Story 2.4 / 2.5；版本号目前 hardcode）
// - 不实装错误 UI（→ Story 2.6）

import SwiftUI

public struct HomeView: View {
    @ObservedObject public var viewModel: HomeViewModel

    public init(viewModel: HomeViewModel) {
        self.viewModel = viewModel
    }

    public var body: some View {
        // 单一 VStack：所有 6 区块都参与同一纵向布局，避免 ZStack overlay 在小屏上覆盖底部 CTA。
        // 版本号 (⑥) 作为最后一个子视图占据 footer 行，靠右对齐；它与 bottomButtonRow (⑤) 在
        // 垂直方向严格分隔，不会再遮挡或截获"合成"按钮的点击。
        VStack(spacing: 16) {
            userInfoBar
            Spacer()
            petAndChestRow
            stepBalanceLabel
            Spacer()
            bottomButtonRow
            versionFooter
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 24)
    }

    // MARK: - ① 用户昵称 + 头像位

    private var userInfoBar: some View {
        HStack(spacing: 8) {
            Text(viewModel.nickname)
            Circle()
                .fill(Color.gray)
                .frame(width: 32, height: 32)
            Spacer()
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text(viewModel.nickname))
        .accessibilityIdentifier(AccessibilityID.Home.userInfo)
    }

    // MARK: - ② 猫展示区 + ④ 宝箱位（横向同一行：中间 + 中间右侧）

    private var petAndChestRow: some View {
        HStack(alignment: .center, spacing: 16) {
            Spacer()
            petArea
            chestArea
            Spacer()
        }
    }

    private var petArea: some View {
        Rectangle()
            .fill(Color.gray)
            .frame(width: 200, height: 200)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(Text("猫展示区"))
            .accessibilityIdentifier(AccessibilityID.Home.petArea)
    }

    private var chestArea: some View {
        Rectangle()
            .fill(Color.brown)
            .frame(width: 64, height: 64)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(Text("宝箱"))
            .accessibilityIdentifier(AccessibilityID.Home.chestArea)
    }

    // MARK: - ③ 步数显示位

    private var stepBalanceLabel: some View {
        Text("0 步")
            .accessibilityIdentifier(AccessibilityID.Home.stepBalance)
    }

    // MARK: - ⑤ 三个主按钮

    private var bottomButtonRow: some View {
        HStack(spacing: 16) {
            Button("进入房间") {
                viewModel.onRoomTap()
            }
            .accessibilityIdentifier(AccessibilityID.Home.btnRoom)

            Button("仓库") {
                viewModel.onInventoryTap()
            }
            .accessibilityIdentifier(AccessibilityID.Home.btnInventory)

            Button("合成") {
                viewModel.onComposeTap()
            }
            .accessibilityIdentifier(AccessibilityID.Home.btnCompose)
        }
    }

    // MARK: - ⑥ 版本号小字（footer 行，靠右）

    private var versionFooter: some View {
        HStack {
            Spacer()
            versionLabel
        }
    }

    private var versionLabel: some View {
        Text("v\(viewModel.appVersion) · \(viewModel.serverInfo)")
            .font(.caption)
            .foregroundStyle(.secondary)
            .accessibilityIdentifier(AccessibilityID.Home.versionLabel)
    }
}

#if DEBUG
struct HomeView_Previews: PreviewProvider {
    static var previews: some View {
        HomeView(viewModel: HomeViewModel())
    }
}
#endif

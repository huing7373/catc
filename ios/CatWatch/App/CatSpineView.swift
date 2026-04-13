import SwiftUI
import SpineModule
import CatShared

/// 最小 Spine 播放骨架。
/// 接入真实资源后，只需要把 atlas / skeleton / animation 名称替换成项目实际值。
struct CatSpineView: View {
    let state: CatState

    @StateObject private var controller = SpineController(
        onInitialized: { controller in
            controller.animationState.setAnimationByName(
                trackIndex: 0,
                animationName: "idle",
                loop: true
            )
        }
    )

    var body: some View {
        SpineView(
            from: .bundle(
                atlasFileName: SpineAsset.atlasFileName,
                skeletonFileName: SpineAsset.skeletonFileName
            ),
            controller: controller,
            mode: .fit,
            alignment: .center
        )
        .onAppear {
            playCurrentState()
        }
        .onChange(of: state) { _, _ in
            playCurrentState()
        }
    }

    private func playCurrentState() {
        controller.animationState.setAnimationByName(
            trackIndex: 0,
            animationName: animationName(for: state),
            loop: true
        )
    }

    private func animationName(for state: CatState) -> String {
        switch state {
        case .idle, .microYawn, .microStretch:
            return "idle"
        case .walking:
            return "walk"
        case .running:
            return "run"
        case .sleeping:
            return "sleep"
        }
    }
}

private enum SpineAsset {
    /// 这里填 atlas 文件名，不带扩展名时再按实际 API 微调。
    static let atlasFileName = "cat.atlas"

    /// 这里填 skeleton 数据文件名，不带扩展名时再按实际 API 微调。
    static let skeletonFileName = "cat.skel"
}

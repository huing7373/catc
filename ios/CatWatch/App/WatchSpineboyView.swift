import SwiftUI
import SpriteKit
import CatShared
import Spine

struct SpineWatchCatView: View {
    let state: CatState
    @State private var scene = WatchSpineboyScene(size: CGSize(width: 320, height: 320))

    var body: some View {
        SpriteView(scene: scene)
            .background(Color.clear)
            .onAppear {
                scene.play(state: state)
            }
            .onChange(of: state) { _, newValue in
                scene.play(state: newValue)
            }
    }
}

#Preview("Spine Idle") {
    ZStack {
        Color.black.opacity(0.9).ignoresSafeArea()
        SpineWatchCatView(state: .idle)
            .frame(width: 140, height: 140)
    }
}

#Preview("Spine Walking") {
    ZStack {
        Color.black.opacity(0.9).ignoresSafeArea()
        SpineWatchCatView(state: .walking)
            .frame(width: 140, height: 140)
    }
}

#Preview("Spine Running") {
    ZStack {
        Color.black.opacity(0.9).ignoresSafeArea()
        SpineWatchCatView(state: .running)
            .frame(width: 140, height: 140)
    }
}

private final class WatchSpineboyScene: SKScene {
    private var skeleton: Skeleton?
    private let animationKey = "watch-spine-animation"
    private let debugFrameNodeName = "debug-frame-node"

    override init(size: CGSize) {
        super.init(size: size)
        scaleMode = .resizeFill
        backgroundColor = .clear
        anchorPoint = CGPoint(x: 0.5, y: 0.5)
        setupSkeletonIfNeeded()
        layoutSkeleton()
    }

    required init?(coder aDecoder: NSCoder) {
        fatalError("init(coder:) has not been implemented")
    }

    override func didChangeSize(_ oldSize: CGSize) {
        super.didChangeSize(oldSize)
        layoutSkeleton()
    }

    func play(state: CatState) {
        setupSkeletonIfNeeded()
        guard let skeleton else { return }

        skeleton.removeAction(forKey: animationKey)

        let animation = animationName(for: state)

        do {
            let action = try skeleton.action(animation: animation)
            skeleton.run(.repeatForever(action), withKey: animationKey)
            print("Watch spineboy animation started: \(animation)")
        } catch {
            print("Watch spineboy animation failed: \(animation), error: \(error)")
            return
        }
    }

    private func setupSkeletonIfNeeded() {
        guard skeleton == nil else { return }

        let atlas = SKTextureAtlas(named: "default")
        let textureNames = atlas.textureNames.sorted()
        print("Watch atlas default texture count: \(textureNames.count)")
        print("Watch atlas default sample textures: \(Array(textureNames.prefix(8)))")

        do {
            let skeleton = try Skeleton(json: "spineboy-ess", skin: "default")
            skeleton.setScale(0.14)
            self.skeleton = skeleton
            addChild(skeleton)
            print("Watch spineboy skeleton loaded")
            print("Watch spineboy child count: \(skeleton.children.count)")
            print("Watch spineboy frame after load: \(skeleton.calculateAccumulatedFrame())")
            installDebugFrame(for: skeleton)
        } catch {
            print("Watch spineboy load failed: \(error)")
        }
    }

    private func layoutSkeleton() {
        guard let skeleton else { return }
        skeleton.position = CGPoint(x: 0, y: -24)

        installDebugFrame(for: skeleton)
        print("Watch spineboy frame after layout: \(skeleton.calculateAccumulatedFrame())")
    }

    private func animationName(for state: CatState) -> String {
        switch state {
        case .running:
            return "run"
        case .walking:
            return "walk"
        case .sleeping, .idle, .microYawn, .microStretch:
            return "idle"
        }
    }

    private func installDebugFrame(for skeleton: Skeleton) {
        childNode(withName: debugFrameNodeName)?.removeFromParent()

        let frame = skeleton.calculateAccumulatedFrame()
        guard !frame.isNull, !frame.isInfinite, frame.width > 0, frame.height > 0 else {
            print("Watch spineboy debug frame skipped: \(frame)")
            return
        }

        let path = CGPath(rect: frame, transform: nil)
        let shape = SKShapeNode(path: path)
        shape.name = debugFrameNodeName
        shape.strokeColor = .green
        shape.lineWidth = 2
        shape.zPosition = 999
        addChild(shape)
    }
}

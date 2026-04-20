import SwiftUI
import UIKit
import CatShared

struct SpineWatchCatView: View {
    let state: CatState

    var body: some View {
        TimelineView(.periodic(from: .now, by: frameDuration(for: normalizedState))) { context in
            if let frameImage = DefaultCatAtlasStore.shared.frame(
                for: normalizedState,
                at: frameIndex(for: context.date, state: normalizedState)
            ) {
                Image(uiImage: frameImage)
                    .resizable()
                    .interpolation(.none)
                    .scaledToFit()
            } else {
                Color.clear
            }
        }
        .accessibilityHidden(true)
    }

    private var normalizedState: CatState {
        switch state {
        case .microYawn, .microStretch:
            return .idle
        default:
            return state
        }
    }

    private func frameDuration(for state: CatState) -> TimeInterval {
        switch state {
        case .idle:
            return 0.26
        case .walking:
            return 0.16
        case .running:
            return 0.10
        case .sleeping:
            return 0.38
        case .microYawn, .microStretch:
            return 0.26
        }
    }

    private func frameIndex(for date: Date, state: CatState) -> Int {
        let duration = frameDuration(for: state)
        let tick = Int(date.timeIntervalSinceReferenceDate / duration)
        return tick % 4
    }
}

#Preview("Frame Idle") {
    ZStack {
        Color.black.opacity(0.9).ignoresSafeArea()
        SpineWatchCatView(state: .idle)
            .frame(width: 140, height: 140)
    }
}

#Preview("Frame Walking") {
    ZStack {
        Color.black.opacity(0.9).ignoresSafeArea()
        SpineWatchCatView(state: .walking)
            .frame(width: 140, height: 140)
    }
}

#Preview("Frame Running") {
    ZStack {
        Color.black.opacity(0.9).ignoresSafeArea()
        SpineWatchCatView(state: .running)
            .frame(width: 140, height: 140)
    }
}

private final class DefaultCatAtlasStore {
    static let shared = DefaultCatAtlasStore()

    private var cachedFrames: [CatState: [UIImage]] = [:]

    func frame(for state: CatState, at index: Int) -> UIImage? {
        let frames = framesForState(state)
        guard frames.indices.contains(index) else { return nil }
        return frames[index]
    }

    private func framesForState(_ state: CatState) -> [UIImage] {
        if let cached = cachedFrames[state] {
            return cached
        }

        guard let atlas = loadAtlasImage(), let cgImage = atlas.cgImage else {
            return []
        }

        let row = rowIndex(for: state)
        let tileWidth = cgImage.width / 4
        let tileHeight = cgImage.height / 4
        let scale = atlas.scale

        let frames: [UIImage] = (0..<4).compactMap { column in
            let rect = CGRect(
                x: column * tileWidth,
                y: row * tileHeight,
                width: tileWidth,
                height: tileHeight
            )

            guard let frameCG = cgImage.cropping(to: rect) else { return nil }
            return UIImage(cgImage: frameCG, scale: scale, orientation: .up)
        }

        cachedFrames[state] = frames
        return frames
    }

    private func rowIndex(for state: CatState) -> Int {
        switch state {
        case .idle, .microYawn, .microStretch:
            return 0
        case .walking:
            return 1
        case .running:
            return 2
        case .sleeping:
            return 3
        }
    }

    private func loadAtlasImage() -> UIImage? {
        guard let url = Bundle.main.url(forResource: "default_cat_atlas", withExtension: "png"),
              let data = try? Data(contentsOf: url) else {
            return nil
        }

        return UIImage(data: data)
    }
}

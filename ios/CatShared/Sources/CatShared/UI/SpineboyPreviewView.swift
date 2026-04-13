import SwiftUI

public struct SpineboyPreviewView: View {
    public init() {}

    public var body: some View {
        ZStack {
            LinearGradient(
                colors: [
                    Color(red: 0.07, green: 0.08, blue: 0.12),
                    Color(red: 0.11, green: 0.14, blue: 0.18)
                ],
                startPoint: .topLeading,
                endPoint: .bottomTrailing
            )
            .ignoresSafeArea()

            VStack(spacing: 12) {
                Text("Spine Preview")
                    .font(.title2.bold())
                    .foregroundStyle(.white)

                Text("Watch 端试播已迁到 CatWatch target。")
                    .font(.footnote)
                    .foregroundStyle(.white.opacity(0.72))
            }
        }
    }
}

// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "CatShared",
    platforms: [
        .iOS(.v17),
        .watchOS(.v10),
        .macOS(.v14)
    ],
    products: [
        .library(name: "CatShared", targets: ["CatShared"]),
        .library(name: "CatCore", targets: ["CatCore"])
    ],
    targets: [
        .target(
            name: "CatShared",
            resources: [
                .process("Resources")
            ]
        ),
        .target(
            name: "CatCore",
            dependencies: ["CatShared"],
            path: "Sources/CatCore"
        ),
        .testTarget(
            name: "CatSharedTests",
            dependencies: ["CatShared"]
        ),
        .testTarget(
            name: "CatCoreTests",
            dependencies: ["CatCore", "CatShared"],
            path: "Tests/CatCoreTests"
        )
    ]
)

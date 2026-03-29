// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "CatShared",
    platforms: [
        .iOS(.v17),
        .watchOS(.v10)
    ],
    products: [
        .library(name: "CatShared", targets: ["CatShared"])
    ],
    targets: [
        .target(name: "CatShared"),
        .testTarget(name: "CatSharedTests", dependencies: ["CatShared"])
    ]
)

// swift-tools-version: 5.10
import PackageDescription

let package = Package(
    name: "portero",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "portero", targets: ["portero"]),
        .library(name: "PorteroCore", targets: ["PorteroCore"]),
    ],
    dependencies: [
        .package(url: "https://github.com/jpsim/Yams.git", from: "5.1.3"),
        .package(url: "https://github.com/apple/swift-argument-parser.git", from: "1.5.0"),
    ],
    targets: [
        .target(name: "PorteroCore", dependencies: ["Yams"]),
        .executableTarget(
            name: "portero",
            dependencies: [
                "PorteroCore",
                .product(name: "ArgumentParser", package: "swift-argument-parser"),
            ]),
        .testTarget(name: "PorteroCoreTests", dependencies: ["PorteroCore"]),
    ]
)

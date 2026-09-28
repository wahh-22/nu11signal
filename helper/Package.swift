// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "SoulKingHelper",
    platforms: [.macOS(.v14)],
    targets: [
        // Pure JSON-lines codec, free of MusicKit so it can be unit tested.
        .target(name: "SoulKingProtocol"),
        // The MusicKit-backed helper process bundled as SoulKingHelper.app.
        .executableTarget(
            name: "soulking-helper",
            dependencies: ["SoulKingProtocol"]
        ),
        .testTarget(
            name: "SoulKingProtocolTests",
            dependencies: ["SoulKingProtocol"]
        ),
    ]
)

// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "Nu11SignalHelper",
    platforms: [.macOS(.v14)],
    targets: [
        // Pure JSON-lines codec, free of MusicKit so it can be unit tested.
        .target(name: "Nu11SignalProtocol"),
        // The MusicKit-backed helper process bundled as Nu11SignalHelper.app.
        .executableTarget(
            name: "nu11signal-helper",
            dependencies: ["Nu11SignalProtocol"]
        ),
        .testTarget(
            name: "Nu11SignalProtocolTests",
            dependencies: ["Nu11SignalProtocol"]
        ),
    ]
)

// swift-tools-version:5.9
// Spike only (odd/tasks/local-queue.md Q2): not part of the helper build.
import PackageDescription

let package = Package(
    name: "LocalQueueSpike",
    platforms: [.macOS(.v14)],
    targets: [.executableTarget(name: "LocalQueueSpike")]
)

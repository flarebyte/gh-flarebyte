// swift-tools-version: 6.0
import PackageDescription

let package = Package(
  name: "SwiftFixture",
  products: [
    .library(name: "FixtureMath", targets: ["FixtureMath"]),
    .executable(name: "fixture-cli", targets: ["FixtureCLI"]),
  ],
  targets: [
    .target(name: "FixtureMath"),
    .executableTarget(name: "FixtureCLI", dependencies: ["FixtureMath"]),
    .testTarget(name: "FixtureMathTests", dependencies: ["FixtureMath"]),
  ]
)

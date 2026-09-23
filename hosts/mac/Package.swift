// swift-tools-version:6.0
// The Mac host: a window with the camera drawn by the system and the core running in a text
// view in front of it. Everything it agrees with the core is in docs/HOST.md.
import PackageDescription

let package = Package(
    name: "PoiesisHost",
    platforms: [.macOS(.v14)],
    dependencies: [
        .package(url: "https://github.com/migueldeicaza/SwiftTerm", exact: "1.20.0"),
    ],
    targets: [
        .executableTarget(
            name: "PoiesisHost",
            dependencies: [.product(name: "SwiftTerm", package: "SwiftTerm")]
        ),
    ],
    swiftLanguageModes: [.v5]
)

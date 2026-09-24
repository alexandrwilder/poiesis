import AppKit
import SwiftTerm

/// The window: the camera under the whole window, the core's text view in front of it with a
/// see-through ground, the title bar transparent with its three buttons.
final class HostApp: NSObject, NSApplicationDelegate, LocalProcessTerminalViewDelegate {
    private var window: NSWindow!
    private var text: LocalProcessTerminalView!
    private let camera = Camera()
    private var link: Link?

    func applicationDidFinishLaunching(_ note: Notification) {
        signal(SIGPIPE, SIG_IGN)
        let socket = Self.stateFolder().appendingPathComponent("host.sock").path
        let link = Link(path: socket)
        link.onMessage = { [weak self] message in self?.handle(message) }
        camera.send = { [weak link] message in link?.send(message) }
        do {
            try link.start()
            self.link = link
        } catch {
            NSLog("Poiesis host: no socket (\(error)); the core will run on its own")
        }
        makeWindow()
        startCore(socket: self.link == nil ? nil : socket)
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ app: NSApplication) -> Bool { true }

    func applicationShouldHandleReopen(_ app: NSApplication, hasVisibleWindows: Bool) -> Bool {
        window.makeKeyAndOrderFront(nil)
        NSApp.activate()
        return true
    }

    /// A poiesis:// link from anywhere on the Mac (docs/FORMAT.md). The core reads it from the
    /// command file, the way it reads the menu bar's, and decides what it may do.
    func application(_ application: NSApplication, open urls: [URL]) {
        for url in urls where url.scheme == "poiesis" {
            try? Data(url.absoluteString.utf8).write(to: Self.stateFolder().appendingPathComponent("command"))
        }
        window?.makeKeyAndOrderFront(nil)
        NSApp.activate()
    }

    func applicationWillTerminate(_ note: Notification) {
        camera.stopRecording()
        link?.stop()
    }

    private func makeWindow() {
        let frame = NSRect(x: 0, y: 0, width: 1100, height: 720)
        window = NSWindow(contentRect: frame,
                          styleMask: [.titled, .closable, .miniaturizable, .resizable, .fullSizeContentView],
                          backing: .buffered, defer: false)
        window.title = "Poiesis"
        window.titlebarAppearsTransparent = true
        window.titleVisibility = .hidden
        window.backgroundColor = .black
        window.center()
        window.setFrameAutosaveName("Poiesis")

        let content = NSView(frame: frame)
        content.wantsLayer = true
        content.layerUsesCoreImageFilters = true // the look is a filter on the camera layer
        content.layer?.backgroundColor = NSColor.black.cgColor
        window.contentView = content
        camera.display.frame = content.bounds
        camera.display.autoresizingMask = [.layerWidthSizable, .layerHeightSizable]
        content.layer?.addSublayer(camera.display)

        text = LocalProcessTerminalView(frame: window.contentLayoutRect)
        text.autoresizingMask = [.width, .height]
        text.processDelegate = self
        text.font = NSFont.monospacedSystemFont(ofSize: 13, weight: .regular)
        text.nativeForegroundColor = NSColor(srgbRed: 0xE6 / 255.0, green: 0xE4 / 255.0, blue: 0xDC / 255.0, alpha: 1)
        text.nativeBackgroundColor = .black
        text.backgroundOpacity = 0 // the camera shows wherever the core draws no background
        content.addSubview(text)

        window.makeKeyAndOrderFront(nil)
        window.makeFirstResponder(text)
        NSApp.activate()
    }

    /// The core runs in the text view with a whole environment (docs/HOST.md). `--core <path>`
    /// and arguments after `--` replace it, for the contract test.
    private func startCore(socket: String?) {
        let here = Bundle.main.executableURL!.deletingLastPathComponent()
        var core = here.appendingPathComponent("poiesis").path
        var args = ["ui"]
        let argv = CommandLine.arguments
        if let i = argv.firstIndex(of: "--core"), i + 1 < argv.count {
            core = argv[i + 1]
            args = argv.firstIndex(of: "--").map { Array(argv[($0 + 1)...]) } ?? []
        }
        let system = ProcessInfo.processInfo.environment
        let tools = here.deletingLastPathComponent().appendingPathComponent("Frameworks/tools").path
        var env = [
            "PATH=\(tools):/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
            "TERM=xterm-256color",
            "COLORTERM=truecolor",
            "TERM_PROGRAM=PoiesisHost",
            "LANG=\(system["LANG"] ?? "en_US.UTF-8")",
        ]
        for key in ["HOME", "USER", "LOGNAME", "TMPDIR", "SHELL"] {
            if let value = system[key] { env.append("\(key)=\(value)") }
        }
        if let socket { env.append("POIESIS_HOST=\(socket)") }
        text.startProcess(executable: core, args: args, environment: env, execName: "poiesis")
    }

    // MARK: the contract

    private func handle(_ message: [String: Any]) {
        switch message["t"] as? String {
        case "hello":
            let versions = message["versions"] as? [Int] ?? []
            link?.send(["t": "hello", "version": versions.contains(1) ? 1 : 0, "host": "poiesis-mac 0.1",
                        "can": ["picture", "look", "record", "level"]])
        case "picture":
            camera.setPicture(on: message["on"] as? Bool ?? false,
                              matrix: message["matrix"] as? [Double],
                              fps: message["fps"] as? Int ?? 15)
        case "record":
            switch message["action"] as? String {
            case "start":
                if let file = message["file"] as? String { camera.startRecording(to: URL(fileURLWithPath: file)) }
            case "stop":
                camera.stopRecording()
            default:
                break
            }
        default:
            break // a message from a later version: ignored
        }
    }

    // MARK: the text view

    func sizeChanged(source: LocalProcessTerminalView, newCols: Int, newRows: Int) {}
    func setTerminalTitle(source: LocalProcessTerminalView, title: String) {}
    func hostCurrentDirectoryUpdate(source: TerminalView, directory: String?) {}
    func processTerminated(source: TerminalView, exitCode: Int32?) {
        NSApp.terminate(nil) // the core ended: so does the window
    }

    static func stateFolder() -> URL {
        let url = FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent("Library/Application Support/Poiesis")
        try? FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        return url
    }
}

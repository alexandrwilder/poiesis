import AppKit

// The Mac host (docs/ARCHITECTURE.md): a regular app with a Dock icon that owns the window,
// the camera and the recording, and runs the core in a text view.
let app = NSApplication.shared
app.setActivationPolicy(.regular)
let host = HostApp()
app.delegate = host
app.run()

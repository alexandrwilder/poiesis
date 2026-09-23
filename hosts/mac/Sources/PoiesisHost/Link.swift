import Foundation

/// The host's side of the socket in docs/HOST.md: one client, one JSON object per line.
/// Messages from the core arrive on the main queue; send may be called from any thread.
final class Link {
    let path: String
    var onMessage: ([String: Any]) -> Void = { _ in }

    private var listenFD: Int32 = -1
    private var clientFD: Int32 = -1
    private let lock = NSLock()

    init(path: String) { self.path = path }

    /// Opens the socket, readable and writable only by this user.
    func start() throws {
        unlink(path)
        listenFD = socket(AF_UNIX, SOCK_STREAM, 0)
        guard listenFD >= 0 else { throw LinkError.socket(errno) }
        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)
        let capacity = MemoryLayout.size(ofValue: addr.sun_path)
        guard path.utf8.count < capacity else { throw LinkError.pathTooLong }
        withUnsafeMutablePointer(to: &addr.sun_path) { p in
            p.withMemoryRebound(to: CChar.self, capacity: capacity) { _ = strncpy($0, path, capacity - 1) }
        }
        let bound = withUnsafePointer(to: &addr) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) { bind(listenFD, $0, socklen_t(MemoryLayout<sockaddr_un>.size)) }
        }
        guard bound == 0 else { throw LinkError.bind(errno) }
        chmod(path, 0o600)
        guard listen(listenFD, 1) == 0 else { throw LinkError.listen(errno) }
        Thread.detachNewThread { [weak self] in self?.acceptLoop() }
    }

    func stop() {
        close(listenFD)
        unlink(path)
    }

    func send(_ message: [String: Any]) {
        guard var bytes = try? JSONSerialization.data(withJSONObject: message) else { return }
        bytes.append(0x0A)
        lock.lock()
        defer { lock.unlock() }
        guard clientFD >= 0 else { return }
        bytes.withUnsafeBytes { raw in
            var sent = 0
            while sent < raw.count {
                let n = write(clientFD, raw.baseAddress! + sent, raw.count - sent)
                if n <= 0 { return }
                sent += n
            }
        }
    }

    private func acceptLoop() {
        while true {
            let fd = accept(listenFD, nil, nil)
            if fd < 0 { return }
            var one: Int32 = 1
            setsockopt(fd, SOL_SOCKET, SO_NOSIGPIPE, &one, socklen_t(MemoryLayout<Int32>.size))
            lock.lock(); clientFD = fd; lock.unlock()
            readLoop(fd)
            lock.lock(); if clientFD == fd { clientFD = -1 }; lock.unlock()
            close(fd)
        }
    }

    private func readLoop(_ fd: Int32) {
        var pending = Data()
        var chunk = [UInt8](repeating: 0, count: 4096)
        while true {
            let n = read(fd, &chunk, chunk.count)
            if n <= 0 { return }
            pending.append(chunk, count: n)
            while let newline = pending.firstIndex(of: 0x0A) {
                let line = pending.subdata(in: pending.startIndex..<newline)
                pending.removeSubrange(pending.startIndex...newline)
                // unknown or garbled lines are ignored, as the contract says
                if let message = try? JSONSerialization.jsonObject(with: line) as? [String: Any] {
                    DispatchQueue.main.async { self.onMessage(message) }
                }
            }
        }
    }
}

enum LinkError: Error {
    case socket(Int32), bind(Int32), listen(Int32), pathTooLong
}

import Foundation
import PorteroCore

/// Opens a link in a browser. Per-browser profile selection lands in the next step;
/// for now every target gets the plain `open -a <browser> <url>`.
enum Launcher {
    static func open(_ url: URL, in target: Target) throws {
        let p = Process()
        p.executableURL = URL(fileURLWithPath: "/usr/bin/open")
        p.arguments = ["-a", target.browser, url.absoluteString]
        try p.run()
        p.waitUntilExit()
        if p.terminationStatus != 0 {
            throw LaunchError.failed(browser: target.browser, status: p.terminationStatus)
        }
    }
}

enum LaunchError: Error, CustomStringConvertible {
    case failed(browser: String, status: Int32)
    var description: String {
        switch self {
        case .failed(let b, let s): return "open -a \(b) exited with \(s)"
        }
    }
}

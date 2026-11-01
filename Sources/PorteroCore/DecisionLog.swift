import Foundation

/// Appends one line per routed link: time, opener, rule, target, url.
public enum DecisionLog {
    public static func append(_ decision: Decision, for request: Request, to file: URL = Config.logFile) {
        let stamp = ISO8601DateFormatter().string(from: Date())
        let target = decision.target.profile.map { "\(decision.target.browser)/\($0)" } ?? decision.target.browser
        let line = "\(stamp)\t\(request.opener ?? "-")\t\(decision.rule ?? "default")\t\(target)\t\(request.url.absoluteString)\n"
        let fm = FileManager.default
        try? fm.createDirectory(at: file.deletingLastPathComponent(), withIntermediateDirectories: true)
        if !fm.fileExists(atPath: file.path) {
            fm.createFile(atPath: file.path, contents: nil, attributes: [.posixPermissions: 0o600])
        }
        if let h = try? FileHandle(forWritingTo: file) {
            defer { try? h.close() }
            _ = try? h.seekToEnd()
            try? h.write(contentsOf: Data(line.utf8))
        }
    }
}

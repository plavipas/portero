import Foundation

/// The result of routing one link.
public struct Decision: Equatable, Sendable {
    public var target: Target
    /// Name or index of the matched rule; nil when the default applied.
    public var rule: String?

    public init(target: Target, rule: String?) {
        self.target = target
        self.rule = rule
    }
}

public struct Request: Sendable {
    public var url: URL
    public var opener: String?

    public init(url: URL, opener: String? = nil) {
        self.url = url
        self.opener = opener
    }
}

public enum Matcher {
    /// First matching rule wins; otherwise the default target.
    public static func route(_ request: Request, with rules: Rules) -> Decision {
        for (i, rule) in rules.rules.enumerated() where matches(rule.match, request) {
            return Decision(target: rule.open, rule: rule.name ?? "#\(i + 1)")
        }
        return Decision(target: rules.default, rule: nil)
    }

    static func matches(_ m: Match, _ r: Request) -> Bool {
        if let hosts = m.host {
            guard let host = r.url.host?.lowercased(),
                  hosts.values.contains(where: { hostMatches(pattern: $0.lowercased(), host: host) })
            else { return false }
        }
        if let paths = m.path {
            let path = r.url.path.isEmpty ? "/" : r.url.path
            guard paths.values.contains(where: { pathMatches(prefix: $0, path: path) }) else { return false }
        }
        if let openers = m.opener {
            guard let opener = r.opener?.lowercased(),
                  openers.values.contains(where: { $0.lowercased() == opener })
            else { return false }
        }
        return true
    }

    /// `example.com` exact; `*.example.com` any subdomain and the bare domain; other `*` are globs.
    static func hostMatches(pattern: String, host: String) -> Bool {
        if pattern.hasPrefix("*.") {
            let base = String(pattern.dropFirst(2))
            if !base.contains("*") {
                return host == base || host.hasSuffix("." + base)
            }
        }
        if !pattern.contains("*") { return host == pattern }
        return glob(pattern, matches: host)
    }

    /// Prefix on path segments: `/org` matches `/org` and `/org/x`, not `/organic`.
    static func pathMatches(prefix: String, path: String) -> Bool {
        var p = prefix
        if p.hasSuffix("*") { p.removeLast() }
        if p.hasSuffix("/") { p.removeLast() }
        if p.isEmpty { return true }
        return path == p || path.hasPrefix(p + "/")
    }

    static func glob(_ pattern: String, matches text: String) -> Bool {
        let p = Array(pattern), t = Array(text)
        var pi = 0, ti = 0, star = -1, mark = 0
        while ti < t.count {
            if pi < p.count, p[pi] == t[ti] { pi += 1; ti += 1 }
            else if pi < p.count, p[pi] == "*" { star = pi; mark = ti; pi += 1 }
            else if star >= 0 { pi = star + 1; mark += 1; ti = mark }
            else { return false }
        }
        while pi < p.count, p[pi] == "*" { pi += 1 }
        return pi == p.count
    }
}

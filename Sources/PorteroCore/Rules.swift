import Foundation
import Yams

/// Where a link should open.
public struct Target: Codable, Equatable, Sendable {
    /// Browser app name (as in /Applications) or bundle id.
    public var browser: String
    /// Profile, space or container name; nil = whatever the browser is on.
    public var profile: String?

    public init(browser: String, profile: String? = nil) {
        self.browser = browser
        self.profile = profile
    }
}

/// One or many strings in YAML: `host: a.com` or `host: [a.com, b.com]`.
public struct OneOrMany: Codable, Equatable, Sendable {
    public var values: [String]

    public init(_ values: [String]) { self.values = values }

    public init(from decoder: Decoder) throws {
        let c = try decoder.singleValueContainer()
        if let one = try? c.decode(String.self) {
            values = [one]
        } else {
            values = try c.decode([String].self)
        }
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.singleValueContainer()
        if values.count == 1 { try c.encode(values[0]) } else { try c.encode(values) }
    }
}

/// Conditions of a rule; every present key must match.
public struct Match: Codable, Equatable, Sendable {
    /// Host patterns: exact (`example.com`), any subdomain (`*.example.com`, also matches the
    /// bare domain), or a glob with `*` anywhere.
    public var host: OneOrMany?
    /// Path prefixes: `/org` matches `/org` and `/org/...`; a trailing `*` is accepted and ignored.
    public var path: OneOrMany?
    /// Bundle ids of the app that opened the link (`com.tinyspeck.slackmacgap`).
    public var opener: OneOrMany?

    public init(host: [String]? = nil, path: [String]? = nil, opener: [String]? = nil) {
        self.host = host.map(OneOrMany.init)
        self.path = path.map(OneOrMany.init)
        self.opener = opener.map(OneOrMany.init)
    }
}

public struct Rule: Codable, Equatable, Sendable {
    public var name: String?
    public var match: Match
    public var open: Target

    public init(name: String? = nil, match: Match, open: Target) {
        self.name = name
        self.match = match
        self.open = open
    }
}

/// The rules file.
public struct Rules: Codable, Equatable, Sendable {
    public var version: Int
    public var `default`: Target
    public var rules: [Rule]

    public init(version: Int = 1, default: Target, rules: [Rule]) {
        self.version = version
        self.default = `default`
        self.rules = rules
    }

    public static let currentVersion = 1

    public static func parse(_ yaml: String) throws -> Rules {
        let rules = try YAMLDecoder().decode(Rules.self, from: yaml)
        try rules.validate()
        return rules
    }

    public static func load(from url: URL) throws -> Rules {
        let text = try String(contentsOf: url, encoding: .utf8)
        return try parse(text)
    }

    public func validate() throws {
        guard version == Rules.currentVersion else {
            throw RulesError.unsupportedVersion(version)
        }
        for (i, rule) in rules.enumerated() {
            if rule.match.host == nil && rule.match.path == nil && rule.match.opener == nil {
                throw RulesError.emptyMatch(index: i, name: rule.name)
            }
            if rule.open.browser.isEmpty {
                throw RulesError.emptyBrowser(index: i, name: rule.name)
            }
        }
    }
}

public enum RulesError: Error, CustomStringConvertible, Equatable {
    case unsupportedVersion(Int)
    case emptyMatch(index: Int, name: String?)
    case emptyBrowser(index: Int, name: String?)

    public var description: String {
        switch self {
        case .unsupportedVersion(let v):
            return "rules file version \(v) is not supported (expected \(Rules.currentVersion))"
        case .emptyMatch(let i, let name):
            return "rule \(label(i, name)) has no match conditions (host, path or opener)"
        case .emptyBrowser(let i, let name):
            return "rule \(label(i, name)) has an empty browser"
        }
    }

    private func label(_ i: Int, _ name: String?) -> String {
        name.map { "\"\($0)\"" } ?? "#\(i + 1)"
    }
}

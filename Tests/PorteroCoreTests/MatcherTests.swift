import Foundation
import Testing
@testable import PorteroCore

let sample = """
version: 1
default:
  browser: Dia
  profile: Personal
rules:
  - name: company forge
    match:
      host: [gitlab.example.com, "*.example.net"]
    open: { browser: Zen }
  - name: client A on GitHub
    match:
      host: github.com
      path: /client-a
    open: { browser: Dia, profile: Client A }
  - name: work Slack links
    match:
      opener: com.tinyspeck.slackmacgap
    open: { browser: Dia, profile: Work }
  - name: wildcard host
    match:
      host: "docs-*.example.org"
    open: { browser: Safari }
"""

func route(_ s: String, opener: String? = nil) throws -> Decision {
    let rules = try Rules.parse(sample)
    return Matcher.route(Request(url: URL(string: s)!, opener: opener), with: rules)
}

@Test func parsesAndValidates() throws {
    let r = try Rules.parse(sample)
    #expect(r.rules.count == 4)
    #expect(r.rules[0].match.host?.values == ["gitlab.example.com", "*.example.net"])
    #expect(r.default == Target(browser: "Dia", profile: "Personal"))
}

@Test func exactAndSubdomainHosts() throws {
    #expect(try route("https://gitlab.example.com/x").target.browser == "Zen")
    #expect(try route("https://jira.example.net/").target.browser == "Zen")
    #expect(try route("https://example.net/").target.browser == "Zen")
    #expect(try route("https://notexample.net/").rule == nil)
    #expect(try route("https://GITLAB.EXAMPLE.COM/").target.browser == "Zen")
}

@Test func pathPrefixIsSegmentBased() throws {
    #expect(try route("https://github.com/client-a/repo").target.profile == "Client A")
    #expect(try route("https://github.com/client-a").target.profile == "Client A")
    #expect(try route("https://github.com/client-ab/repo").rule == nil)
    #expect(try route("https://github.com/other").target == Target(browser: "Dia", profile: "Personal"))
}

@Test func openerMatchesAndDefaultApplies() throws {
    #expect(try route("https://anything.io/", opener: "com.tinyspeck.slackmacgap").target.profile == "Work")
    #expect(try route("https://anything.io/", opener: "com.apple.mail").rule == nil)
    let d = try route("https://anything.io/")
    #expect(d.rule == nil && d.target.browser == "Dia")
}

@Test func globHosts() throws {
    #expect(try route("https://docs-v2.example.org/").target.browser == "Safari")
    #expect(try route("https://docs.example.org/").rule == nil)
}

@Test func validationErrors() {
    #expect(throws: RulesError.unsupportedVersion(2)) {
        try Rules.parse("version: 2\ndefault: {browser: Dia}\nrules: []")
    }
    #expect(throws: RulesError.emptyMatch(index: 0, name: "x")) {
        try Rules.parse("version: 1\ndefault: {browser: Dia}\nrules:\n  - name: x\n    match: {}\n    open: {browser: Dia}")
    }
}

@Test func pathHelpers() {
    #expect(Matcher.pathMatches(prefix: "/org/*", path: "/org/repo"))
    #expect(Matcher.pathMatches(prefix: "/", path: "/anything"))
    #expect(!Matcher.pathMatches(prefix: "/org", path: "/organic"))
}

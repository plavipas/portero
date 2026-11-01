import ArgumentParser
import Foundation
import PorteroCore

@main
struct Portero: ParsableCommand {
    static let configuration = CommandConfiguration(
        abstract: "Default-browser router for macOS.",
        subcommands: [Open.self, RulesCommand.self],
        defaultSubcommand: nil)
}

struct Open: ParsableCommand {
    static let configuration = CommandConfiguration(abstract: "Route a link to the browser and profile its rule picks.")

    @Argument(help: "The link to open.") var url: String
    @Option(help: "Bundle id of the app that opened the link.") var opener: String?
    @Flag(help: "Print the decision instead of opening the link.") var dryRun = false

    func run() throws {
        guard let u = URL(string: url), u.host != nil else { throw ValidationError("not a URL: \(url)") }
        let rules = try Rules.load(from: Config.rulesFile())
        let request = Request(url: u, opener: opener)
        let decision = Matcher.route(request, with: rules)
        print(describe(decision))
        if dryRun { return }
        DecisionLog.append(decision, for: request)
        try Launcher.open(u, in: decision.target)
    }
}

struct RulesCommand: ParsableCommand {
    static let configuration = CommandConfiguration(
        commandName: "rules", abstract: "Show, check or try the rules file.",
        subcommands: [Path.self, Check.self, Match.self], defaultSubcommand: Path.self)

    struct Path: ParsableCommand {
        static let configuration = CommandConfiguration(abstract: "Print the path of the rules file in use.")
        func run() { print(Config.rulesFile().path) }
    }

    struct Check: ParsableCommand {
        static let configuration = CommandConfiguration(abstract: "Parse and validate the rules file.")
        func run() throws {
            let file = Config.rulesFile()
            let rules = try Rules.load(from: file)
            print("\(file.path): \(rules.rules.count) rule(s), default \(rules.default.browser)\(rules.default.profile.map { "/" + $0 } ?? "")")
        }
    }

    struct Match: ParsableCommand {
        static let configuration = CommandConfiguration(abstract: "Show which rule a link would hit.")
        @Argument var url: String
        @Option var opener: String?
        func run() throws {
            guard let u = URL(string: url), u.host != nil else { throw ValidationError("not a URL: \(url)") }
            let rules = try Rules.load(from: Config.rulesFile())
            print(describe(Matcher.route(Request(url: u, opener: opener), with: rules)))
        }
    }
}

func describe(_ d: Decision) -> String {
    let target = d.target.profile.map { "\(d.target.browser) / \($0)" } ?? d.target.browser
    return "\(target)  (rule: \(d.rule ?? "default"))"
}

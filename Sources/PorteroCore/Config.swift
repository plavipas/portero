import Foundation

/// Where the rules file is: `$PORTERO_RULES`, else `rules:` in `~/.config/portero/config.yaml`,
/// else `~/.config/portero/rules.yaml`. `portero sync` points the config at a copy in iCloud Drive.
public enum Config {
    public static var configDir: URL {
        let base = ProcessInfo.processInfo.environment["XDG_CONFIG_HOME"].map(URL.init(fileURLWithPath:))
            ?? FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent(".config")
        return base.appendingPathComponent("portero")
    }

    public static var configFile: URL { configDir.appendingPathComponent("config.yaml") }
    public static var defaultRulesFile: URL { configDir.appendingPathComponent("rules.yaml") }
    public static var logFile: URL { configDir.appendingPathComponent("decisions.log") }

    public static var iCloudDriveDir: URL {
        FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/Mobile Documents/com~apple~CloudDocs/Portero")
    }

    public static func rulesFile(environment: [String: String] = ProcessInfo.processInfo.environment) -> URL {
        if let p = environment["PORTERO_RULES"], !p.isEmpty {
            return URL(fileURLWithPath: (p as NSString).expandingTildeInPath)
        }
        if let text = try? String(contentsOf: configFile, encoding: .utf8) {
            for line in text.split(separator: "\n") {
                let parts = line.split(separator: ":", maxSplits: 1).map { $0.trimmingCharacters(in: .whitespaces) }
                if parts.count == 2, parts[0] == "rules", !parts[1].isEmpty {
                    return URL(fileURLWithPath: (parts[1] as NSString).expandingTildeInPath)
                }
            }
        }
        return defaultRulesFile
    }
}

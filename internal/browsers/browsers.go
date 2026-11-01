// Package browsers knows the installed browsers: how each one is launched and where its profiles live.
package browsers

import "strings"

// Kind says how a browser takes a URL and a profile.
type Kind string

const (
	// Plain: `open -a <app> <url>`, no profile selection.
	Plain Kind = "plain"
	// Chromium: `open -b <bundle> -n --args --profile-directory=<dir> <url>`; the directory comes from Local State.
	Chromium Kind = "chromium"
	// Firefox: `open -a <app> -n --args -P <profile> <url>`; names come from profiles.ini.
	Firefox Kind = "firefox"
	// Dia: single-instance browser, driven through its AppleScript dictionary (window > profile > tab).
	Dia Kind = "dia"
	// Arc: single-instance browser, AppleScript dictionary (window > space > tab).
	Arc Kind = "arc"
)

// Browser is one entry of the table.
type Browser struct {
	Name     string // app name as in /Applications
	BundleID string
	Kind     Kind
	// ConfigDir is relative to ~/Library/Application Support: the folder with Local State (Chromium)
	// or profiles.ini (Firefox).
	ConfigDir string
}

// Known is the table of browsers Portero understands. Anything else is launched Plain by name.
var Known = []Browser{
	{Name: "Dia", BundleID: "company.thebrowser.dia", Kind: Dia},
	{Name: "Arc", BundleID: "company.thebrowser.Browser", Kind: Arc},
	{Name: "Google Chrome", BundleID: "com.google.Chrome", Kind: Chromium, ConfigDir: "Google/Chrome"},
	{Name: "Google Chrome Beta", BundleID: "com.google.Chrome.beta", Kind: Chromium, ConfigDir: "Google/Chrome Beta"},
	{Name: "Google Chrome Canary", BundleID: "com.google.Chrome.canary", Kind: Chromium, ConfigDir: "Google/Chrome Canary"},
	{Name: "Chromium", BundleID: "org.chromium.Chromium", Kind: Chromium, ConfigDir: "Chromium"},
	{Name: "Brave Browser", BundleID: "com.brave.Browser", Kind: Chromium, ConfigDir: "BraveSoftware/Brave-Browser"},
	{Name: "Microsoft Edge", BundleID: "com.microsoft.edgemac", Kind: Chromium, ConfigDir: "Microsoft Edge"},
	{Name: "Vivaldi", BundleID: "com.vivaldi.Vivaldi", Kind: Chromium, ConfigDir: "Vivaldi"},
	{Name: "Opera", BundleID: "com.operasoftware.Opera", Kind: Chromium, ConfigDir: "com.operasoftware.Opera"},
	{Name: "Comet", BundleID: "ai.perplexity.comet", Kind: Chromium, ConfigDir: "Perplexity/Comet"},
	{Name: "Firefox", BundleID: "org.mozilla.firefox", Kind: Firefox, ConfigDir: "Firefox"},
	{Name: "Firefox Developer Edition", BundleID: "org.mozilla.firefoxdeveloperedition", Kind: Firefox, ConfigDir: "Firefox"},
	{Name: "Zen", BundleID: "app.zen-browser.zen", Kind: Firefox, ConfigDir: "zen"},
	{Name: "Safari", BundleID: "com.apple.Safari", Kind: Plain},
	{Name: "Orion", BundleID: "com.kagi.kagimacOS", Kind: Plain},
}

// Aliases map short names people write in rules to table names.
var Aliases = map[string]string{
	"chrome": "Google Chrome",
	"brave":  "Brave Browser",
	"edge":   "Microsoft Edge",
}

// Lookup finds a browser by app name, alias or bundle id (case-insensitive).
// An unknown name is returned as a Plain browser with that name, ok = false.
func Lookup(s string) (Browser, bool) {
	key := strings.ToLower(strings.TrimSpace(s))
	if a, ok := Aliases[key]; ok {
		key = strings.ToLower(a)
	}
	for _, b := range Known {
		if strings.ToLower(b.Name) == key || strings.ToLower(b.BundleID) == key {
			return b, true
		}
	}
	return Browser{Name: strings.TrimSpace(s), Kind: Plain}, false
}

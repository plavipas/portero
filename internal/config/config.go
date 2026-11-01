// Package config resolves the files Portero uses: rules, config and the decision log.
package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Dir is `$XDG_CONFIG_HOME/portero`, by default `~/.config/portero`.
func Dir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "portero")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "portero")
}

// File returns the config file `config.yaml` inside Dir.
func File() string { return filepath.Join(Dir(), "config.yaml") }

// LogFile is the decision log inside Dir.
func LogFile() string { return filepath.Join(Dir(), "decisions.log") }

// RulesFile is `$PORTERO_RULES`, else `rules: <path>` from the config file, else `rules.yaml` in Dir.
// `portero sync` points the config at a copy in a synced folder.
func RulesFile() string {
	if p := os.Getenv("PORTERO_RULES"); p != "" {
		return expand(p)
	}
	if p := rulesFromConfig(File()); p != "" {
		return expand(p)
	}
	return filepath.Join(Dir(), "rules.yaml")
}

func rulesFromConfig(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if ok && strings.TrimSpace(k) == "rules" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func expand(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

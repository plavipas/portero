package browsers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Profile is one browser profile: the name people see and the directory or id the browser wants.
type Profile struct {
	Name string
	Dir  string
	// Default is true for the Firefox profile marked Default=1 in profiles.ini.
	Default bool
	// InstallDefault is true when an [Install...] section of profiles.ini names this profile as Default=.
	InstallDefault bool
}

// ChromiumProfiles reads `<localState>` (the Local State JSON) and returns name -> directory.
func ChromiumProfiles(localState string) ([]Profile, error) {
	raw, err := os.ReadFile(localState)
	if err != nil {
		return nil, err
	}
	var state struct {
		Profile struct {
			InfoCache map[string]struct {
				Name string `json:"name"`
			} `json:"info_cache"`
		} `json:"profile"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("%s: %w", localState, err)
	}
	var out []Profile
	for dir, p := range state.Profile.InfoCache {
		out = append(out, Profile{Name: p.Name, Dir: dir})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dir < out[j].Dir })
	return out, nil
}

// FirefoxProfiles reads profiles.ini and returns the profile names (Dir is the Path= value).
func FirefoxProfiles(profilesIni string) ([]Profile, error) {
	f, err := os.Open(profilesIni)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Profile
	var cur *Profile
	var installs []string
	inInstall := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "[Profile"):
			inInstall = false
			out = append(out, Profile{})
			cur = &out[len(out)-1]
		case strings.HasPrefix(line, "["):
			cur = nil
			inInstall = strings.HasPrefix(line, "[Install")
		case inInstall:
			if k, v, ok := strings.Cut(line, "="); ok && k == "Default" && v != "" {
				installs = append(installs, v)
			}
		case cur != nil:
			if k, v, ok := strings.Cut(line, "="); ok {
				switch k {
				case "Name":
					cur.Name = v
				case "Path":
					cur.Dir = v
				case "Default":
					cur.Default = v == "1"
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	base := filepath.Dir(profilesIni)
	for i := range out {
		abs := out[i].Dir
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(base, abs)
		}
		for _, d := range installs {
			if d == out[i].Dir || d == abs {
				out[i].InstallDefault = true
			}
		}
	}
	return out, nil
}

// Resolve picks the profile whose Name matches (case-insensitive), else whose Dir matches exactly.
func Resolve(profiles []Profile, want string) (Profile, bool) {
	for _, p := range profiles {
		if strings.EqualFold(p.Name, want) {
			return p, true
		}
	}
	for _, p := range profiles {
		if p.Dir == want {
			return p, true
		}
	}
	return Profile{}, false
}

// Names lists the profile names for messages.
func Names(profiles []Profile) string {
	var n []string
	for _, p := range profiles {
		n = append(n, p.Name)
	}
	return strings.Join(n, ", ")
}

// SupportDir is ~/Library/Application Support/<ConfigDir> for b.
func (b Browser) SupportDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", b.ConfigDir)
}

// Profiles lists the profiles of a Chromium or Firefox browser from its files on this Mac.
func (b Browser) Profiles() ([]Profile, error) {
	switch b.Kind {
	case Chromium:
		return ChromiumProfiles(filepath.Join(b.SupportDir(), "Local State"))
	case Firefox:
		return FirefoxProfiles(filepath.Join(b.SupportDir(), "profiles.ini"))
	}
	return nil, nil
}

// ProfileDir is the absolute directory of a Firefox profile: Path= as is when absolute, else under SupportDir.
func (b Browser) ProfileDir(p Profile) string {
	if filepath.IsAbs(p.Dir) {
		return p.Dir
	}
	return filepath.Join(b.SupportDir(), p.Dir)
}

// DefaultProfile returns the profile an [Install...] section names as default, else the one marked
// Default=1, else the first one.
func (b Browser) DefaultProfile(profiles []Profile) (Profile, bool) {
	for _, p := range profiles {
		if p.InstallDefault {
			return p, true
		}
	}
	for _, p := range profiles {
		if p.Default {
			return p, true
		}
	}
	if len(profiles) > 0 {
		return profiles[0], true
	}
	return Profile{}, false
}

// Containers lists the containers of a Firefox profile; other browsers have none.
func (b Browser) Containers(p Profile) ([]Container, error) {
	if b.Kind != Firefox {
		return nil, nil
	}
	return FirefoxContainers(filepath.Join(b.ProfileDir(p), "containers.json"))
}

// ContainerExtension reports whether the "Open external links in a container" add-on is active in the profile.
func (b Browser) ContainerExtension(p Profile) (bool, error) {
	if b.Kind != Firefox {
		return false, nil
	}
	return FirefoxExtensionActive(filepath.Join(b.ProfileDir(p), "extensions.json"), ContainerExtensionID)
}

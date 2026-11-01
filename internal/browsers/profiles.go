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
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "[Profile"):
			out = append(out, Profile{})
			cur = &out[len(out)-1]
		case strings.HasPrefix(line, "["):
			cur = nil
		case cur != nil:
			if k, v, ok := strings.Cut(line, "="); ok {
				switch k {
				case "Name":
					cur.Name = v
				case "Path":
					cur.Dir = v
				}
			}
		}
	}
	return out, sc.Err()
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

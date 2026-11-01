package browsers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLookup(t *testing.T) {
	for in, want := range map[string]string{
		"dia": "Dia", "DIA": "Dia", "company.thebrowser.dia": "Dia", "chrome": "Google Chrome",
		"com.google.Chrome": "Google Chrome", "Zen": "Zen", "safari": "Safari",
	} {
		if b, ok := Lookup(in); !ok || b.Name != want {
			t.Errorf("Lookup(%q) = %+v, %v; want %s", in, b, ok, want)
		}
	}
	if b, ok := Lookup("Some Browser"); ok || b.Kind != Plain || b.Name != "Some Browser" {
		t.Errorf("unknown browser: %+v %v", b, ok)
	}
}

func TestChromiumProfiles(t *testing.T) {
	p := filepath.Join(t.TempDir(), "Local State")
	os.WriteFile(p, []byte(`{"profile":{"info_cache":{"Default":{"name":"Personal"},"Profile 1":{"name":"Work"}}}}`), 0o600)
	profiles, err := ChromiumProfiles(p)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := Resolve(profiles, "work"); !ok || got.Dir != "Profile 1" {
		t.Errorf("by name: %+v %v", got, ok)
	}
	if got, ok := Resolve(profiles, "Default"); !ok || got.Name != "Personal" {
		t.Errorf("by dir: %+v %v", got, ok)
	}
	if _, ok := Resolve(profiles, "Nope"); ok {
		t.Error("unknown profile resolved")
	}
}

func TestFirefoxProfiles(t *testing.T) {
	p := filepath.Join(t.TempDir(), "profiles.ini")
	os.WriteFile(p, []byte("[Install1]\nDefault=Profiles/a.default\n\n[Profile1]\nName=Work\nIsRelative=1\nPath=Profiles/b.work\n\n[Profile0]\nName=Default\nPath=Profiles/a.default\nDefault=1\n"), 0o600)
	profiles, err := FirefoxProfiles(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 || profiles[0].Name != "Work" || profiles[0].Dir != "Profiles/b.work" || profiles[1].Name != "Default" {
		t.Errorf("profiles = %+v", profiles)
	}
}

const containersFixture = `{"version":8,"lastUserContextId":9,"identities":[
{"userContextId":4,"public":true,"icon":"cart","color":"pink"},
{"userContextId":1,"public":true,"icon":"fingerprint","color":"blue"},
{"userContextId":2,"public":true,"icon":"briefcase","color":"orange"},
{"userContextId":3,"public":true,"icon":"dollar","color":"green"},
{"userContextId":5,"public":true,"icon":"circle","color":"red","name":"Alpha"},
{"userContextId":6,"public":true,"icon":"circle","color":"red","name":"Beta"},
{"userContextId":7,"public":false,"icon":"","color":"","name":"userContextIdInternal.thumbnail"},
{"userContextId":8,"public":false,"icon":"","color":"","name":"userContextIdInternal.webextStorageLocal"},
{"userContextId":9,"public":true,"icon":"circle","color":"red","l10nID":"userContextShopping.label"},
{"userContextId":10,"public":true,"icon":"circle","color":"red","l10nID":"userContextUnknown.label"},
{"userContextId":11,"public":true,"icon":"circle","color":"red"}
]}`

func TestFirefoxContainers(t *testing.T) {
	p := filepath.Join(t.TempDir(), "containers.json")
	os.WriteFile(p, []byte(containersFixture), 0o600)
	got, err := FirefoxContainers(p)
	if err != nil {
		t.Fatal(err)
	}
	want := []Container{{"Personal", 1}, {"Work", 2}, {"Banking", 3}, {"Shopping", 4}, {"Alpha", 5}, {"Beta", 6}, {"Shopping", 9}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	if _, err := FirefoxContainers(filepath.Join(t.TempDir(), "none.json")); err == nil {
		t.Error("missing file: no error")
	}
	os.WriteFile(p, []byte("{"), 0o600)
	if _, err := FirefoxContainers(p); err == nil {
		t.Error("bad JSON: no error")
	}
}

func TestFirefoxExtensionActive(t *testing.T) {
	dir := t.TempDir()
	for name, c := range map[string]struct {
		json string
		want bool
	}{
		"active":   {`{"addons":[{"id":"x"},{"id":"` + ContainerExtensionID + `","active":true,"userDisabled":false,"appDisabled":false}]}`, true},
		"user off": {`{"addons":[{"id":"` + ContainerExtensionID + `","active":true,"userDisabled":true}]}`, false},
		"app off":  {`{"addons":[{"id":"` + ContainerExtensionID + `","active":true,"appDisabled":true}]}`, false},
		"inactive": {`{"addons":[{"id":"` + ContainerExtensionID + `","active":false}]}`, false},
		"absent":   {`{"addons":[{"id":"other","active":true}]}`, false},
	} {
		p := filepath.Join(dir, "e.json")
		os.WriteFile(p, []byte(c.json), 0o600)
		got, err := FirefoxExtensionActive(p, ContainerExtensionID)
		if err != nil || got != c.want {
			t.Errorf("%s: got %v, %v; want %v", name, got, err, c.want)
		}
	}
	if ok, err := FirefoxExtensionActive(filepath.Join(dir, "none.json"), ContainerExtensionID); ok || err == nil {
		t.Errorf("missing file: %v, %v", ok, err)
	}
	os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{"), 0o600)
	if ok, err := FirefoxExtensionActive(filepath.Join(dir, "bad.json"), ContainerExtensionID); ok || err == nil {
		t.Errorf("bad JSON: %v, %v", ok, err)
	}
}

func TestProfileDirAndDefault(t *testing.T) {
	b := Browser{Name: "Zen", Kind: Firefox, ConfigDir: "zen"}
	if got := b.ProfileDir(Profile{Dir: "/abs/p"}); got != "/abs/p" {
		t.Errorf("absolute: %q", got)
	}
	if got := b.ProfileDir(Profile{Dir: "Profiles/a"}); got != filepath.Join(b.SupportDir(), "Profiles/a") {
		t.Errorf("relative: %q", got)
	}
	ini := filepath.Join(t.TempDir(), "profiles.ini")
	os.WriteFile(ini, []byte("[Profile1]\nName=Work\nPath=Profiles/b\n\n[Profile0]\nName=Main\nPath=Profiles/a\nDefault=1\n"), 0o600)
	profiles, _ := FirefoxProfiles(ini)
	if d, ok := b.DefaultProfile(profiles); !ok || d.Name != "Main" {
		t.Errorf("Default=1: %+v %v", d, ok)
	}
	if d, ok := b.DefaultProfile([]Profile{{Name: "A"}, {Name: "B"}}); !ok || d.Name != "A" {
		t.Errorf("first: %+v %v", d, ok)
	}
	if _, ok := b.DefaultProfile(nil); ok {
		t.Error("empty list has a default")
	}
	if c, err := (Browser{Kind: Chromium}).Containers(Profile{}); c != nil || err != nil {
		t.Errorf("chromium containers: %v %v", c, err)
	}
}

func TestContainersFromProfileDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "containers.json"), []byte(containersFixture), 0o600)
	os.WriteFile(filepath.Join(dir, "extensions.json"), []byte(`{"addons":[{"id":"`+ContainerExtensionID+`","active":true}]}`), 0o600)
	b := Browser{Name: "Zen", Kind: Firefox}
	p := Profile{Name: "Default", Dir: dir}
	if c, err := b.Containers(p); err != nil || len(c) == 0 {
		t.Errorf("containers: %v %v", c, err)
	}
	if ok, err := b.ContainerExtension(p); err != nil || !ok {
		t.Errorf("extension: %v %v", ok, err)
	}
}

func TestDefaultProfileInstallSection(t *testing.T) {
	b := Browser{Name: "Zen", Kind: Firefox, ConfigDir: "zen"}
	profs := "[Profile1]\nName=Work\nPath=Profiles/b\n\n[Profile0]\nName=Main\nPath=Profiles/a\nDefault=1\n\n"
	noLegacy := "[Profile1]\nName=Work\nPath=Profiles/b\n\n[Profile0]\nName=Main\nPath=Profiles/a\n\n"
	tests := []struct {
		name, ini, want string
	}{
		{"install beats Default=1", profs + "[Install1]\nDefault=Profiles/b\nLocked=1\n", "Work"},
		{"install only", noLegacy + "[Install1]\nDefault=Profiles/a\n", "Main"},
		{"neither", noLegacy, "Work"},
		{"install path missing falls back to Default=1", profs + "[Install1]\nDefault=Profiles/zzz\n", "Main"},
		{"missing path in one install section is ignored", profs + "[InstallA]\nDefault=Profiles/zzz\n[InstallB]\nDefault=Profiles/b\n", "Work"},
		{"two install sections, same profile", profs + "[InstallA]\nDefault=Profiles/b\n[InstallB]\nDefault=Profiles/b\n", "Work"},
		{"two install sections, two profiles: unknown", profs + "[InstallA]\nDefault=Profiles/b\n[InstallB]\nDefault=Profiles/a\n", ""},
		{"absolute install path", noLegacy + "[Install1]\nDefault=ABSDIR/Profiles/a\n", "Main"},
		{"case sensitive", profs + "[Install1]\nDefault=profiles/b\n", "Main"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			ini := filepath.Join(dir, "profiles.ini")
			content := strings.ReplaceAll(tc.ini, "ABSDIR", dir)
			if err := os.WriteFile(ini, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			profiles, err := FirefoxProfiles(ini)
			if err != nil {
				t.Fatal(err)
			}
			if d, ok := b.DefaultProfile(profiles); ok != (tc.want != "") || d.Name != tc.want {
				t.Errorf("got %+v %v, want %q", d, ok, tc.want)
			}
		})
	}
}

// An [Install...] Default= that does not name a profile exactly never picks a third profile: it falls back to
// Default=1. Surrounding whitespace is trimmed like every other profiles.ini line.
func TestDefaultProfileInstallPathVariants(t *testing.T) {
	b := Browser{Name: "Zen", Kind: Firefox}
	base := "[Profile1]\nName=Work\nPath=Profiles/b\n\n[Profile0]\nName=Main\nPath=Profiles/a\nDefault=1\n\n"
	tests := []struct{ name, install, want string }{
		{"trailing whitespace", "Profiles/b  ", "Work"},
		{"trailing slash falls back", "Profiles/b/", "Main"},
		{"dot prefix falls back", "./Profiles/b", "Main"},
		{"backslash falls back", `Profiles\b`, "Main"},
		{"empty value falls back", "", "Main"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ini := filepath.Join(t.TempDir(), "profiles.ini")
			if err := os.WriteFile(ini, []byte(base+"[Install1]\nDefault="+tc.install+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			profiles, err := FirefoxProfiles(ini)
			if err != nil {
				t.Fatal(err)
			}
			if d, ok := b.DefaultProfile(profiles); !ok || d.Name != tc.want {
				t.Errorf("Default=%q: got %+v %v, want %s", tc.install, d, ok, tc.want)
			}
		})
	}
}

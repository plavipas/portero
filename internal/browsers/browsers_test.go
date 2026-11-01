package browsers

import (
	"os"
	"path/filepath"
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

package launch

import (
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestContainerURL(t *testing.T) {
	shape := regexp.MustCompile(`^ext\+container:name=[A-Za-z0-9._~%-]*&url=[A-Za-z0-9._~%-]+$`)
	links := []string{
		"https://example.com/a?b=1&c=2#top",
		"https://example.com/path with space/%41?q=a+b&r=100%25#frag=1",
		"https://example.com/caf%C3%A9?x=a%2Fb",
		"https://example.com/путь?q=привет&n=señor#ñ",
		"http://example.com:8080/a/b?c=d=e?f",
	}
	containers := []string{
		"Work", "Alpha Beta", "&url=https://evil.example", "a#b", "100%", "a+b=c", "Café", "Рабочий", "señal", "a/b:c?d",
	}
	for _, l := range links {
		u, err := url.Parse(l)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range containers {
			out := ContainerURL(c, u)
			if !shape.MatchString(out) {
				t.Errorf("shape: ContainerURL(%q, %q) = %q", c, l, out)
			}
			q, err := url.ParseQuery(strings.TrimPrefix(out, "ext+container:"))
			if err != nil {
				t.Errorf("ParseQuery(%q): %v", out, err)
				continue
			}
			if q.Get("name") != c || q.Get("url") != u.String() || len(q) != 2 {
				t.Errorf("round trip of %q / %q: %v", c, l, q)
			}
		}
	}
}

func TestContainerURLExact(t *testing.T) {
	u, _ := url.Parse("https://example.com/a?b=1&c=2#top")
	if got, want := ContainerURL("Work", u), "ext+container:name=Work&url=https%3A%2F%2Fexample.com%2Fa%3Fb%3D1%26c%3D2%23top"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := ContainerURL("&url=https://evil.example", u); !strings.HasPrefix(got, "ext+container:name=%26url%3Dhttps%3A%2F%2Fevil.example&url=") {
		t.Errorf("injection not escaped: %q", got)
	}
	if got := escape(" +&#%=/:?"); got != "%20%2B%26%23%25%3D%2F%3A%3F" {
		t.Errorf("escape = %q", got)
	}
}

func TestFirefoxLinkSkipsOtherSchemes(t *testing.T) {
	u, _ := url.Parse("file:///tmp/a.html")
	link, _, notes := firefoxLink(browsersZen(), u, "", "Work")
	if link != "file:///tmp/a.html" || len(notes) != 1 || notes[0] != "container skipped: file links cannot be opened by the extension" {
		t.Errorf("link %q notes %v", link, notes)
	}
	if link, _, notes := firefoxLink(browsersZen(), u, "", ""); link != u.String() || notes != nil {
		t.Errorf("no container: %q %v", link, notes)
	}
}

func TestResolveTargetIgnoresContainerOutsideFirefox(t *testing.T) {
	b, _ := browsersLookup("Dia")
	profile, container, note, err := resolveTarget(b, rulesTarget("Dia", "Personal", "Work"))
	if err != nil || profile != "Personal" || container != "" || note != `container "Work" ignored: Dia does not take one` {
		t.Errorf("got %q %q %q %v", profile, container, note, err)
	}
}

// TestContainerURLHardLinks feeds links that tend to break hand-written encoders and checks that the
// extension's query decodes back to the container name and to u.String() byte for byte.
func TestContainerURLHardLinks(t *testing.T) {
	long := "https://example.com/" + strings.Repeat("seg%2F", 2000) + "?q=" + strings.Repeat("a&b=c%26", 2000) + "#" + strings.Repeat("f", 4000)
	links := map[string]string{
		"already encoded slash": "https://example.com/a%2Fb/c?redirect=https%3A%2F%2Fexample.org%2F%3Fx%3D1",
		"double encoded":        "https://example.com/?q=%252F%2520",
		"idn host":              "https://bücher.example/straße?ä=ö",
		"punycode host":         "https://xn--bcher-kva.example/",
		"userinfo":              "https://user:p%40ss@example.com/a",
		"ipv6 host":             "http://[2001:db8::1]:8080/a?b=c",
		"ipv6 zone":             "http://[fe80::1%25en0]/",
		"empty fragment":        "https://example.com/a?#",
		"semicolon query":       "https://example.com/a?x=1;y=2",
		"plus and space":        "https://example.com/a+b?q=a+b%20c",
		"very long":             long,
	}
	containers := []string{"Work", "x&url=https://evil.example", "a b", "%41", "日本"}
	for name, l := range links {
		u, err := url.Parse(l)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, c := range containers {
			out := ContainerURL(c, u)
			if strings.ContainsAny(out[len("ext+container:"):], "#?/ +") {
				t.Errorf("%s / %q: unescaped delimiter in %q", name, c, out)
			}
			q, err := url.ParseQuery(strings.TrimPrefix(out, "ext+container:"))
			if err != nil {
				t.Errorf("%s / %q: ParseQuery: %v", name, c, err)
				continue
			}
			if len(q) != 2 || len(q["name"]) != 1 || len(q["url"]) != 1 || q.Get("name") != c || q.Get("url") != u.String() {
				t.Errorf("%s / %q: round trip %v", name, c, q)
			}
		}
	}
}

func TestContainerURLInjectionStaysInName(t *testing.T) {
	u, _ := url.Parse("https://example.com/")
	got := ContainerURL("x&url=https://evil.example", u)
	want := "ext+container:name=x%26url%3Dhttps%3A%2F%2Fevil.example&url=https%3A%2F%2Fexample.com%2F"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// firefoxHome builds a fake HOME with a Zen support dir: profiles.ini, and per profile an optional
// containers.json and extensions.json. SupportDir derives from os.UserHomeDir, which reads $HOME.
func firefoxHome(t *testing.T, ini string, files map[string]string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	support := filepath.Join(home, "Library", "Application Support", browsersZen().ConfigDir)
	if err := os.MkdirAll(support, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(support, "profiles.ini"), []byte(ini), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		p := filepath.Join(support, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

const (
	testIni     = "[Profile0]\nName=Personal\nPath=Profiles/a\nDefault=1\n\n[Profile1]\nName=Work\nPath=Profiles/b\n\n"
	extOn       = `{"addons":[{"id":"{f069aec0-43c5-4bbf-b6b4-df95c4326b98}","active":true}]}`
	extOff      = `{"addons":[{"id":"{f069aec0-43c5-4bbf-b6b4-df95c4326b98}","active":false}]}`
	testContain = `{"identities":[{"userContextId":1,"name":"Work","public":true},{"userContextId":2,"name":"Banking","public":true}]}`
)

func TestFirefoxLinkExtensionStates(t *testing.T) {
	u, _ := url.Parse("https://example.com/x")
	plain := u.String()
	withContainer := ContainerURL("Work", u)
	tests := []struct {
		name    string
		ini     string
		files   map[string]string
		profile string
		link    string
		note    string
	}{
		{"active in default profile", testIni, map[string]string{"Profiles/a/extensions.json": extOn}, "", withContainer, ""},
		{"active in named profile", testIni, map[string]string{"Profiles/b/extensions.json": extOn}, "Work", withContainer, ""},
		{"named profile not default", testIni, map[string]string{"Profiles/a/extensions.json": extOff, "Profiles/b/extensions.json": extOn}, "Work", withContainer, ""},
		{"not active", testIni, map[string]string{"Profiles/a/extensions.json": extOff}, "", plain,
			`container extension not active in profile "Personal" of Zen: opening without container`},
		{"extensions.json missing", testIni, nil, "", plain,
			`container extension state unknown in profile "Personal" of Zen (extensions.json not found): opening without container`},
		{"extensions.json broken", testIni, map[string]string{"Profiles/a/extensions.json": "{"}, "", plain,
			`container extension state unknown in profile "Personal" of Zen (extensions.json unreadable): opening without container`},
		{"install sections disagree", testIni + "[InstallA]\nDefault=Profiles/a\n[InstallB]\nDefault=Profiles/b\n",
			map[string]string{"Profiles/a/extensions.json": extOn, "Profiles/b/extensions.json": extOn}, "", plain,
			"default profile of Zen is unknown (no profiles, or several Install sections disagree): opening without container"},
		{"disagreeing installs, named profile", testIni + "[InstallA]\nDefault=Profiles/a\n[InstallB]\nDefault=Profiles/b\n",
			map[string]string{"Profiles/b/extensions.json": extOn}, "Work", withContainer, ""},
		{"unknown named profile", testIni, nil, "Nope", plain, `profile "Nope" of Zen not found: opening without container`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			firefoxHome(t, tc.ini, tc.files)
			link, _, notes := firefoxLink(browsersZen(), u, tc.profile, "Work")
			if link != tc.link {
				t.Errorf("link %q, want %q", link, tc.link)
			}
			var want []string
			if tc.note != "" {
				want = []string{tc.note}
			}
			if !reflect.DeepEqual(notes, want) {
				t.Errorf("notes %q, want %q", notes, want)
			}
		})
	}
}

func TestResolveTargetContainerNames(t *testing.T) {
	tests := []struct {
		name, container, want, note string
	}{
		{"exact", "Work", "Work", ""},
		{"case-insensitive uses the file spelling", "work", "Work", ""},
		{"unknown name passes as written with a note", "Team Alpha", "Team Alpha",
			`container "Team Alpha" not found in profile "Personal" of Zen (have: Work, Banking); the extension creates it on first use`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			firefoxHome(t, testIni, map[string]string{"Profiles/a/containers.json": testContain, "Profiles/a/extensions.json": extOn})
			_, container, note, err := resolveTarget(browsersZen(), rulesTarget("Zen", "", tc.container))
			if err != nil || container != tc.want || note != tc.note {
				t.Errorf("got %q %q %v", container, note, err)
			}
			u, _ := url.Parse("https://example.com/")
			link, _, notes := firefoxLink(browsersZen(), u, "", container)
			if link != ContainerURL(tc.want, u) || len(notes) != 0 {
				t.Errorf("link %q notes %q", link, notes)
			}
		})
	}
	t.Run("containers.json unreadable gives no note", func(t *testing.T) {
		firefoxHome(t, testIni, nil)
		_, container, note, err := resolveTarget(browsersZen(), rulesTarget("Zen", "", "Work"))
		if err != nil || container != "Work" || note != "" {
			t.Errorf("got %q %q %v", container, note, err)
		}
	})
}

package launch

import (
	"net/url"
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
	link, notes := firefoxLink(browsersZen(), u, "", "Work")
	if link != "file:///tmp/a.html" || len(notes) != 1 || notes[0] != "container skipped: file links cannot be opened by the extension" {
		t.Errorf("link %q notes %v", link, notes)
	}
	if link, notes := firefoxLink(browsersZen(), u, "", ""); link != u.String() || notes != nil {
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

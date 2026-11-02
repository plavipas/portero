package main

import (
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/plavipas/portero/internal/config"
	"github.com/plavipas/portero/internal/launch"
	"github.com/plavipas/portero/internal/rules"
)

// recordOrder replaces startDownload and returns an open func; both append to the same event list.
func recordOrder(t *testing.T) (*[]string, func(*url.URL, rules.Target) (launch.Plan, error)) {
	t.Helper()
	var events []string
	old := startDownload
	startDownload = func(string) error { events = append(events, "download"); return nil }
	t.Cleanup(func() { startDownload = old })
	return &events, func(*url.URL, rules.Target) (launch.Plan, error) {
		events = append(events, "open")
		return launch.Plan{}, nil
	}
}

func TestRouteLinkOffloadedOpensBeforeDownload(t *testing.T) {
	offloadRig(t)
	events, open := recordOrder(t)
	if err := routeLinkWith("https://example.com/", "", open); err != nil {
		t.Fatal(err)
	}
	if strings.Join(*events, ",") != "open,download" {
		t.Errorf("events = %v, want open then download", *events)
	}
}

func TestOpenUIOffloadedOpensBeforeDownload(t *testing.T) {
	offloadRig(t)
	events, open := recordOrder(t)
	link := "http://127.0.0.1:5555/"
	if err := openUI(link, link+"auth/x", open); err != nil {
		t.Fatal(err)
	}
	if strings.Join(*events, ",") != "open,download" {
		t.Errorf("events = %v, want open then download", *events)
	}
}

func TestRouteLinkOffloadedLogLineHasFiveColumns(t *testing.T) {
	offloadRig(t)
	_, open := recordOrder(t)
	if err := routeLinkWith("https://example.com/a", "com.example.app", open); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(config.LogFile())
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("want one log line, got %q", b)
	}
	cols := strings.Split(lines[0], "\t")
	if len(cols) != 5 {
		t.Fatalf("want 5 columns, got %d: %q", len(cols), lines[0])
	}
	if cols[1] != "com.example.app" || cols[2] != "ERROR" || cols[4] != `"https://example.com/a"` {
		t.Errorf("columns = %q", cols)
	}
}

func TestRouteLinkOffloadedLogsDashWithoutOpener(t *testing.T) {
	offloadRig(t)
	_, open := recordOrder(t)
	if err := routeLinkWith("https://example.com/", "", open); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(config.LogFile())
	if cols := strings.Split(string(b), "\t"); len(cols) < 2 || cols[1] != "-" {
		t.Errorf("log = %q", b)
	}
}

func TestRouteLinkOffloadedReturnsOpenError(t *testing.T) {
	offloadRig(t)
	calls := fakeDownload(t, nil)
	want := errors.New("open failed")
	open := func(*url.URL, rules.Target) (launch.Plan, error) { return launch.Plan{}, want }
	if err := routeLinkWith("https://example.com/", "", open); !errors.Is(err, want) {
		t.Errorf("err = %v, want the open error", err)
	}
	if len(*calls) != 1 {
		t.Errorf("download must still be started when the browser fails, calls = %v", *calls)
	}
}

func TestRouteLinkOffloadedRefusesNonURLFirst(t *testing.T) {
	offloadRig(t)
	calls := fakeDownload(t, nil)
	opened := false
	open := func(*url.URL, rules.Target) (launch.Plan, error) { opened = true; return launch.Plan{}, nil }
	if err := routeLinkWith("not a url", "", open); err == nil {
		t.Error("want an error for a non-URL")
	}
	if opened || len(*calls) != 0 {
		t.Errorf("opened=%v downloads=%v", opened, *calls)
	}
}

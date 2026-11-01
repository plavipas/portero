package main

import (
	"flag"
	"testing"
)

func TestParseLinkFlagOrder(t *testing.T) {
	cases := map[string][]string{
		"flags after":   {"https://example.com/a", "--cwd", "/x", "--opener", "com.example.app", "--dry-run"},
		"flags before":  {"--cwd", "/x", "--opener", "com.example.app", "--dry-run", "https://example.com/a"},
		"flags around":  {"--cwd=/x", "https://example.com/a", "--opener=com.example.app", "--dry-run"},
		"url in middle": {"--dry-run", "https://example.com/a", "--cwd", "/x", "--opener", "com.example.app"},
	}
	for name, args := range cases {
		got, err := parseLink(flag.NewFlagSet("open", flag.ContinueOnError), args)
		if err != nil || got.url.String() != "https://example.com/a" || got.cwd != "/x" || got.opener != "com.example.app" || !got.dry {
			t.Errorf("%s: got %+v, %v", name, got, err)
		}
	}
	for name, args := range map[string][]string{
		"no url":   {"--cwd", "/x"},
		"two urls": {"https://a.example", "https://b.example"},
		"no host":  {"--dry-run", "nothing"},
	} {
		if _, err := parseLink(flag.NewFlagSet("open", flag.ContinueOnError), args); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

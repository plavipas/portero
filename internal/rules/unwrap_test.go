package rules

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
)

func fakeJWT(payload string) string {
	e := base64.RawURLEncoding
	return e.EncodeToString([]byte(`{"alg":"none"}`)) + "." + e.EncodeToString([]byte(payload)) + ".sig"
}

func slackLink(hint string) string {
	return "https://example-team.slack.com/openid/connect/login_initiate_redirect?login_hint=" + url.QueryEscape(hint)
}

func TestUnwrap(t *testing.T) {
	good := fakeJWT(`{"https://slack.com/target_uri":"https://app.example.com/x?a=1"}`)
	padded := strings.Replace(good, ".sig", ".sig", 1)
	parts := strings.Split(good, ".")
	padded = parts[0] + "." + parts[1] + "==." + parts[2]
	g := func(inner string) string { return "https://www.google.com/url?q=" + url.QueryEscape(inner) }
	cases := []struct {
		name, in, want string
	}{
		{"slack sign-in", slackLink(good), "https://app.example.com/x?a=1"},
		{"slack sign-in padded", slackLink(padded), "https://app.example.com/x?a=1"},
		{"slack bare host", "https://slack.com/openid/connect/login_initiate_redirect?login_hint=" + good, "https://app.example.com/x?a=1"},
		{"google q", g("https://github.com/x"), "https://github.com/x"},
		{"google url", "https://google.com/url?url=https%3A%2F%2Fgithub.com%2Fx", "https://github.com/x"},
		{"google q wins", "https://www.google.com/url?q=https%3A%2F%2Fa.example.com&url=https%3A%2F%2Fb.example.com", "https://a.example.com"},
		{"safelinks", "https://eur01.safelinks.protection.outlook.com/?url=https%3A%2F%2Fdocs.example.com%2Fa&data=x", "https://docs.example.com/a"},
		{"slack redirect", "https://slack-redirect.com/link?url=https%3A%2F%2Fdocs.example.com%2Fa", "https://docs.example.com/a"},
		{"safelinks to google", "https://eur01.safelinks.protection.outlook.com/?url=" + url.QueryEscape(g("https://github.com/x")), "https://github.com/x"},
		{"three levels", "https://slack-redirect.com/link?url=" + url.QueryEscape("https://eur01.safelinks.protection.outlook.com/?url="+url.QueryEscape(g("https://github.com/x"))), "https://github.com/x"},
		{"depth limit", "https://slack-redirect.com/link?url=" + url.QueryEscape("https://slack-redirect.com/link?url="+url.QueryEscape("https://slack-redirect.com/link?url="+url.QueryEscape("https://slack-redirect.com/link?url=https%3A%2F%2Fgithub.com"))), "https://slack-redirect.com/link?url=https%3A%2F%2Fgithub.com"},
		{"plain link", "https://github.com/x", ""},
		{"search page", "https://www.google.com/search?q=hello", ""},
		{"relative q", "https://www.google.com/url?q=/search", ""},
		{"no q", "https://www.google.com/url", ""},
		{"javascript inner", g("javascript:alert(1)"), ""},
		{"file inner", g("file:///etc/passwd"), ""},
		{"hostless inner", g("https:///path"), ""},
		{"hostless http", g("http:example"), ""},
		{"oversize inner", g("https://example.com/" + strings.Repeat("a", 9000)), ""},
		{"wrong google host", "https://evilgoogle.com/url?q=https%3A%2F%2Fgithub.com", ""},
		{"wrong slack path", "https://example-team.slack.com/other?login_hint=" + good, ""},
		{"bad base64", slackLink("a.!!!.c"), ""},
		{"bad json", slackLink(fakeJWT(`not json`)), ""},
		{"no claim", slackLink(fakeJWT(`{"sub":"x"}`)), ""},
		{"non-string claim", slackLink(fakeJWT(`{"https://slack.com/target_uri":5}`)), ""},
		{"two parts", slackLink("a.b"), ""},
		{"oversize hint", slackLink(strings.Repeat("a", 17000)), ""},
		{"javascript claim", slackLink(fakeJWT(`{"https://slack.com/target_uri":"javascript:alert(1)"}`)), ""},
	}
	for _, c := range cases {
		u, err := url.Parse(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got, ok := Unwrap(u)
		if c.want == "" {
			if ok {
				t.Errorf("%s: unwrapped to %s, want no unwrap", c.name, got)
			}
			continue
		}
		if !ok || got.String() != c.want {
			t.Errorf("%s: got %v %v, want %s", c.name, got, ok, c.want)
		}
	}
	if _, ok := Unwrap(nil); ok {
		t.Error("nil unwrapped")
	}
}

func FuzzUnwrap(f *testing.F) {
	f.Add("https://www.google.com/url?q=https%3A%2F%2Fgithub.com")
	f.Add(slackLink(fakeJWT(`{"https://slack.com/target_uri":"https://a.example.com"}`)))
	f.Fuzz(func(t *testing.T, s string) {
		u, err := url.Parse(s)
		if err != nil {
			return
		}
		if in, ok := Unwrap(u); ok && (in == nil || in.Host == "") {
			t.Fatalf("bad inner for %q", s)
		}
	})
}

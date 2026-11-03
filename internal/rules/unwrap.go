package rules

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
)

const (
	// MaxUnwrapDepth is how many wrappers in a row are peeled off one link.
	MaxUnwrapDepth = 3
	maxLoginHint   = 16 << 10
	maxInnerURL    = 8 << 10
	slackTargetKey = "https://slack.com/target_uri"
)

// Unwrap returns the real target of a link that a known service wrapped in a redirect, so that the link routes
// like the inner one clicked directly. Wrappers: the Slack sign-in redirect (a JWT in login_hint whose
// "https://slack.com/target_uri" claim is the target; the signature is not checked, the claim only picks a
// browser), Google's /url?q=, Outlook Safe Links and slack-redirect.com. Up to MaxUnwrapDepth levels are
// peeled. The inner URL must be http or https, have a host and be at most 8 KiB. When nothing can be
// unwrapped ok is false. No network is used.
func Unwrap(u *url.URL) (inner *url.URL, ok bool) {
	cur := u
	for i := 0; i < MaxUnwrapDepth; i++ {
		next := unwrapOnce(cur)
		if next == nil {
			break
		}
		cur = next
	}
	if cur == u {
		return nil, false
	}
	return cur, true
}

func unwrapOnce(u *url.URL) *url.URL {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	q := u.Query()
	var raw string
	switch {
	case (host == "slack.com" || strings.HasSuffix(host, ".slack.com")) && u.Path == "/openid/connect/login_initiate_redirect":
		raw = slackTarget(q.Get("login_hint"))
	case (host == "google.com" || host == "www.google.com") && u.Path == "/url":
		if raw = q.Get("q"); raw == "" {
			raw = q.Get("url")
		}
	case strings.HasSuffix(host, ".safelinks.protection.outlook.com"):
		raw = q.Get("url")
	case host == "slack-redirect.com" && u.Path == "/link":
		raw = q.Get("url")
	}
	return parseInner(raw)
}

func parseInner(raw string) *url.URL {
	if raw == "" || len(raw) > maxInnerURL {
		return nil
	}
	in, err := url.Parse(raw)
	if err != nil || in.Host == "" || in.Hostname() == "" {
		return nil
	}
	if s := strings.ToLower(in.Scheme); s != "http" && s != "https" {
		return nil
	}
	return in
}

// slackTarget reads the target_uri claim of the JWT in login_hint.
func slackTarget(jwt string) string {
	if jwt == "" || len(jwt) > maxLoginHint {
		return ""
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return ""
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return ""
	}
	var claims map[string]json.RawMessage
	if json.Unmarshal(b, &claims) != nil {
		return ""
	}
	var s string
	if json.Unmarshal(claims[slackTargetKey], &s) != nil {
		return ""
	}
	return s
}

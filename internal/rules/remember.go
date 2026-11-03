package rules

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

var meetCode = regexp.MustCompile(`^[a-z]{3}-[a-z]{4}-[a-z]{3}$`)

const maxRememberPath = 512

// RememberKey returns the host and path a "Remember" rule pins for u: the host in lower case and the part of the
// path that names one meeting or page (the query is ignored). ok is false when there is nothing specific to pin
// (`/` or an empty path) or the path cannot be written as a plain path prefix.
//
//	meet.google.com/abc-defg-hij?authuser=1   -> meet.google.com /abc-defg-hij
//	us06web.zoom.us/j/123?pwd=x               -> us06web.zoom.us /j/123
//	example.com/team/room/                    -> example.com /team/room
func RememberKey(u *url.URL) (host, path string, ok bool) {
	if u == nil {
		return "", "", false
	}
	host = strings.ToLower(u.Hostname())
	p := u.Path
	if host == "" || p == "" || p == "/" {
		return "", "", false
	}
	segs := strings.Split(strings.Trim(p, "/"), "/")
	switch {
	case host == "meet.google.com":
		if !meetCode.MatchString(segs[0]) {
			return "", "", false
		}
		p = "/" + segs[0]
	case host == "zoom.us" || strings.HasSuffix(host, ".zoom.us"):
		switch {
		case len(segs) >= 2 && (segs[0] == "j" || segs[0] == "w" || segs[0] == "s"):
			p = "/" + segs[0] + "/" + segs[1]
		case len(segs) >= 2 && segs[0] == "my":
			p = "/my/" + segs[1]
		default:
			p = strings.TrimRight(p, "/")
		}
	default:
		p = strings.TrimRight(p, "/")
	}
	if p == "" || p != strings.TrimSpace(p) || len(p) > maxRememberPath || strings.Contains(p, "*") || strings.Contains(p, " / ") {
		return "", "", false
	}
	for _, r := range p + host {
		if r < 0x20 || r == 0x7f || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			return "", "", false
		}
	}
	if strings.ContainsAny(host, "*\t\n\r ") {
		return "", "", false
	}
	return host, p, true
}

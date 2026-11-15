package rules

import (
	"errors"
	"net/url"
	"strings"
)

// PorteroScheme is the URL scheme Portero.app registers for links that web pages hand to it.
const PorteroScheme = "portero"

func rejectedLink(reason string) error { return errors.New("portero link rejected: " + reason) }

// PorteroLink reads a `portero://open?url=<percent-encoded http(s) URL>` link (ADR 0008). is is false for any
// other scheme and the link is none of its business. For a portero link it returns the inner URL, or a
// rejection whose text is fixed: the link content never appears in it.
//
// The form is strict: host `open`, an empty path or `/`, no user info, port or fragment, and a query with
// exactly one parameter, `url`, given once. The value is decoded once (never as a portero link again) and must
// be an http or https URL with a host, at most 8 KiB. Unknown parameters and fragments are rejected because
// they are what an unencoded inner URL turns into.
func PorteroLink(u *url.URL) (inner *url.URL, is bool, err error) {
	if u == nil || !strings.EqualFold(u.Scheme, PorteroScheme) {
		return nil, false, nil
	}
	if u.Opaque != "" || u.User != nil || !strings.EqualFold(u.Host, "open") || (u.Path != "" && u.Path != "/") {
		return nil, true, rejectedLink("unknown action")
	}
	if u.Fragment != "" || u.RawFragment != "" {
		return nil, true, rejectedLink("fragment outside the url parameter")
	}
	q, perr := url.ParseQuery(u.RawQuery)
	if perr != nil {
		return nil, true, rejectedLink("malformed query")
	}
	for k := range q {
		if k != "url" {
			return nil, true, rejectedLink("unknown parameter")
		}
	}
	switch vals := q["url"]; {
	case len(vals) == 0 || vals[0] == "":
		return nil, true, rejectedLink("missing url parameter")
	case len(vals) > 1:
		return nil, true, rejectedLink("url parameter repeated")
	default:
		in := parseInner(vals[0])
		if in == nil {
			return nil, true, rejectedLink("url is not an http(s) URL with a host, at most 8 KiB")
		}
		return in, true, nil
	}
}

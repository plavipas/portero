package launch

import (
	"fmt"
	"net/url"
	"strings"
)

// ContainerURL builds the link the "Open external links in a container" extension understands:
// ext+container:name=<container>&url=<link>. Both values are percent-encoded exactly once.
func ContainerURL(container string, u *url.URL) string {
	return "ext+container:name=" + escape(container) + "&url=" + escape(u.String())
}

// escape percent-encodes every byte except the RFC 3986 unreserved characters.
func escape(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%c%c", hex[c>>4], hex[c&15])
		}
	}
	return b.String()
}

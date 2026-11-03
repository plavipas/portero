package rules

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Punycode (RFC 3492) on the standard library, for showing an internationalised host in both forms. It is a
// display aid, not a resolver: no UTS 46 mapping, no validation beyond the codec (only `xn--` labels).

const (
	pcBase        = 36
	pcTMin        = 1
	pcTMax        = 26
	pcSkew        = 38
	pcDamp        = 700
	pcInitialBias = 72
	pcInitialN    = 128
	acePrefix     = "xn--"
)

var errPunycode = errors.New("invalid punycode")

func pcAdapt(delta, numPoints int, first bool) int {
	if first {
		delta /= pcDamp
	} else {
		delta /= 2
	}
	delta += delta / numPoints
	k := 0
	for delta > ((pcBase-pcTMin)*pcTMax)/2 {
		delta /= pcBase - pcTMin
		k += pcBase
	}
	return k + (pcBase-pcTMin+1)*delta/(delta+pcSkew)
}

func pcThreshold(k, bias int) int {
	switch {
	case k <= bias:
		return pcTMin
	case k >= bias+pcTMax:
		return pcTMax
	}
	return k - bias
}

func pcDigit(d int) byte {
	if d < 26 {
		return byte('a' + d)
	}
	return byte('0' + d - 26)
}

func pcValue(c byte) int {
	switch {
	case c >= 'a' && c <= 'z':
		return int(c - 'a')
	case c >= 'A' && c <= 'Z':
		return int(c - 'A')
	case c >= '0' && c <= '9':
		return int(c-'0') + 26
	}
	return -1
}

// punyEncode is the RFC 3492 encoding of s (without the `xn--` prefix).
func punyEncode(s string) (string, error) {
	in := []rune(s)
	var out []byte
	for _, r := range in {
		if r < 0x80 {
			out = append(out, byte(r))
		}
	}
	b := len(out)
	h := b
	if b > 0 {
		out = append(out, '-')
	}
	n, delta, bias := pcInitialN, 0, pcInitialBias
	for h < len(in) {
		m := int(^uint32(0) >> 1)
		for _, r := range in {
			if int(r) >= n && int(r) < m {
				m = int(r)
			}
		}
		if (m - n) > (int(^uint32(0)>>1)-delta)/(h+1) {
			return "", errPunycode
		}
		delta += (m - n) * (h + 1)
		n = m
		for _, r := range in {
			if int(r) < n {
				delta++
			}
			if int(r) == n {
				q := delta
				for k := pcBase; ; k += pcBase {
					t := pcThreshold(k, bias)
					if q < t {
						break
					}
					out = append(out, pcDigit(t+(q-t)%(pcBase-t)))
					q = (q - t) / (pcBase - t)
				}
				out = append(out, pcDigit(q))
				bias = pcAdapt(delta, h+1, h == b)
				delta = 0
				h++
			}
		}
		delta++
		n++
	}
	return string(out), nil
}

// punyDecode is the RFC 3492 decoding of s (without the `xn--` prefix).
func punyDecode(s string) (string, error) {
	var out []rune
	rest := s
	if i := strings.LastIndexByte(s, '-'); i >= 0 {
		for j := 0; j < i; j++ {
			if s[j] >= 0x80 {
				return "", errPunycode
			}
			out = append(out, rune(s[j]))
		}
		rest = s[i+1:]
	}
	n, i, bias := pcInitialN, 0, pcInitialBias
	const maxInt = int(^uint32(0) >> 1)
	for p := 0; p < len(rest); {
		oldi, w := i, 1
		for k := pcBase; ; k += pcBase {
			if p >= len(rest) {
				return "", errPunycode
			}
			d := pcValue(rest[p])
			p++
			if d < 0 || d > (maxInt-i)/w {
				return "", errPunycode
			}
			i += d * w
			t := pcThreshold(k, bias)
			if d < t {
				break
			}
			if w > maxInt/(pcBase-t) {
				return "", errPunycode
			}
			w *= pcBase - t
		}
		bias = pcAdapt(i-oldi, len(out)+1, oldi == 0)
		if i/(len(out)+1) > maxInt-n {
			return "", errPunycode
		}
		n += i / (len(out) + 1)
		i %= len(out) + 1
		if n > utf8.MaxRune || (n >= 0xD800 && n <= 0xDFFF) {
			return "", errPunycode
		}
		out = append(out, 0)
		copy(out[i+1:], out[i:])
		out[i] = rune(n)
		i++
	}
	return string(out), nil
}

// unsafeLabel reports a label that is empty or holds control, format (bidi override, zero-width) or
// line-separator characters.
func unsafeLabel(l string) bool {
	if l == "" {
		return true
	}
	for _, r := range l {
		if r < 0x20 || (r >= 0x7f && r < 0xa0) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			return true
		}
	}
	return false
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// HostForms is the host as the "Remember" label and `rules match` show it. An ASCII host stays as is; a host
// with non-ASCII labels is `<unicode> = <punycode>`; a host with `xn--` labels is `<punycode> = <unicode>`, so
// a look-alike cannot hide behind either form. A codec error leaves the plain host. The rule written is never
// changed by this.
func HostForms(host string) string {
	labels := strings.Split(strings.ToLower(host), ".")
	ace := make([]string, len(labels))
	uni := make([]string, len(labels))
	nonASCII, hasACE, unsafe := false, false, false
	for i, l := range labels {
		switch {
		case !isASCII(l):
			p, err := punyEncode(l)
			if err != nil {
				return host
			}
			nonASCII = true
			ace[i], uni[i] = acePrefix+p, l
			unsafe = unsafe || unsafeLabel(l)
		case strings.HasPrefix(l, acePrefix):
			u, err := punyDecode(l[len(acePrefix):])
			if err != nil {
				return host
			}
			hasACE = true
			ace[i], uni[i] = l, u
			unsafe = unsafe || unsafeLabel(u)
		default:
			ace[i], uni[i] = l, l
		}
	}
	if unsafe {
		// Control, format (bidi, zero-width) or empty labels must not reach a terminal or a panel: only ASCII.
		return strings.Join(ace, ".")
	}
	switch {
	case nonASCII:
		return strings.Join(uni, ".") + " = " + strings.Join(ace, ".")
	case hasACE:
		return strings.Join(ace, ".") + " = " + strings.Join(uni, ".")
	}
	return host
}

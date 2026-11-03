package rules

import (
	"testing"
	"unicode/utf8"
)

func TestHostForms(t *testing.T) {
	tests := []struct{ host, want string }{
		{"example.com", "example.com"},
		{"", ""},
		{"xn--80ak6aa92e.com", "xn--80ak6aa92e.com = аррӏе.com"},
		{"аррӏе.com", "аррӏе.com = xn--80ak6aa92e.com"},
		{"bücher.example", "bücher.example = xn--bcher-kva.example"},
		{"xn--bcher-kva.example", "xn--bcher-kva.example = bücher.example"},
		{"www.xn--bcher-kva.example", "www.xn--bcher-kva.example = www.bücher.example"},
		{"xn--ab-g4t.com", "xn--ab-g4t.com"},
		{"xn--.com", "xn--.com"},
		// A pure-ASCII decode (nothing after the last hyphen) is shown as the label it decodes to.
		{"xn--abc-", "xn--abc- = abc"},
		{"a\u0085b.xn--!!.com", "xn--ab-qa.xn--!!.com"},
		{"a\u009bb.xn--!!.com", "xn--ab-mca.xn--!!.com"},
		{"aİb.com", "aib.com"},
		{"XN--BCHER-KVA.example", "xn--bcher-kva.example = bücher.example"},
		{"example.com.", "example.com."},
		{"xn--bcher-kva.example.", "xn--bcher-kva.example. = bücher.example."},
		{"a\u202eb.com", "xn--ab-g4t.com"},
		{"xn--!!.example", "xn--!!.example"},
		{"xn--9999999999999999.example", "xn--9999999999999999.example"},
	}
	for _, tt := range tests {
		if got := HostForms(tt.host); got != tt.want {
			t.Errorf("HostForms(%q) = %q, want %q", tt.host, got, tt.want)
		}
	}
}

func TestPunycodeRFCVectors(t *testing.T) {
	// RFC 3492 section 7.1 samples.
	tests := []struct{ uni, ace string }{
		{"почемужеонинеговорятпорусски", "b1abfaaepdrnnbgefbadotcwatmq2g4l"},
		{"他们为什么不说中文", "ihqwcrb4cv8a8dqg056pqjye"},
		{"Pročprostěnemluvíčesky", "Proprostnemluvesky-uyb24dma41a"},
		{"3年B組金八先生", "3B-ww4c5e180e575a65lsy2b"},
	}
	for _, tt := range tests {
		enc, err := punyEncode(tt.uni)
		if err != nil || enc != tt.ace && !equalFold(enc, tt.ace) {
			t.Errorf("punyEncode(%q) = %q, %v; want %q", tt.uni, enc, err, tt.ace)
		}
		dec, err := punyDecode(tt.ace)
		if err != nil || dec != tt.uni {
			t.Errorf("punyDecode(%q) = %q, %v; want %q", tt.ace, dec, err, tt.uni)
		}
	}
}

func equalFold(a, b string) bool {
	return len(a) == len(b) && lower(a) == lower(b)
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func FuzzPunycodeRoundTrip(f *testing.F) {
	for _, s := range []string{"bücher", "почемужеонинеговорятпорусски", "他们为什么不说中文", "a-b", "abc", "аррӏе", "3年B組金八先生", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) {
			t.Skip()
		}
		enc, err := punyEncode(s)
		if err != nil {
			t.Skip()
		}
		dec, err := punyDecode(enc)
		if err != nil || dec != s {
			t.Errorf("punyDecode(punyEncode(%q)) = %q, %v (encoded %q)", s, dec, err, enc)
		}
	})
}

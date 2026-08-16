package douyin

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEncodeAccountSDKSourceInfoRoundTrip(t *testing.T) {
	type tiny struct {
		HardwareConcurrency int  `json:"hardwareConcurrency"`
		Webdriver           bool `json:"webdriver"`
	}
	hex := encodeAccountSDKSourceInfo(tiny{4, false})
	if hex == "" || len(hex)%2 != 0 {
		t.Fatalf("bad hex %q", hex)
	}
	raw := make([]byte, len(hex)/2)
	for i := 0; i < len(raw); i++ {
		var v byte
		for _, c := range []byte{hex[i*2], hex[i*2+1]} {
			v <<= 4
			switch {
			case c >= '0' && c <= '9':
				v |= c - '0'
			case c >= 'a' && c <= 'f':
				v |= c - 'a' + 10
			default:
				t.Fatalf("non-hex %q", hex)
			}
		}
		raw[i] = v
	}
	for i := range raw {
		raw[i] ^= sdkInfoXOR
	}
	var out tiny
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err, string(raw))
	}
	if out.HardwareConcurrency != 4 || out.Webdriver {
		t.Fatalf("decoded %+v", out)
	}
}

func TestAccountSDKSourceInfoShape(t *testing.T) {
	hex := accountSDKSourceInfo()
	if hex == "" {
		t.Fatal("empty")
	}
	if hex != strings.ToLower(hex) {
		t.Fatal("expected lowercase hex")
	}
	// decode
	raw := make([]byte, len(hex)/2)
	for i := 0; i < len(raw); i++ {
		a, b := hex[i*2], hex[i*2+1]
		nibble := func(c byte) byte {
			if c >= 'a' {
				return c - 'a' + 10
			}
			return c - '0'
		}
		raw[i] = nibble(a)<<4 | nibble(b)
		raw[i] ^= sdkInfoXOR
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err, string(raw[:min(120, len(raw))]))
	}
	for _, k := range []string{
		"hardwareConcurrency", "webdriver", "plugins", "innerWidth",
		"stoargeStatus", "browser", "request_pathname",
	} {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing key %s in %v", k, keysOf(m))
		}
	}
	// cached
	if accountSDKSourceInfo() != hex {
		t.Fatal("not cached")
	}
	t.Logf("sdk_info bytes=%d hardware=%v path=%v",
		len(raw), m["hardwareConcurrency"], m["request_pathname"])
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

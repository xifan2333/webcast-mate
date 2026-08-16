package douyin

// Pure a_bogus (bdms 1.0.1.20) — port of ~/douyin-live/abogus_pure.py
// No Chromium.

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	mrand "math/rand"
	"strings"
	"time"

	"github.com/tjfoc/gmsm/sm3"
)

const (
	s3                               = "ckdp1h4ZKsUB80/Mfvw36XIgR25+WQAlEi7NLboqYTOPuzmFjJnryx9HVGDaStCe"
	s4                               = "Dkdpgh2ZmsQB80/MfvV36XI1R45-WUAlEixNLwoqYTOPuzKFjJnry79HbGcaStCe"
	salt                             = "dhzx"
	timeBaseMS                       = int64(1_721_836_800_000)
	rc4KeyByte                       = byte(211)
	magic                            = 41
	versionStr                       = "1.0.1.20"
	defaultScreen                    = "1366|768|1366|768|1366|768|1366|768|Win32"
	m145, m110, m66, m189, m44, m211 = 145, 110, 66, 189, 44, 211
	aa, bb                           = 170, 85
)

var packOrder = []int{
	34, 44, 56, 61, 73, 29, 70, 45, 35, 49, 38, 66, 51, 68, 28, 48, 64, 47, 30,
	71, 26, 55, 31, 69, 59, 40, 62, 63, 27, 72, 41, 74, 57, 52, 42, 39, 33, 67,
	53, 43, 65, 46, 36, 24, 60, 32, 79, 80, 84, 85,
}

var oracleSticky = map[int]int{
	27: 6, 28: 3, 35: 5, 36: 0, 38: 1, 39: 0, 40: 0, 41: 0, 42: 0, 43: 0,
	44: 14, 45: 0, 46: 0, 47: 0, 56: 48, 57: 240, 59: 249, 60: 255, 61: 19,
	62: 9, 63: 204, 64: 146, 65: 1, 66: 3, 67: 44, 68: 157, 69: 0, 70: 0,
	71: 31, 72: 8, 73: 0, 74: 0,
}

var stickyProtected = map[int]struct{}{
	48: {}, 49: {}, 51: {}, 52: {}, 53: {}, 55: {},
	29: {}, 30: {}, 31: {}, 32: {}, 33: {}, 34: {}, 24: {},
	26: {}, 79: {}, 80: {}, 84: {}, 85: {},
}

type floatRNG interface{ Float64() float64 }

// SignABogus pure local algo.
func SignABogus(query, body string) (string, error) {
	return GenerateABogus(query, body, userAgent, 0, 0), nil
}

func GenerateABogus(query, body, ua string, tsMS, seed int64) string {
	if ua == "" {
		ua = userAgent
	}
	if tsMS == 0 {
		tsMS = time.Now().UnixMilli()
	}
	rng := newRNG(seed)
	body93, verNums, L := buildPayload(query, body, ua, tsMS, defaultScreen, mrand.New(mrand.NewSource(0)), oracleSticky)
	verG := fn147GarbleVersion(verNums, rng)
	chk := xorChecksum(verG, L)
	payload94 := append(append([]int{}, body93...), chk)
	prefix := fn146Garble2([]int{3, 82}, 1, rng, "Chrome")
	payG := fn148Garble3(payload94, rng)
	rc4In := append(append([]int{}, verG...), payG...)
	rc4Out := rc4Variant([]byte{rc4KeyByte}, rc4In)
	raw := append(append([]int{}, prefix...), rc4Out...)
	return b64Encode(raw, s4, true)
}

func newRNG(seed int64) *mrand.Rand {
	if seed == 0 {
		var b [8]byte
		if _, err := rand.Read(b[:]); err == nil {
			seed = int64(binary.LittleEndian.Uint64(b[:]))
		} else {
			seed = time.Now().UnixNano()
		}
	}
	return mrand.New(mrand.NewSource(seed))
}

func sm3Bytes(data []byte) []int {
	h := sm3.Sm3Sum(data)
	out := make([]int, len(h))
	for i, v := range h {
		out[i] = int(v)
	}
	return out
}

func sm3DoubleSalt(s string) []int {
	inner := sm3Bytes([]byte(s + salt))
	buf := make([]byte, len(inner))
	for i, v := range inner {
		buf[i] = byte(v)
	}
	return sm3Bytes(buf)
}

func b64Encode(data []int, table string, forcePad bool) string {
	alphabet := strings.TrimRight(table, "=")
	pad := forcePad || strings.Contains(table, "=")
	var out strings.Builder
	n := len(data)
	for i := 0; i < n; i += 3 {
		b0 := data[i]
		var b1, b2 *int
		if i+1 < n {
			v := data[i+1]
			b1 = &v
		}
		if i+2 < n {
			v := data[i+2]
			b2 = &v
		}
		out.WriteByte(alphabet[(b0>>2)&63])
		if b1 == nil {
			out.WriteByte(alphabet[(b0&3)<<4])
			if pad {
				out.WriteString("==")
			}
		} else if b2 == nil {
			out.WriteByte(alphabet[(((b0 & 3) << 4) | (*b1 >> 4))])
			out.WriteByte(alphabet[(*b1&15)<<2])
			if pad {
				out.WriteByte('=')
			}
		} else {
			out.WriteByte(alphabet[(((b0 & 3) << 4) | (*b1 >> 4))])
			out.WriteByte(alphabet[(((*b1 & 15) << 2) | (*b2 >> 6))])
			out.WriteByte(alphabet[*b2&63])
		}
	}
	return out.String()
}

func rc4Variant(key []byte, data []int) []int {
	S := make([]int, 256)
	for i := 0; i < 256; i++ {
		S[i] = 255 - i
	}
	j := 0
	for i := 0; i < 256; i++ {
		j = (j*S[i] + j + int(key[i%len(key)])) % 256
		S[i], S[j] = S[j], S[i]
	}
	i := 0
	j = 0
	out := make([]int, len(data))
	for k, b := range data {
		i = (i + 1) % 256
		j = (j + S[i]) % 256
		S[i], S[j] = S[j], S[i]
		out[k] = b ^ S[(S[i]+S[j])%256]
	}
	return out
}

func fn144Rand(rng floatRNG) int {
	x := int(rng.Float64() * 240)
	if x > 109 {
		return x + (x % 2) + 1
	}
	return x
}

func fn143BrowserRand(name string, rng floatRNG) int {
	r := int(rng.Float64() * 40)
	switch name {
	case "Chrome":
		return r
	case "Firefox":
		return r + 40
	case "Safari":
		return r + 81
	case "Edge":
		return r + 125
	case "Huawei":
		return r + 170
	default:
		return r + 210
	}
}

func fn145EnvFlags(rng floatRNG) int {
	v := int(rng.Float64()*255) & 77
	v |= 1 << 1
	v |= 1 << 4
	v |= 1 << 5
	v |= 1 << 7
	return v & 255
}

func fn146Garble2(pair []int, mode int, rng floatRNG, browser string) []int {
	a0, a1 := pair[0]&255, pair[1]&255
	rnd := int(rng.Float64() * 65535)
	r0, r1 := rnd&255, (rnd>>8)&255
	switch mode {
	case 1:
		r1 = fn143BrowserRand(browser, rng)
	case 2:
		r0 = fn144Rand(rng)
		r1 = fn145EnvFlags(rng)
	}
	return []int{
		(r0 & aa) | (a0 & bb),
		(r0 & bb) | (a0 & aa),
		(r1 & aa) | (a1 & bb),
		(r1 & bb) | (a1 & aa),
	}
}

func fn147GarbleVersion(arr4 []int, rng floatRNG) []int {
	a := fn146Garble2([]int{arr4[0], arr4[1]}, 0, rng, "Chrome")
	b := fn146Garble2([]int{arr4[2], arr4[3]}, 2, rng, "Chrome")
	return append(a, b...)
}

func fn148Garble3(data []int, rng floatRNG) []int {
	out := make([]int, 0, (len(data)/3)*4+2)
	i, n := 0, len(data)
	for i < n {
		if i+2 < n {
			rnd := int(rng.Float64()*1000) & 255
			d0, d1, d2 := data[i], data[i+1], data[i+2]
			out = append(out,
				(rnd&m145)|(d0&m110),
				(rnd&m66)|(d1&m189),
				(rnd&m44)|(d2&m211),
				(d0&m145)|(d1&m66)|(d2&m44),
			)
			i += 3
		} else {
			out = append(out, data[i])
			if i+1 < n && data[i+1] != 0 {
				out = append(out, data[i+1])
			}
			break
		}
	}
	return out
}

func pickHash(h []int, start, avoid, fb int) int {
	i := start
	for i < len(h) && h[i] == avoid {
		i++
	}
	if i < len(h) {
		return h[i]
	}
	return fb
}

func timeDiffPeriods(ms int64) int {
	if ms < timeBaseMS {
		return 0
	}
	return int((ms - timeBaseMS) / 1000 / 60 / 60 / 24 / 14)
}

func bytesToInts(b []byte) []int {
	out := make([]int, len(b))
	for i, v := range b {
		out[i] = int(v)
	}
	return out
}

func buildPayload(query, body, ua string, tsMS int64, screen string, rng floatRNG, sticky map[int]int) (body93, verNums []int, L map[int]int) {
	uh := sm3DoubleSalt(query)
	var bh []int
	if body != "" {
		bh = sm3DoubleSalt(body)
	} else {
		bh = make([]int, 32)
	}
	uaB64 := b64Encode(bytesToInts([]byte(ua)), s3, true)
	uaH := sm3Bytes([]byte(uaB64))
	verNums = []int{}
	for _, p := range strings.Split(versionStr, ".") {
		var n int
		fmt.Sscanf(p, "%d", &n)
		verNums = append(verNums, n)
	}
	for len(verNums) < 4 {
		verNums = append(verNums, 0)
	}
	verNums = verNums[:4]
	env4 := fn145EnvFlags(rng)
	br := fn143BrowserRand("Chrome", rng)
	ink := (tsMS - 1) & ((1 << 48) - 1)
	L = map[int]int{}
	L[24] = magic
	L[26] = timeDiffPeriods(tsMS) & 255
	L[27] = br & 255
	L[28] = 2
	L[29] = int(tsMS) & 255
	L[30] = int(tsMS>>8) & 255
	L[31] = int(tsMS>>16) & 255
	L[32] = int(tsMS>>24) & 255
	L[33] = int(uint64(tsMS)/(256*256*256*256)) & 255
	L[34] = int(uint64(tsMS)/(256*256*256*256*256)) & 255
	L[35], L[36] = 0, 0
	L[38] = env4 & 255
	L[39] = 0
	L[40], L[41], L[42], L[43] = 0, 0, 0, 0
	L[44], L[45], L[46], L[47] = 0, 0, 0, 0
	L[48], L[49] = uh[9], uh[18]
	L[51] = pickHash(uh, 3, 11, 12)
	if body != "" {
		L[52], L[53], L[55] = bh[10], bh[19], pickHash(bh, 4, 8, 9)
	}
	L[56], L[57] = uaH[11], uaH[21]
	L[59] = pickHash(uaH, 5, 12, 13)
	for i := 0; i < 6; i++ {
		L[60+i] = int(ink>>(8*i)) & 255
	}
	L[66] = 3
	for i := 67; i <= 74; i++ {
		L[i] = 0
	}
	scr := []byte(screen)
	tc := []byte(fmt.Sprintf("%d,", (tsMS+3)&255))
	L[79], L[80] = len(scr)&255, (len(scr)>>8)&255
	L[84], L[85] = len(tc)&255, (len(tc)>>8)&255
	if sticky != nil {
		for k, v := range sticky {
			if _, ok := stickyProtected[k]; ok {
				continue
			}
			L[k] = v
		}
	}
	head := make([]int, len(packOrder))
	for i, idx := range packOrder {
		head[i] = L[idx] & 255
	}
	body93 = append(head, bytesToInts(scr)...)
	body93 = append(body93, bytesToInts(tc)...)
	return body93, verNums, L
}

func xorChecksum(verG []int, L map[int]int) int {
	parts := append([]int{}, verG...)
	for _, k := range []int{
		24, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 38, 39, 40, 41, 42, 43,
		44, 45, 46, 47, 48, 49, 51, 52, 53, 55, 56, 57, 59, 60, 61, 62, 63, 64,
		65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 79, 80, 84, 85,
	} {
		parts = append(parts, L[k]&255)
	}
	c := 0
	for _, x := range parts {
		c ^= x & 255
	}
	return c & 255
}

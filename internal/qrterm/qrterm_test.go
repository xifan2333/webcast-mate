package qrterm

import (
	"bytes"
	"strings"
	"testing"
)

func TestFprintSquareish(t *testing.T) {
	var buf bytes.Buffer
	url := "https://account.bilibili.com/h5/account-h5/auth/scan-web?qrcode_key=test"
	if err := Fprint(&buf, url); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) < 10 {
		t.Fatalf("too few lines: %d", len(lines))
	}
	w := len([]rune(lines[0]))
	h := len(lines)
	// half-block packing → height ≈ width/2 * 2 modules… roughly width ≈ height in cells
	// allow some slack
	if w < 20 || h < 10 {
		t.Fatalf("tiny qr w=%d h=%d", w, h)
	}
	// Half-blocks: ~2 modules per row → cell aspect ≈ 2:1, which looks square
	// on typical terminals (cell height ≈ 2× cell width).
	ratio := float64(w) / float64(h)
	if ratio < 1.5 || ratio > 2.5 {
		t.Fatalf("bad half-block aspect w=%d h=%d ratio=%.2f\n%s", w, h, ratio, buf.String())
	}
}

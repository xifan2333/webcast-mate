// Package qrterm prints scannable QR codes in a terminal with correct aspect ratio.
//
// Terminal cells are typically taller than wide. Drawing one QR module per full
// cell stretches the code vertically. We pack two vertical modules into one
// cell using Unicode half-block characters so the printed code is approximately
// square and phone cameras can read it.
package qrterm

import (
	"fmt"
	"io"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// Fprint writes a terminal QR for content to w (usually os.Stderr).
func Fprint(w io.Writer, content string) error {
	if content == "" {
		return nil
	}
	q, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return err
	}
	// Disable border in library; we add our own quiet zone of 2 modules.
	q.DisableBorder = true
	bits := q.Bitmap() // includes no border when DisableBorder
	if len(bits) == 0 {
		return fmt.Errorf("empty qr bitmap")
	}

	const quiet = 2
	n := len(bits)
	size := n + quiet*2

	// at returns true if module is dark (with quiet zone padding)
	at := func(x, y int) bool {
		x -= quiet
		y -= quiet
		if x < 0 || y < 0 || x >= n || y >= n {
			return false
		}
		return bits[y][x]
	}

	var b strings.Builder
	// process two rows at a time
	for y := 0; y < size; y += 2 {
		for x := 0; x < size; x++ {
			top := at(x, y)
			bot := false
			if y+1 < size {
				bot = at(x, y+1)
			}
			switch {
			case top && bot:
				b.WriteRune('█') // full block
			case top && !bot:
				b.WriteRune('▀') // upper half
			case !top && bot:
				b.WriteRune('▄') // lower half
			default:
				b.WriteRune(' ')
			}
		}
		b.WriteByte('\n')
	}
	_, err = io.WriteString(w, b.String())
	return err
}

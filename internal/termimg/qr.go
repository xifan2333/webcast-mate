// Package termimg displays images and QR codes in the terminal.
package termimg

import (
	"encoding/json"
	"os"
	"os/exec"

	qrcode "github.com/skip2/go-qrcode"
)

// ShowQR shows a QR code and returns a close func that kills the viewer.
//  1. write PNG to a temp file and open imv / xdg-open
//  2. else kitty graphics if stderr is a TTY and supports it
//  3. else print the raw URL as a JSONL event
func ShowQR(png []byte, content string) (close func()) {
	close = func() {}
	if len(png) == 0 && content != "" {
		if q, err := qrcode.New(content, qrcode.Medium); err == nil {
			png, _ = q.PNG(360)
		}
	}
	if len(png) > 0 {
		if path, err := writeTempPNG(png); err == nil {
			if proc, ok := openViewer(path); ok {
				return func() {
					if proc != nil {
						_ = proc.Kill()
					}
				}
			}
		}
		if isTTY() && SupportsKitty() && WriteKittyPNG(os.Stderr, png) == nil {
			return close
		}
	}
	if content == "" {
		return close
	}
	// fallback: no image viewer / kitty — print the raw URL
	enc := json.NewEncoder(os.Stderr)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]any{"event": "qr_login", "url": content})
	return close
}

func isTTY() bool {
	fi, err := os.Stderr.Stat()
	return err == nil && (fi.Mode()&os.ModeCharDevice) != 0
}

func writeTempPNG(png []byte) (string, error) {
	f, err := os.CreateTemp("", "webcast-mate-qr-*.png")
	if err != nil {
		return "", err
	}
	path := f.Name()
	if _, err := f.Write(png); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return path, nil
}

func openViewer(path string) (*os.Process, bool) {
	for _, bin := range []string{"imv", "xdg-open"} {
		if p, err := exec.LookPath(bin); err == nil {
			cmd := exec.Command(p, path)
			cmd.Stdout = nil
			cmd.Stderr = nil
			if err := cmd.Start(); err == nil {
				return cmd.Process, true
			}
		}
	}
	return nil, false
}

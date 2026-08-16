// Package termimg displays images in terminals that support the Kitty graphics protocol
// (Kitty, WezTerm, Ghostty, etc.).
package termimg

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
)

// SupportsKitty reports whether this environment likely understands Kitty graphics.
func SupportsKitty() bool {
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return true
	}
	term := strings.ToLower(os.Getenv("TERM"))
	if strings.Contains(term, "kitty") || strings.Contains(term, "ghostty") || strings.Contains(term, "wezterm") {
		return true
	}
	switch strings.ToLower(os.Getenv("TERM_PROGRAM")) {
	case "wezterm", "ghostty", "kitty":
		return true
	}
	// WezTerm also sets WEZTERM_EXECUTABLE / WEZTERM_PANE
	if os.Getenv("WEZTERM_EXECUTABLE") != "" || os.Getenv("WEZTERM_PANE") != "" {
		return true
	}
	if os.Getenv("GHOSTTY_RESOURCES_DIR") != "" {
		return true
	}
	return false
}

// WriteKittyPNG transmits a PNG via Kitty graphics protocol (a=T display).
// Data is chunked per protocol (4096-byte base64 chunks).
func WriteKittyPNG(w io.Writer, png []byte) error {
	if len(png) == 0 {
		return fmt.Errorf("empty png")
	}
	b64 := base64.StdEncoding.EncodeToString(png)
	const chunk = 4096
	for i := 0; i < len(b64); i += chunk {
		end := i + chunk
		more := 1
		if end >= len(b64) {
			end = len(b64)
			more = 0
		}
		part := b64[i:end]
		var err error
		if i == 0 {
			// f=100 PNG; a=T transmit and display; m=1 more chunks follow
			_, err = fmt.Fprintf(w, "\033_Ga=T,f=100,m=%d;%s\033\\", more, part)
		} else {
			_, err = fmt.Fprintf(w, "\033_Gm=%d;%s\033\\", more, part)
		}
		if err != nil {
			return err
		}
	}
	// move to next line after image
	_, err := io.WriteString(w, "\n")
	return err
}

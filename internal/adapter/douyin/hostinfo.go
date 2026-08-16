package douyin

// Local hardware fingerprint for device_register header.
// Uses only the Go standard library (DMI sysfs, /etc/machine-id, net.Interfaces).
//
// Wire still claims Windows/PC (companion protocol). Hardware fields are this host.

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// hostFingerprint is stable machine identity used in device_register.
type hostFingerprint struct {
	DeviceModel string
	PCUUID      string
	PCSerial    string
	MAC         string
	Resolution  string
}

func localHostFingerprint() hostFingerprint {
	return hostFingerprint{
		DeviceModel: firstNonEmpty(readTrim("/sys/class/dmi/id/product_name"), "PC"),
		PCUUID:      localPCUUID(),
		PCSerial:    localPCSerial(),
		MAC:         localMAC(),
		Resolution:  localResolution(),
	}
}

func localPCUUID() string {
	// Prefer SMBIOS UUID when readable (often root-only on Linux).
	if u := readTrim("/sys/class/dmi/id/product_uuid"); looksLikeUUID(u) {
		return strings.ToUpper(u)
	}
	// /etc/machine-id is 32 hex; companion Wine path used the same value as UUID.
	if id := readTrim("/etc/machine-id"); len(id) == 32 && isHex(id) {
		return strings.ToUpper(id[0:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:32])
	}
	if id := readTrim("/var/lib/dbus/machine-id"); len(id) == 32 && isHex(id) {
		return strings.ToUpper(id[0:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:32])
	}
	return ""
}

func localPCSerial() string {
	for _, p := range []string{
		"/sys/class/dmi/id/product_serial",
		"/sys/class/dmi/id/board_serial",
		"/sys/class/dmi/id/chassis_serial",
	} {
		s := readTrim(p)
		if s == "" || s == "None" || s == "To be filled by O.E.M." || s == "Default string" {
			continue
		}
		return s
	}
	// No root / empty DMI: stable per-machine serial (not shared "fake_serialNum",
	// not random-each-run). Derived from machine-id so register fingerprints stick.
	return derivedSerial()
}

// derivedSerial makes a BIOS-like serial from machine-id (or uuid material).
// Format: 16 uppercase hex — looks random, stable on one host.
func derivedSerial() string {
	seed := firstNonEmpty(
		readTrim("/etc/machine-id"),
		readTrim("/var/lib/dbus/machine-id"),
		localPCUUID(),
		localMAC(),
	)
	if seed == "" {
		// last resort: still unique-ish per process lifetime only
		sum := sha256.Sum256([]byte("webcast-mate-empty-host"))
		return strings.ToUpper(hex.EncodeToString(sum[:8]))
	}
	sum := sha256.Sum256([]byte("webcast-mate-pc-serial:" + seed))
	return strings.ToUpper(hex.EncodeToString(sum[:8]))
}

func localMAC() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	var fallback string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		name := strings.ToLower(iface.Name)
		if skipIfaceName(name) {
			continue
		}
		hw := iface.HardwareAddr
		if len(hw) != 6 {
			continue
		}
		// skip all-zero
		zero := true
		for _, b := range hw {
			if b != 0 {
				zero = false
				break
			}
		}
		if zero {
			continue
		}
		mac := strings.ToLower(hw.String())
		// prefer physical NICs
		if strings.HasPrefix(name, "en") || strings.HasPrefix(name, "eth") ||
			strings.HasPrefix(name, "wl") || strings.HasPrefix(name, "ww") {
			return mac
		}
		if fallback == "" {
			fallback = mac
		}
	}
	return fallback
}

func skipIfaceName(name string) bool {
	prefixes := []string{
		"docker", "br-", "veth", "virbr", "cni", "flannel", "tun", "tap",
		"wg", "zt", "tailscale", "nerdctl", "podman",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return name == "lo"
}

func localResolution() string {
	// DRM connector modes: first line is often preferred mode "1366x768"
	matches, _ := filepath.Glob("/sys/class/drm/*/modes")
	for _, p := range matches {
		// skip loose "card0/modes" style if any; want card0-HDMI-A-1/modes etc.
		base := filepath.Base(filepath.Dir(p))
		if !strings.Contains(base, "-") {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// modes look like 1920x1080 or 1920x1080i
			line = strings.TrimRight(line, "iI")
			if strings.Contains(line, "x") {
				return line
			}
		}
	}
	return "1920x1080"
}

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func looksLikeUUID(s string) bool {
	// 8-4-4-4-12
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !isHexByte(byte(c)) {
				return false
			}
		}
	}
	return true
}

func isHex(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isHexByte(s[i]) {
			return false
		}
	}
	return true
}

func isHexByte(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

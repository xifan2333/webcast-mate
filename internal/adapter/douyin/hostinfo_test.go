package douyin

import (
	"strings"
	"testing"
)

func TestLocalHostFingerprint(t *testing.T) {
	h := localHostFingerprint()
	if h.DeviceModel == "" {
		t.Fatal("device_model empty")
	}
	if h.PCUUID == "" {
		t.Fatal("pc_uuid empty")
	}
	if !looksLikeUUID(h.PCUUID) {
		t.Fatalf("pc_uuid not uuid-shaped: %q", h.PCUUID)
	}
	if h.PCSerial == "" {
		t.Fatal("pc_serial empty")
	}
	if h.PCSerial == "fake_serialNum" {
		t.Fatal("pc_serial still using shared fake fallback")
	}
	if h2 := localHostFingerprint(); h2.PCSerial != h.PCSerial {
		t.Fatalf("pc_serial unstable: %q vs %q", h.PCSerial, h2.PCSerial)
	}
	if h.MAC == "" {
		t.Fatal("mc empty")
	}
	if strings.Count(h.MAC, ":") != 5 {
		t.Fatalf("mc format: %q", h.MAC)
	}
	if !strings.Contains(h.Resolution, "x") {
		t.Fatalf("resolution: %q", h.Resolution)
	}
	t.Logf("model=%s uuid=%s serial=%s mac=%s res=%s",
		h.DeviceModel, h.PCUUID, h.PCSerial, h.MAC, h.Resolution)
}

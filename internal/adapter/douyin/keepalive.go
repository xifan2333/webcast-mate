package douyin

// Background keepalive for douyin live (SPEC §5.4).
//
//	every 5s:  ping/anchor status=2
//	every 5m:  passport/token/beat/web/
//
// start detaches child with WEBCAST_MATE_INTERNAL=douyin-keepalive.

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/xifan2333/webcast-mate/internal/conv"

	"github.com/xifan2333/webcast-mate/internal/appdir"
	"github.com/xifan2333/webcast-mate/internal/live"
	"github.com/xifan2333/webcast-mate/internal/platform"
	"github.com/xifan2333/webcast-mate/internal/secrets"
)

const (
	envKeepalive      = "WEBCAST_MATE_INTERNAL"
	envKeepaliveVal   = "douyin-keepalive"
	pingInterval      = 5 * time.Second
	tokenBeatInterval = 5 * time.Minute
)

func StartKeepalive() error {
	_ = StopKeepalive()
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("keepalive exe: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	logPath, err := appdir.DouyinKeepaliveLog()
	if err != nil {
		return err
	}
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer logf.Close()

	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), envKeepalive+"="+envKeepaliveVal)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = logf, logf, nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("keepalive start: %w", err)
	}
	pidPath, err := appdir.DouyinKeepalivePID()
	if err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0o600); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	go func() { _ = cmd.Process.Release() }()
	return nil
}

func StopKeepalive() error {
	pidPath, err := appdir.DouyinKeepalivePID()
	if err != nil {
		return err
	}
	b, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		_ = os.Remove(pidPath)
		return nil
	}
	if proc, err := os.FindProcess(pid); err == nil && proc != nil {
		_ = proc.Signal(syscall.SIGTERM)
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if err := proc.Signal(syscall.Signal(0)); err != nil {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		_ = proc.Signal(syscall.SIGKILL)
	}
	_ = os.Remove(pidPath)
	return nil
}

func IsKeepaliveChild() bool {
	return os.Getenv(envKeepalive) == envKeepaliveVal
}

func RunKeepalive() int {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGTERM, syscall.SIGINT)

	cli := NewClient()
	if s, e := secrets.Load(platform.Douyin); e == nil {
		cli.ApplySecrets(s)
	}
	_ = keepaliveOnce(cli, true)
	lastBeat := time.Now()
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sigc:
			return 0
		case <-ticker.C:
			if s, e := secrets.Load(platform.Douyin); e == nil && s.HasAuth() {
				cli.ApplySecrets(s)
			}
			needBeat := time.Since(lastBeat) >= tokenBeatInterval
			if err := keepaliveOnce(cli, needBeat); err != nil {
				if err == errNotLive {
					return 0
				}
				continue
			}
			if needBeat {
				lastBeat = time.Now()
			}
		}
	}
}

var errNotLive = fmt.Errorf("douyin not live")

func keepaliveOnce(cli *Client, doBeat bool) error {
	t, ok := live.Get(platform.Douyin)
	if !ok || t.RoomID == "" || t.StreamID == "" || (t.Server == "" && t.Key == "") {
		return errNotLive
	}
	if err := cli.PingAnchor(t.RoomID, t.StreamID, RoomLiving); err != nil {
		return fmt.Errorf("ping LIVING: %w", err)
	}
	if doBeat {
		_ = cli.TokenBeat()
	}
	return nil
}

func (c *Client) TokenBeat() error {
	q := c.passportQuery()
	qs, err := withABogus(q.Encode(), "")
	if err != nil {
		return err
	}
	m, err := c.getJSON(hostStreaming+"/passport/token/beat/web/?"+qs, nil)
	if err != nil {
		return err
	}
	if msg, _ := m["message"].(string); msg != "" && msg != "success" {
		if ec := conv.AnyInt(m["error_code"]); ec != 0 && ec != 7 {
			return fmt.Errorf("token/beat message=%s err=%d", msg, ec)
		}
	}
	if sc, ok := m["status_code"].(float64); ok && sc != 0 {
		return fmt.Errorf("token/beat status_code=%v", sc)
	}
	return nil
}

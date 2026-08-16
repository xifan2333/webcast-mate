package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/xifan2333/webcast-mate/internal/adapter"
	"github.com/xifan2333/webcast-mate/internal/adapter/bilibili"
	"github.com/xifan2333/webcast-mate/internal/adapter/stub"
	"github.com/xifan2333/webcast-mate/internal/appcfg"
	"github.com/xifan2333/webcast-mate/internal/appdir"
	"github.com/xifan2333/webcast-mate/internal/live"
	"github.com/xifan2333/webcast-mate/internal/platform"
	"github.com/xifan2333/webcast-mate/internal/secrets"
)

// version is overridden at link time:
//
//	go build -ldflags "-X main.version=v0.1.0" ./cmd/webcast-mate
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		printHelp()
		return 0
	}
	switch args[0] {
	case "-h", "--help", "help":
		printHelp()
		return 0
	case "-v", "--version", "version":
		fmt.Println("webcast-mate", version)
		return 0
	case "start":
		return cmdStart(args[1:])
	case "stop":
		return cmdStop(args[1:])
	case "status":
		return cmdStatus(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
		printHelp()
		return 2
	}
}

func printHelp() {
	fmt.Printf(`Work with multi-platform live streaming protocol (no browser).

USAGE
  webcast-mate
  webcast-mate <command> <platform>

COMMANDS
  start  <platform>  Go live: session + RTMP + update live.json
  stop   <platform>  End live (idempotent)
  status <platform>  Current session + push fields (same shape as start)

  help               Show this help
  version            Show version

FLAGS
  -y, --yes          Non-interactive (use saved config / defaults)
  -h, --help         Show this help
  -v, --version      Show version

PLATFORMS
  bilibili  douyin  xiaohongshu

OUTPUT
  start / status   one JSON line: platform, room_id, cookie, server, key
  stop             one JSON line: platform, room_id, status

  status is live when server+key are set (from live.json); otherwise idle
  (cookie still filled from secrets when present).

  Diagnostics go to stderr. Pipe stdout to jq.

EXAMPLES
  $ webcast-mate start bilibili
  $ webcast-mate start bilibili -y
  $ out=$(webcast-mate start douyin)
  $ echo "$out" | jq -r .server
  $ webcast-mate stop douyin
  $ webcast-mate status bilibili | jq .

CONFIG
  $XDG_CONFIG_HOME/webcast-mate/
    config.yaml           preferences (room, title, 分区, bitrate)
    secrets/<platform>.json   cookies (0600)
    live.json             active push targets for capture
  (root: %s)

STATUS
  bilibili: QR login + huh prompts (title/area/cover) + start/stop; -y skips prompts
  douyin / xiaohongshu: stub
`, configRootDisplay())
}

func registry() *adapter.Registry {
	return adapter.NewRegistry(
		bilibili.New(),
		stub.New(platform.Douyin),
		stub.New(platform.XiaoHongShu),
	)
}

func cmdStart(args []string) int {
	if hasHelp(args) {
		fmt.Print(`Go live on a platform.

Interactive (default, TTY): prompt room, title, category, cover,
then go live. Use -y to skip prompts and use saved config.

USAGE
  webcast-mate start <platform> [-y]

FLAGS
  -y, --yes   Non-interactive

OUTPUT
  {"platform":"…","room_id":"…","cookie":"…","server":"…","key":"…"}
`)
		return 0
	}
	yes, rest := stripYes(args)
	id, code := parsePlatformArg(rest, "start")
	if code != 0 {
		return code
	}
	a, ok := registry().Get(id)
	if !ok {
		fmt.Fprintf(os.Stderr, "no adapter for %s\n", id)
		return 1
	}
	ctx := context.Background()
	res, err := a.Start(ctx, adapter.StartOpts{Yes: yes})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitCode(err)
	}
	return printJSON(res)
}

func cmdStatus(args []string) int {
	if hasHelp(args) {
		fmt.Print(`Show current live session for a platform.

Same JSON fields as start success (platform, room_id, cookie, server, key).
Reads live.json + secrets; does not call remote APIs.

USAGE
  webcast-mate status <platform>

OUTPUT
  {"platform":"…","room_id":"…","cookie":"…","server":"…","key":"…","status":"live|idle"}
`)
		return 0
	}
	id, code := parsePlatformArg(args, "status")
	if code != 0 {
		return code
	}
	out := statusResult{Platform: string(id), Status: "idle"}

	if t, ok := live.Get(string(id)); ok && (t.Server != "" || t.Key != "") {
		out.RoomID = t.RoomID
		out.Server = t.Server
		out.Key = t.Key
		out.Status = "live"
	}
	if s, err := secrets.Load(string(id)); err == nil && s != nil {
		out.Cookie = s.Cookie
	}
	// fill room_id from config when not live
	if out.RoomID == "" {
		if f, err := appcfg.Load(); err == nil {
			out.RoomID = f.GetPlatform(string(id)).RoomID
		}
	}
	return printJSON(out)
}

type statusResult struct {
	Platform string `json:"platform"`
	RoomID   string `json:"room_id"`
	Cookie   string `json:"cookie"`
	Server   string `json:"server"`
	Key      string `json:"key"`
	Status   string `json:"status"` // live | idle
}

func cmdStop(args []string) int {
	if hasHelp(args) {
		fmt.Print(`End live on a platform.

Idempotent: no active room still exits 0.

USAGE
  webcast-mate stop <platform>

OUTPUT
  {"platform":"…","room_id":"…","status":"stopped"}
`)
		return 0
	}
	id, code := parsePlatformArg(args, "stop")
	if code != 0 {
		return code
	}
	a, ok := registry().Get(id)
	if !ok {
		fmt.Fprintf(os.Stderr, "no adapter for %s\n", id)
		return 1
	}
	res, err := a.Stop(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if res.Status == "" {
		res.Status = "stopped"
	}
	return printJSON(res)
}

func stripYes(args []string) (yes bool, rest []string) {
	for _, a := range args {
		switch a {
		case "-y", "--yes":
			yes = true
		default:
			rest = append(rest, a)
		}
	}
	return yes, rest
}

func parsePlatformArg(args []string, cmd string) (platform.ID, int) {
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "usage: webcast-mate %s <platform>\n", cmd)
		return "", 2
	}
	id, ok := platform.Parse(args[0])
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown platform %q\n", args[0])
		return "", 2
	}
	return id, 0
}

func printJSON(v any) int {
	// Do not HTML-escape (& → \u0026); stream keys must stay raw for conf/scripts.
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func hasHelp(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" || a == "help" {
			return true
		}
	}
	return false
}

func configRootDisplay() string {
	p, err := appdirRoot()
	if err != nil || p == "" {
		return "$XDG_CONFIG_HOME/webcast-mate"
	}
	return p
}

func appdirRoot() (string, error) {
	return appdir.Root()
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "not configured"):
		return 2
	case strings.Contains(s, "not logged in"):
		return 3
	case strings.Contains(s, "not implemented"):
		return 1
	case strings.Contains(s, "qrcode"), strings.Contains(s, "timeout"), strings.Contains(s, "face auth"):
		return 5
	default:
		return 1
	}
}

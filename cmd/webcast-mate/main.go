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
	"github.com/xifan2333/webcast-mate/internal/conf"
	"github.com/xifan2333/webcast-mate/internal/platform"
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
  start <platform>   Go live: session + RTMP + write platforms.conf
  stop  <platform>   End live (idempotent)

  help               Show this help
  version            Show version

FLAGS
  -y, --yes          Non-interactive (use saved config / defaults)
  -h, --help         Show this help
  -v, --version      Show version

PLATFORMS
  bilibili  douyin  xiaohongshu

OUTPUT
  start   one JSON line: platform, room_id, cookie, server, key
  stop    one JSON line: platform, room_id, status

  Diagnostics go to stderr. Pipe stdout to jq.

EXAMPLES
  $ webcast-mate start bilibili
  $ webcast-mate start bilibili -y
  $ out=$(webcast-mate start douyin)
  $ echo "$out" | jq -r .server
  $ webcast-mate stop douyin

CONFIG
  Session/state:  $XDG_CONFIG_HOME/webcast-mate/
  Push conf:      ~/.config/livestream/platforms.conf
  (currently: %s)

STATUS
  bilibili: QR login + huh prompts (title/area/cover) + start/stop; -y skips prompts
  douyin / xiaohongshu: stub
`, confPathDisplay())
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
	if res.Server != "" || res.Key != "" {
		if err := writeConf(string(id), res.Server, res.Key); err != nil {
			fmt.Fprintf(os.Stderr, "write conf: %v\n", err)
			return 10
		}
	}
	return printJSON(res)
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

func writeConf(platformID, server, key string) error {
	path := conf.DefaultPath()
	f, err := conf.Load(path)
	if err != nil {
		return err
	}
	// default bitrates per SPEC
	vbr, abr := "4000", "128"
	if platformID == "bilibili" {
		vbr = "3200"
	}
	f.UpsertServerKey(platformID, server, key, vbr, abr)
	return f.Write(path)
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

func confPathDisplay() string {
	p := conf.DefaultPath()
	if p == "" {
		return "$HOME/.config/livestream/platforms.conf"
	}
	return p
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


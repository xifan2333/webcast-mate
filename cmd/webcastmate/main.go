package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/xifan2333/webcast-mate/internal/adapter"
	"github.com/xifan2333/webcast-mate/internal/adapter/bilibili"
	"github.com/xifan2333/webcast-mate/internal/adapter/douyin"
	"github.com/xifan2333/webcast-mate/internal/adapter/xiaohongshu"
	"github.com/xifan2333/webcast-mate/internal/appdir"
	"github.com/xifan2333/webcast-mate/internal/platform"
)

var version = "dev"

func main() {
	// detached douyin keepalive child (SPEC §5.4)
	if douyin.IsKeepaliveChild() {
		os.Exit(douyin.RunKeepalive())
	}
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
		fmt.Println("webcastmate", version)
		return 0
	case "login", "logout", "start", "stop", "status":
		return runCommand(args[0], args[1:])
	default:
		return fail(2, args[0], "", fmt.Sprintf("unknown command %q", args[0]))
	}
}

// cmdSpec is one subcommand. run performs the adapter call and returns the
// stdout payload; runCommand adds the common ok/command/platform fields.
type cmdSpec struct {
	help    string
	withYes bool // start-style -y flag
	run     func(a adapter.Adapter, opts adapter.StartOpts) (map[string]any, error)
}

var commands = map[string]cmdSpec{
	"login": {
		help: `login <platform> — ensure session (QR/CAS if needed), write secrets.

OUTPUT (stdout JSONL)
  {"ok":true,"command":"login","platform":"…","user_id":"…","user_name":"…","cookies":{…},"headers":{…},"params":{…},"login_at":"…"}
`,
		run: func(a adapter.Adapter, _ adapter.StartOpts) (map[string]any, error) {
			res, err := a.Login(context.Background())
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"user_id": res.UserID, "user_name": res.UserName,
				"cookies": res.Cookies, "headers": res.Headers, "params": res.Params,
				"login_at": res.LoginAt,
			}, nil
		},
	},
	"logout": {
		help: `logout <platform> — clear local secrets (idempotent).

OUTPUT (stdout JSONL)
  {"ok":true,"command":"logout","platform":"…","status":"logged_out"}
`,
		run: func(a adapter.Adapter, _ adapter.StartOpts) (map[string]any, error) {
			res, err := a.Logout(context.Background())
			if err != nil {
				return nil, err
			}
			st := res.Status
			if st == "" {
				st = "logged_out"
			}
			return map[string]any{"status": st}, nil
		},
	},
	"start": {
		help: `start <platform> [-y] — go live.

OUTPUT (stdout JSONL)
  {"ok":true,"command":"start","platform":"…","room_id":"…","cookies":{…},"headers":{…},"params":{…},"server":"…","key":"…"}
`,
		withYes: true,
		run: func(a adapter.Adapter, opts adapter.StartOpts) (map[string]any, error) {
			res, err := a.Start(context.Background(), opts)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"room_id": res.RoomID,
				"cookies": res.Cookies, "headers": res.Headers, "params": res.Params,
				"server": res.Server, "key": res.Key,
			}, nil
		},
	},
	"status": {
		help: `status <platform> — query live state + push fields.

OUTPUT (stdout JSONL)
  {"ok":true,"command":"status","platform":"…","room_id":"…","cookies":{…},"headers":{…},"params":{…},"server":"…","key":"…","status":"live|idle|round"}
`,
		run: func(a adapter.Adapter, _ adapter.StartOpts) (map[string]any, error) {
			res, err := a.Status(context.Background())
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"room_id": res.RoomID,
				"cookies": res.Cookies, "headers": res.Headers, "params": res.Params,
				"server": res.Server, "key": res.Key, "status": res.Status,
			}, nil
		},
	},
	"stop": {
		help: `stop <platform> — end live (idempotent).

OUTPUT (stdout JSONL)
  {"ok":true,"command":"stop","platform":"…","room_id":"…","status":"stopped"}
`,
		run: func(a adapter.Adapter, _ adapter.StartOpts) (map[string]any, error) {
			res, err := a.Stop(context.Background())
			if err != nil {
				return nil, err
			}
			st := res.Status
			if st == "" {
				st = "stopped"
			}
			return map[string]any{"room_id": res.RoomID, "status": st}, nil
		},
	},
}

// runCommand dispatches one subcommand: parse platform → resolve adapter →
// run → emit one JSONL result. All five commands share this shape.
func runCommand(name string, args []string) int {
	spec, ok := commands[name]
	if !ok {
		return fail(2, name, "", fmt.Sprintf("unknown command %q", name))
	}
	if hasHelp(args) {
		fmt.Fprint(os.Stderr, spec.help)
		return 0
	}
	yes := false
	rest := args
	if spec.withYes {
		yes, rest = stripYes(args)
	}
	id, code := parsePlatformArg(rest, name)
	if code != 0 {
		return code
	}
	a, ok := registry().Get(id)
	if !ok {
		return fail(1, name, string(id), "no adapter")
	}
	res, err := spec.run(a, adapter.StartOpts{Yes: yes})
	if err != nil {
		return fail(exitCode(err), name, string(id), err.Error())
	}
	res["ok"] = true
	res["command"] = name
	res["platform"] = id
	return out(res)
}

func printHelp() {
	fmt.Fprintf(os.Stderr, `webcastmate — multi-platform live protocol CLI (no browser)

USAGE
  webcastmate <command> <platform> [-y]

COMMANDS
  login   <platform>   ensure session (QR if needed) → secrets
  logout  <platform>   clear local secrets (idempotent)
  start   <platform>   go live → one JSONL result on stdout
  stop    <platform>   end live (idempotent)
  status  <platform>   room + push fields
  version              tool version (plain text)
  help                 this text (plain text, stderr)

FLAGS
  -y, --yes            non-interactive (saved config / defaults)

PLATFORMS
  bilibili  douyin  xiaohongshu

STDOUT (data commands: one JSONL object, SetEscapeHTML=false)
  login/start/status carry auth as cookies/headers/params buckets:
    {"ok":true,"command":"login","platform":"…","user_id":"…","user_name":"…",
     "cookies":{…},"headers":{…},"params":{…},"login_at":"…"}
    {"ok":true,"command":"start","platform":"…","room_id":"…",
     "cookies":{…},"headers":{…},"params":{…},"server":"…","key":"…"}
    {"ok":true,"command":"stop","platform":"…","room_id":"…","status":"stopped"}
    {"ok":false,"command":"start","platform":"douyin","error":"…","code":3}

STDERR
  QR graphics (plain) + JSONL diagnostics (e.g. face auth). help is plain. No progress spam.

EXAMPLES
  webcastmate login bilibili | jq -c '{ok,user_name,user_id}'
  webcastmate start bilibili -y | jq -r .server
  webcastmate status douyin | jq -c .
  webcastmate stop xiaohongshu | jq -e .ok
  webcastmate logout douyin | jq -c .

CONFIG
  %s/
    config.yaml
    secrets/<platform>.json
    live.json
    run/                     # douyin keepalive pid/log
`, configRootDisplay())
}

func registry() *adapter.Registry {
	return adapter.NewRegistry(
		bilibili.New(),
		douyin.New(),
		xiaohongshu.New(),
	)
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
		fail(2, cmd, "", "usage: webcastmate "+cmd+" <platform>")
		return "", 2
	}
	id, ok := platform.Parse(args[0])
	if !ok {
		fail(2, cmd, args[0], fmt.Sprintf("unknown platform %q", args[0]))
		return "", 2
	}
	return id, 0
}

// out writes one JSONL object to stdout, no HTML escaping.
func out(v any) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return 1
	}
	return 0
}

// fail writes one failure JSONL to stdout (jq-safe) and returns the code.
func fail(code int, command, platform, msg string) int {
	_ = out(map[string]any{
		"ok": false, "command": command, "platform": platform, "error": msg, "code": code,
	})
	return code
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
	p, err := appdir.Root()
	if err != nil || p == "" {
		return "$XDG_CONFIG_HOME/webcast-mate"
	}
	return p
}

func exitCode(err error) int {
	return adapter.ExitCode(err)
}

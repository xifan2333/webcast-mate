# webcastmate

Multi-platform **live protocol** CLI (name aligned with Douyin `webcast_mate` / StreamingTool).

- **Does**: session + go-live RTMP + write `~/.config/webcastmate/live.json` for capture + stop
- **Does not**: capture / encode / push (use `capture-router` + `gpu-screen-recorder`)
- **Constraint**: **no browser** for login or open
- **Language**: Go
- **Pipe**: stdout JSON feeds scripts → UniBarrage / dmnotifier; conf feeds gsr

## Docs

| Doc | Content |
|-----|---------|
| [docs/SPEC.md](./docs/SPEC.md) | Boundary, pipe model, **CLI `start`/`stop`**, JSON fields, adapters |
| [docs/protocol-platforms.md](./docs/protocol-platforms.md) | Platform HTTP details |
| [docs/protocol-xhs-danmaku.md](./docs/protocol-xhs-danmaku.md) | Xiaohongshu danmaku (UniBarrage) |
| [docs/pipeline.md](./docs/pipeline.md) | How the pieces fit together (capture-router / dmnotifier / herdr) |

## CLI (SPEC §5)

```bash
webcastmate start <platform>   # → JSON: platform, room_id, cookie, server, key
webcastmate stop  <platform>   # → JSON: platform, room_id, status
webcastmate -h | -v
```

Platforms (no aliases): `bilibili` `douyin` `xiaohongshu`

```bash
out=$(webcastmate start douyin)
echo "$out" | jq -r .server
echo "$out" | jq -r .key
# optional danmaku
dmnotifier start "$(echo "$out" | jq -r '[.platform,.room_id,.cookie]|join(":")')"
```

Douyin differs internally: `create` + periodic `ping LIVING` keepalive; `stop` sends FINISH. Same CLI surface.

## Pipe

```text
webcastmate start ──► stdout JSON + platforms.conf
        │                      │
        │                      └─► capture-router / gsr (video)
        └─► script ──► dmnotifier start platform:rid:cookie ──► UniBarrage
webcastmate stop  ──► protocol end (+ douyin keepalive stop)
```

## Config layout

```text
~/.config/webcastmate/
  config.yaml                 # room / title / 分区 (no secrets)
  secrets/<platform>.json     # cookies only (0600)
  live.json                   # active RTMP targets — capture reads this
```

Capture (`livestream-service`) loads `live.json` only. No `platforms.conf`.

## Build

```bash
go build -o webcastmate ./cmd/webcastmate
go test ./...
```

## Status

| Piece | State |
|-------|--------|
| SPEC CLI `start`/`stop` + default JSON | done |
| Go module + CLI skeleton + conf R/W | done |
| `bilibili` adapter | QR login + startLive/stopLive + face auth |
| `douyin` | streamingtool QR login; create_info + a_bogus + create; ping LIVING/FINISH; status |
| `xiaohongshu` | live-helper 4.4.0: CAS QR → AT → redobs pre/start/stop; distribute 0/1 |

`a_bogus` for douyin: pure-Go (bdms 1.0.1.20 port), no chromium.

Next: reverse 2026 小红书直播伴侣 for start/stop/status.

### bilibili setup

```bash
# interactive (npm-init style): room / title / area / cover
webcastmate start bilibili

# non-interactive: use saved config
webcastmate start bilibili -y

webcastmate stop bilibili
```

Config is saved to `~/.config/webcastmate/bilibili/config.yaml`.
Cover accepts an existing `*.hdslb.com` URL (local file upload TBD).

## Reference

- Douyin protocol: `docs/protocol-douyin-companion.md`
- Bilibili: `~/Code/userscripts` live helper

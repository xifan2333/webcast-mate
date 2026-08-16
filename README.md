# webcast-mate

Multi-platform **live protocol** CLI (name aligned with Douyin `webcast_mate` / StreamingTool).

- **Does**: session + go-live RTMP + write `~/.config/webcast-mate/live.json` for capture + stop
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

## CLI (SPEC §5)

```bash
webcast-mate start <platform>   # → JSON: platform, room_id, cookie, server, key
webcast-mate stop  <platform>   # → JSON: platform, room_id, status
webcast-mate -h | -v
```

Platforms (no aliases): `bilibili` `douyin` `xiaohongshu`

```bash
out=$(webcast-mate start douyin)
echo "$out" | jq -r .server
echo "$out" | jq -r .key
# optional danmaku
dmnotifier start "$(echo "$out" | jq -r '[.platform,.room_id,.cookie]|join(":")')"
```

Douyin differs internally: `create` + periodic `ping LIVING` keepalive; `stop` sends FINISH. Same CLI surface.

## Pipe

```text
webcast-mate start ──► stdout JSON + platforms.conf
        │                      │
        │                      └─► capture-router / gsr (video)
        └─► script ──► dmnotifier start platform:rid:cookie ──► UniBarrage
webcast-mate stop  ──► protocol end (+ douyin keepalive stop)
```

## Config layout

```text
~/.config/webcast-mate/
  config.yaml                 # room / title / 分区 / bitrate (no secrets)
  secrets/<platform>.json     # cookies only (0600)
  live.json                   # active RTMP targets — capture reads this
```

Capture (`livestream-service`) loads `live.json` only. No `platforms.conf`.

## Build

```bash
go build -o webcast-mate ./cmd/webcast-mate
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

`a_bogus` for douyin: needs chromium on PATH (bdms 1.0.1.20), or `WEBCAST_MATE_DY_ABOGUS` / `WEBCAST_MATE_DY_ABOGUS_CMD`.

Next: reverse 2026 小红书直播伴侣 for start/stop/status; pure a_bogus without chromium when ready.

### bilibili setup

```bash
# interactive (npm-init style): room / title / area / cover
webcast-mate start bilibili

# non-interactive: use saved config
webcast-mate start bilibili -y

webcast-mate stop bilibili
```

Config is saved to `~/.config/webcast-mate/bilibili/config.yaml`.
Cover accepts an existing `*.hdslb.com` URL (local file upload TBD).

## Reference

- Douyin Python: `~/douyin-live` (`login.py` / `pure_create.py`)
- Bilibili: `~/Code/userscripts` live helper

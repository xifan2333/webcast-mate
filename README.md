# webcast-mate

Multi-platform **live protocol** CLI (name aligned with Douyin `webcast_mate` / StreamingTool).

- **Does**: session + go-live RTMP + write `~/.config/livestream/platforms.conf` + stop
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
| Platform adapters | **stub** (`start` → not implemented) |

Next: bilibili adapter → xiaohongshu → douyin (+ keepalive).

## Reference

- Douyin Python: `~/douyin-live` (`login.py` / `pure_create.py`)
- Bilibili: `~/Code/userscripts` live helper

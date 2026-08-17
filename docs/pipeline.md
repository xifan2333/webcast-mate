# 直播配合手册

> 本文是**操作手册**，讲清 `webcastmate` 与 `capture-router`、`dmnotifier`、`UniBarrage`、`gpu-screen-recorder`、`herdr` 之间如何配合完成一场直播。设计与契约见 [SPEC.md](./SPEC.md)。

## 1. 组件与职责

| 组件 | 职责 | 命令 |
|------|------|------|
| **webcastmate** | 协议层：登录 → 开播拿 RTMP → 写 `live.json` → 关播 | `webcastmate <login\|start\|stop\|status> <platform>` |
| **capture-router / livestream-service** | 推流编排：起停 `gpu-screen-recorder` | `capture-router livestream start\|stop\|status` |
| **gpu-screen-recorder** | 画面采集 + 编码（被 capture-router 调用） | — |
| **dmnotifier** | 弹幕监听客户端（消费 UniBarrage WS） | `dmnotifier start\|stop\|list` |
| **UniBarrage** | 弹幕采集转发（API `:8080` / WS `:7777`） | `unibarrage` |
| **herdr** | 终端 workspace 管理（放各环节的终端） | `herdr workspace ...` |

**分工原则**：每个程序只做一段，靠「标准接口」串成链，不互相 import、不共享内存。

## 2. 数据流

```text
webcastmate start <platform>
        │
        ├─► stdout JSON ──► server / key / room_id / cookies
        │
        └─► ~/.config/webcastmate/live.json   ← capture-router 读这里
                 │
                 ▼
        capture-router livestream start ──► gpu-screen-recorder -o <rtmp>
                 │
                 └─► 画面推流

webcastmate start ──► stdout JSON ──► dmnotifier start platform:rid:cookie
                                            │
                                            └─► UniBarrage（弹幕采集）
```

## 3. 配置目录

```text
~/.config/webcastmate/
  config.yaml                 # 偏好（title/area），无密钥
  secrets/<platform>.json     # 登录 cookie（0600）
  live.json                   # 当前推流目标，capture-router 只读这个
  run/                        # douyin keepalive 的 pid / log
```

- `start` 成功会写 `live.json`；`stop` 会清掉对应平台段（空则删文件）。
- `live.json` 是「本场 RTMP 真相」：`server` + `key` + `room_id`。

## 4. 开播流程（完整）

```bash
# 0) 首次先登录（扫码；之后可复用 secrets）
webcastmate login douyin

# 1) 开播：拿 RTMP + 写 live.json，stdout 是一行 JSONL
out=$(webcastmate start douyin -y)
echo "$out" | jq .

#   {"ok":true,"command":"start","platform":"douyin","room_id":"…",
#    "cookies":{…},"headers":{…},"params":{…},"server":"rtmp://…","key":"…"}

# 2) 推流（capture-router 读 live.json，起 gsr）
capture-router livestream start

# 3) 弹幕（可选）：把 cookies 拼成 cookie 串喂 dmnotifier
rid=$(echo "$out" | jq -r .room_id)
cookie=$(echo "$out" | jq -r '.cookies | to_entries | map("\(.key)=\(.value)") | join("; ")')
dmnotifier start "douyin:${rid}:${cookie}"
```

**要点**：

- `-y` = 非交互，用已保存的 config；去掉 `-y` 会交互式询问 title/area。
- stdout 是**单行 JSONL**，可直接 `jq` 消费；进度/诊断走 stderr。
- `start` 成功才写 `live.json`，失败不会写半截码。

## 5. 关播流程

```bash
webcastmate stop douyin     # 协议关播（douyin 会停保活 + ping FINISH）
capture-router livestream stop   # 停推流
dmnotifier stop douyin:<rid>     # 停弹幕监听（如有）
```

`stop` 幂等：没有房间也返回成功（`{"ok":true,...,"status":"stopped"}`）。

## 6. 检查状态

```bash
# 是否在播（读 live.json + 远端状态）
webcastmate status douyin | jq .

# 推流侧状态
capture-router livestream status
```

## 7. herdr workspace

用 herdr 把直播相关终端聚到一个 workspace：

```bash
# 新建（cwd 指向 capture 侧源码；不抢焦点）
herdr workspace create --label live --cwd ~/Code/arch-post-install --no-focus

# 切过去
herdr workspace focus live
```

建议 layout：

| 窗口 | 用途 |
|------|------|
| `live` workspace | `capture-router livestream start/stop`、`dmnotifier`、`unibarrage` |
| `webcast-mate` workspace | 协议侧 `webcastmate start/stop/status`、看代码 |

## 8. 命令速查

```text
webcastmate login|logout|start|stop|status <platform>   # platform: bilibili|douyin|xiaohongshu
webcastmate start <platform> [-y]

capture-router livestream start|stop|toggle|status [target]

dmnotifier start|stop|list <platform:rid[:cookie]>...

unibarrage     # API :8080  WS :7777
```

## 9. 平台差异

| 平台 | 开播 | 关播 | 备注 |
|------|------|------|------|
| bilibili | `startLive` | `stopLive` | 可能触发人脸验证（60024，打开浏览器扫码） |
| douyin | `create_info` + `create` + ping LIVING | 停保活 + ping FINISH | 开播后有后台保活进程（`run/`） |
| xiaohongshu | CAS QR → AT → `pre/before/start` | `stop` | `distribute` 0/1（是否公开） |

## 10. 退出码（webcastmate）

| code | 含义 |
|------|------|
| 0 | 成功 |
| 1 | 其它运行时错误 |
| 2 | 用法 / 配置错误 |
| 3 | 未登录 / 会话失效 |
| 4 | 网络 / 上游 API 错误 |
| 5 | 交互未完成（超时、取消扫码） |
| 6 | 风控 / 验证未通过 |
| 10 | 写 conf 失败 |

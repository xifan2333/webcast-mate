# webcastmate · 产品与扩展规范

> 状态：**规范阶段**（无实现要求）。  
> 实现语言倾向：**Go**（单文件、标准库 HTTP；性能对协议 CLI 足够）。  
> 协议细节：[`protocol-platforms.md`](./protocol-platforms.md)。  
> 抖音伴侣真链路：[protocol-douyin-companion.md](./protocol-douyin-companion.md)。抓包工具可放 `~/douyin-live`（仅 mitm，非运行时）。

---

## 1. 一句话

**多平台「直播伴侣」协议层 CLI**：登录 → 开播拿到本场 RTMP → 写入本机推流 conf → 关播。  
**不负责**采集与编码；推流继续用已有 `gpu-screen-recorder` + `capture-router`。  
**核心是「管道的一节」**：产出会话/房间/推流码，喂给下游 `UniBarrage`（弹幕）、`dmnotifier`（展示）、gsr（画面）。

官方抖音客户端标识：

| 概念 | 值 |
|------|-----|
| 产品/域名 | StreamingTool · `streamingtool.douyin.com` |
| 包名 / app_name | `webcast_mate` |
| 本仓库名 | `webcast-mate`（对齐包名，多平台共用此名） |

---

## 2. 管道与协作（核心哲学）

> 不造大而全的直播套件。每个程序只做一段，靠**标准接口**串成链：
> `webcastmate` 管「会话+房间+推流码」，`UniBarrage` 管「弹幕采集转发」，
> `dmnotifier` 管「通知/TUI/TTS」，gsr 管「画面」。
>
> **三者均为自有项目**（webcastmate 待建；UniBarrage、dmnotifier 已在用），
> 管道契约可随需求协同演进，不必迁就第三方上游。

### 2.1 原则

- **单一职责**：每个程序一个输入、一个输出、一个状态文件。
- **接口优先**：程序间用 HTTP / WebSocket / 文件 / stdout，不互相 import、不共享内存。
- **stdout 只出数据，stderr 出进度**：命令可被 shell 管道或 `jq` 消费。
- **本工具是「源头」**：登录出会话，开播出房间+推流码；下游按需订阅。
- **自有闭环**：下游能力不够就加（如 UniBarrage 加 xhs 弹幕），但接口契约先行、不破坏已通链路。

### 2.2 一条直播的完整管道

```text
                   ┌─ 画面链 ──────────────────────────────┐
webcastmate       │ start 写 server/key                    │
  start <platform> ┤→ ~/.config/webcastmate/live.json   │
  (会话+开播+码) ──┤→ capture-router livestream start        │
                   ┤→ gpu-screen-recorder -o <url>          │
         │         └───────────────────────────────────────┘
         │
         │  stdout JSON: platform, room_id, cookie, server, key
         │  ┌─ 弹幕链（可选，默认不自动调）────────────────┐
         └─►│ 脚本: dmnotifier start platform:rid:cookie   │
            │ UniBarrage ← dmnotifier / 手工 POST          │
            └────────────────────────────────────────────┘

stop <platform> → 协议关播（抖音: 停保活 + ping FINISH）
```

### 2.3 与 UniBarrage 的衔接契约

UniBarrage 已装（`/usr/bin/unibarrage`，默认 `API :8080`、`WS :7777`）。它需要的输入正好是本工具产出：

| UniBarrage 字段 | 来源（webcastmate） |
|-----------------|----------------------|
| `platform` | platform id（`bilibili`/`douyin`/`xiaohongshu`，无别名） |
| `rid` | `start` 返回的 `room_id` |
| `cookie` | `start` 返回的 cookie（bili/dy 整段；**xhs 固定空**，弹幕另取浏览器） |

HTTP 接口（见其 README）：

```text
POST   http://127.0.0.1:8080/{platform}        body: {rid, cookie}   → 201
DELETE http://127.0.0.1:8080/{platform}/{rid}                          → 200
GET    http://127.0.0.1:8080/all / {platform} / {platform}/{rid}
```

**本工具的职责**：把 `start`/`stop` 与上述 API 对齐，但**默认不自动调用** UniBarrage（见 2.4）。

> UniBarrage 现有：douyin / bilibili / kuaishou / douyu / huya / **xiaohongshu**。
> 弹幕协议见 [protocol-xhs-danmaku.md](./protocol-xhs-danmaku.md)。

### 2.4 弹幕链（管道可裁剪）

| 形态 | 行为 |
|------|------|
| 默认 | `start`/`stop` 只管协议与 conf，**不动** UniBarrage / dmnotifier |
| 脚本 | 用 `start` 的 JSON 拼 `dmnotifier start platform:rid:cookie` |

### 2.5 shell 管道友好

- **`start`/`stop` 默认 stdout = compact 单行 JSON**（无需 `--json`；可 `jq`）。
- 诊断/扫码提示走 **stderr**。
- 后续可加 `webcastmate events` 输出 **NDJSON 事件流**（start/stop 状态），
  供 shell `while read` / 其它程序订阅——不阻塞本工具主流程。
- 密钥/日志仍走 stderr 或 `0600` 文件，**不**进 stdout 污染管道。

### 2.6 下游演进需求（跨项目，均为自有）

管道契约会随下游演进，以下两项是**后续要改的下游项目**，webcastmate 接口需预先兼容：

#### 2.6.1 dmnotifier：多订阅 + CLI 传参

现状：单一 `ws_address` + `pipeline.plugins`，从 `~/.dmnotifier/config.yaml` 读。
目标：

```text
dmnotifier --ws ws://127.0.0.1:7777 \
           --subscribe douyin:123456,bilibili:789012 \
           --plugins notify,tts \
           --no-tui
```

- **多订阅**：一次启动消费多个 `platform:rid`（UniBarrage WS 消息已带 `platform`/`rid`，客户端按需过滤）。
- **CLI 传参启动**：不再只读 config.yaml；参数可覆盖配置文件，缺省回落文件。
- **对 webcastmate 的要求**：`start` JSON 含 `platform`/`room_id`/`cookie`，脚本拼 `dmnotifier start platform:rid:cookie`。

#### 2.6.2 capture-router：生命周期 hook

现状：`capture-router livestream start|stop` 直接起/停 `livestream-service`（gsr），无 hook。
目标：

```text
~/.config/livestream/hooks/
  pre_start    # livestream start 前执行（可调 webcastmate start）
  post_stop    # livestream stop 后执行（可调 webcastmate stop）
```

- **hook 是编排入口**：画面与协议的启停解耦——capture 管 gsr，hook 里调 `webcastmate`。
- **对 webcastmate 的要求**：`start`/`stop` 必须**幂等、可被 hook 反复调用**（`stop` 无房间视为成功）；
  hook 调用时用 stdout JSON + 退出码，不靠人读文案。
- 与 2.4 不冲突：hook（画面侧编排）与 `--barrage`（弹幕侧编排）可独立启用。

#### 2.6.3 顺序与归属

| 改动 | 归属仓库 | 依赖 |
|------|----------|------|
| dmnotifier CLI | dmnotifier | 已落地 `start/stop/list`；mate 输出 cookie/room 即可 |
| capture hook | arch-post-install | 无；webcastmate 只需幂等命令 |
| UniBarrage 加 xhs | UniBarrage | xhs 弹幕协议文档 |

> 三处下游改动**不阻塞** webcastmate P1–P4；只在 P5（管道接通）前需定型接口。

---

## 3. 非目标（明确不做）

| 不做 | 原因 |
|------|------|
| 内置推流 / 改 gsr | 本机已有 livestream 栈；一场内 URL 静态，gsr `-o` 足够 |
| 为动态 URL 改 gsr 热切换 | 不需要：码在开播时生成，推流期间不变 |
| 依赖 **浏览器 / Chromium / WebView** 完成登录或开播 | 产品约束；见 §7 开放问题（抖音 a_bogus） |
| 依赖 Wine / 官方伴侣 GUI | 仅作历史对照 |
| **自行采集/解析弹幕** | 交给 UniBarrage（见 §2），不重复造 |
| 替 UniBarrage 做 TUI/通知/TTS | 交给 dmnotifier |
| 代替 OBS 做导播 | 只产出 RTMP server+key |

---

## 4. 与本机推流栈的契约

```
webcastmate                          已有栈（arch-post-install）
────────────                          ────────────────────────
start / stop                          live.json
        │                                    │
        │  start 成功后写入                    │
        └─────► ~/.config/webcastmate/live.json
                                             │
                                             ▼
                                  capture-router livestream start
                                             │
                                             ▼
                                  gpu-screen-recorder -c flv -o <url>
```

### 4.1 `live.json` 段约定（静态，兼容现网）

```ini
[bilibili]   # 段名 = platform id（小写英文）
server = rtmp://…
key = …

[douyin]
server = rtmp://…
key = …
…

[xiaohongshu]
server = rtmp://live-push.xhscdn.com/live
key = …
…
```

- **只使用现有字段**：`server` + `key`。不引入 `url_command` 等动态字段（已否决）。  
- `start`：更新对应段的 `server`/`key`。  
- `stop`：**只做协议关播**，不强制改 conf；是否清空 key 由实现可选（建议保留上次码便于排查，并在文档说明已失效）。  
- 推流启停：**用户或脚本**调用 `capture-router`，不强制 `webcastmate` 绑死 gsr。

### 4.2 一场一码

| 时机 | URL |
|------|-----|
| 同一次 start 到 stop 之间 | **不变** |
| 再次 start | **必须重新拉码并写 conf** |

---


## 5. CLI 表面（稳定契约）

二进制名：`webcastmate`。

```text
webcastmate
webcastmate <command> <platform>
webcastmate help | version | -h | --help | -v | --version
```

### 5.1 platform id（稳定，小写，**无别名**）

| id | 显示名 | conf 段 |
|----|--------|--------|
| `bilibili` | B 站 | `[bilibili]` |
| `douyin` | 抖音 | `[douyin]` |
| `xiaohongshu` | 小红书 | `[xiaohongshu]` |

不接受 `xhs` / `dy` / `bili` 等别名（与 UniBarrage / dmnotifier 一致）。

### 5.2 命令（第一版）

| 命令 | 语义 | 退出 |
|------|------|------|
| `start <platform>` | 确保会话 → 开播拿 RTMP → 写 conf → **stdout JSON**；抖音另启 LIVING 保活 | 0 |
| `stop <platform>` | 协议关播；抖音停保活 + FINISH；无房间视为成功（幂等） | 0 |
| `help` / `-h` / `--help` | 英文帮助 | 0 |
| `version` / `-v` / `--version` | 版本 | 0 |

无子命令时打印 help。参数**只有** `platform`（房间号/标题等进 XDG 配置，不堆 CLI flag）。

**第一版不做**：独立 `login`/`open`/`logout`/`status`/`platforms`；默认调 UniBarrage/capture-router。

### 5.3 机器输出（**默认 JSON**）

- **stdout**：仅业务结果，默认 **compact 单行 JSON**（无需 `--json`）
- **stderr**：进度、扫码提示、诊断
- 可选后置：`--pretty`；不默认 TSV

#### `start` 成功（字段固定）

```json
{"platform":"douyin","room_id":"7123…","cookie":"ttwid=…; sessionid=…","server":"rtmp://…/game/","key":"stream-xxxx"}
```

| 字段 | 说明 |
|------|------|
| `platform` | 规范 id |
| `room_id` | 本场房间号 |
| `cookie` | 弹幕用会话 Cookie；bili/dy = secrets；**xhs 固定 `""`**（浏览器另取） |
| `server` | 推流服务器（不含 key），写入 conf |
| `key` | 推流密钥，写入 conf |

```bash
out=$(webcastmate start douyin)
echo "$out" | jq -r .server
echo "$out" | jq -r .key
dmnotifier start "$(echo "$out" | jq -r '[.platform,.room_id,.cookie]|join(":")')"
```

#### `stop` 成功

```json
{"platform":"douyin","room_id":"7123…","status":"stopped"}
```

### 5.4 抖音保活（实现细节，不暴露子命令）

与 bili/xhs 不同：`create` 后必须 `ping/anchor status=2`（LIVING），过程中需周期性 LIVING ping；关播 `status=4`（FINISH）。

| 步骤 | 行为 |
|------|------|
| `start douyin` | create → ping LIVING → 写 conf → **后台保活** → stdout JSON 后命令可返回 |
| 保活 | 按间隔 `ping LIVING`；pid/state 写 XDG |
| `stop douyin` | 停保活 → ping FINISH → stdout stop JSON |

B 站 / 小红书：`start` 为短命令（开播 + 写 conf 即返回）。

### 5.5 退出码

| code | 含义 |
|------|------|
| 0 | 成功 |
| 1 | 其它运行时错误 |
| 2 | 用法错误（未知命令/平台） |
| 3 | 未登录 / 会话失效 |
| 4 | 网络/上游 API 错误 |
| 5 | 交互未完成（超时、取消扫码） |
| 6 | 风控/验证未通过 |
| 10 | 写 conf 失败 |

---

## 6. 扩展模型（多平台）

### 6.1 原则

- **Platform Adapter** 是唯一扩展点。  
- 核心只依赖接口，不 `if platform ==` 散落业务。  
- 每个平台：自有会话存储、自有 API、自有错误映射到 §5.4。  
- 共享能力：HTTP client（Cookie 罐）、conf 读写、二维码展示（终端/文件）、密钥文件权限 `0600`。

### 6.2 Adapter 接口（语言无关）

实现侧（Go 伪代码）应具备等价能力：

```text
type PlatformID string  // "bilibili" | "douyin" | "xiaohongshu"

type Session interface {
    Valid(ctx) (bool, error)
    Clear() error
    // 可选: Export cookies 路径
}

type OpenRequest struct {
    Title   string
    // 平台扩展字段放 map 或具体类型（由 CLI 层解析 flag 注入）
    Extra   map[string]string
}

type OpenResult struct {
    RoomID  string
    Server  string   // rtmp://… 无 key 部分，或完整 URL 的 server 段
    Key     string   // stream key / path+query
    // 若平台只返回完整 URL：拆成 Server+Key 再写 conf；完整 URL 可放 RawPushURL
    RawPushURL string
    Title   string
    Extra   map[string]string
}

type Adapter interface {
    ID() PlatformID
    Login(ctx, opts) error          // 阻塞至完成或失败；可含扫码/短信
    Logout(ctx) error
    Status(ctx) (Status, error)
    Open(ctx, OpenRequest) (OpenResult, error)
    Stop(ctx) error                 // 幂等
}
```

### 6.3 会话存储（统一 secrets schema）

```text
~/.config/webcastmate/
  secrets/<platform>.json   # 0600；三家同一 schema
  live.json                 # 本场 RTMP 真相（server/key/room_id）
  config.yaml               # 非密钥偏好（标题、分区…）
```

`secrets/<platform>.json` 字段固定：

| 字段 | 类型 | 说明 |
|------|------|------|
| `cookie` | string | **开播鉴权**材料，统一写成 `k=v; k2=v2` 串 |
| `user_id` | string | 可选 |
| `user_name` | string | 可选 |
| `login_at` | RFC3339 | 可选 |

各平台 `cookie` 语义（都进同一字段，不另开 schema）：

| platform | `secrets.cookie` 内容 | stdout `cookie`（弹幕链） |
|----------|----------------------|---------------------------|
| bilibili | 浏览器会话 Cookie（`SESSDATA`/`bili_jct`…） | = secrets.cookie |
| douyin | 桌面会话 Cookie（`sessionid`/`ttwid`…） | = secrets.cookie |
| xiaohongshu | live-helper 开播凭证串：`access-token=AT-…; device-id=…; a1=…; webId=…` | **始终 `""`**（弹幕用浏览器 Cookie，不走 helper AT） |

- **不要**把会话写进 git 或世界可读路径。  
- room_id / server / key **不**进 secrets，只进 `live.json`。

### 6.4 conf 写入规则

1. 读现有 ini（保留其它段与注释策略：实现可选用“只改目标段键值，尽量保留文件其余部分”）。  
2. 设置 `server`、`key`。  
3. 文件权限：目录 `0700`，文件 `0600`。  
4. `open` 失败**不得**写入半截码。

### 6.5 新增平台清单

1. 在 `docs/protocol-platforms.md`（或 `docs/platforms/<id>.md`）写协议。  
2. 实现 Adapter + 注册到工厂。  
3. 补 `start`/`stop` 与集成测试（可用 mock HTTP）。  
4. 默认 conf 段名与 id 一致。

---

## 7. 硬约束：无浏览器

| 层级 | 要求 |
|------|------|
| 登录 | 禁止 Chromium/WebView/CDP 完成登录 |
| 开播/关播 | 禁止为发业务请求拉起浏览器 |
| 二维码 | 仅作**展示**（写 PNG / 终端渲染）；识别与确认在手机 App |
| HTTP | 自建 client（Go：`net/http` + CookieJar；可 impersonate TLS 若某平台需要） |

### 7.1 抖音 a_bogus（开放问题）

当前 Python 对照用 **Chromium + bdms.js** 生成 a_bogus，**违反**本规范的无浏览器约束。

可选方向（实现前必须在文档中选定其一）：

| 方案 | 说明 |
|------|------|
| A. 纯 Go 移植/嵌入 bdms | 无浏览器；工程量大，版本跟 CDN |
| B. 无头 JS 运行时（goja 等）跑官方 bdms | 无「浏览器」，仍依赖 JS 引擎与环境伪造 |
| C. 阶段性：start 调用本机已有签名服务（unix socket），服务可单独演进 | CLI 主体无浏览器；签名进程另算 |
| D. 暂缓 douyin start 的纯 Go，先做 bili/xhs | 产品可先用两平台 |

**规范要求**：合并进主路径的 `douyin` Adapter **不得** `exec chromium`。在解决前，可将 douyin 标为 `partial`（会话可有，start 未实现）。

---

## 8. 平台能力矩阵（规范目标）

| 能力 | bilibili | douyin | xiaohongshu |
|------|----------|--------|-------------|
| 无浏览器登录 | 扫码 Passport §协议文档 | 扫码 streamingtool | 网页会话（OBS 同源；**非 robs 短信**） |
| start 拿码 | startLive | create+ping LIVING + 保活 | 6 位码 → `obs/push_url` |
| stop | stopLive | 停保活+ping FINISH | 手机关播 + 清 live.json |
| 写 conf | ✓ | ✓ | ✓ |
| 额外交互 | 人脸 60024 扫码 | 登录确认频控 | 手机预直播 6 位码 |
| 无浏览器 start | ✓（纯 HTTP） | create 需 a_bogus（chromium+bdms 或外部 CMD） | 取码纯 HTTP；登录来源待钉死 |
| 弹幕链（UniBarrage） | ✓ | ✓ | ✓（xiaohongshu） |

---

## 9. 实现分期（建议，非日程）

| 阶段 | 内容 |
|------|------|
| P0 | SPEC + 协议文档；CLI = `start`/`stop` + 默认 JSON 五字段 |
| P1 | Go module + CLI 骨架 + conf 读写 + 空 Adapter |
| P2 | `bilibili`：会话 + start/stop + 人脸 |
| P3 | `xiaohongshu`：会话 + pre/start/stop |
| P4 | `douyin`：会话 + create/ping + **保活守护** + 无浏览器签名 |
| P5 | hook/脚本对接 capture-router；stdout 喂 dmnotifier |

对照：抖音见 [protocol-douyin-companion.md](./protocol-douyin-companion.md)；B 站可对 `userscripts` live helper；小红书见 helper/danmaku 协议文档。**不**把 Python 当运行时依赖。  
管道目标（P5）以 §2 为准：只调 HTTP API / 输出 NDJSON，不 import 下游程序。

---

## 10. 测试与质量

- 单元测试：conf 合并、URL 拆 server/key、错误码映射。  
- 集成测试：对 mock server；实网测试手动、不进 CI 密钥。  
- 禁止测试提交真实 Cookie/短信/推流 key。  
- `go build ./…`；release 用 GitHub Actions 多架构构建（Linux amd64/arm64，参考 dmnotifier）。

---

## 11. 文档索引

| 文档 | 内容 |
|------|------|
| [SPEC.md](./SPEC.md) | 本文件：产品边界、管道协作、CLI、Adapter、conf、无浏览器 |
| [protocol-platforms.md](./protocol-platforms.md) | 三平台 HTTP 级协议与状态机 |
| `~/douyin-live` | 可选 mitm 全量抓包工作区；协议以 `protocol-douyin-companion.md` 为准 |
| `~/.config/webcastmate/live.json` | 推流侧 conf（本工具的写入目标） |
| `capture-router` | 推流启停（本工具默认不替代） |
| `unibarrage` | 弹幕采集转发（API :8080 / WS :7777），管道弹幕链消费本工具 session+room |
| `dmnotifier` | `start/stop/list` + TUI；消费 UniBarrage WS |

---

## 12. 变更规则

- 改 CLI（`start`/`stop`）/退出码/platform id/JSON 字段/conf → 先改 SPEC，再实现。  
- 改某平台 URL/字段 → 先改 `protocol-platforms.md`。  
- 规范与实现冲突时，**以已合并的 SPEC 为准**，协议文档补齐后再改代码。

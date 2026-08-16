# 小红书直播助手 4.4.0 协议（静态 + 抓包）

> 来源  
> - 安装包：`xhs-live-helper_release_4.4.0_0730142007.exe`  
> - 静态：`resources/app/public/js/5522.466b291.js`（API 表 + 调用封装）  
> - 抓包：`/tmp/xhs_live_flows.jsonl`（2026-08-16 Wine + mitm，登录→pre→before/start）  
>
> **不做**旧 XiaoHongShu_OBS 短信 `sid` 主路径；4.4.0 主链是 **CAS 扫码 → robs ticket 换 access_token → redobs center/room/\***。

---

## 1. Host

| 角色 | Host (production) |
|------|-------------------|
| CAS 登录 | `https://customer.xiaohongshu.com` |
| 会话 / 旧 robs API | `https://robs.xiaohongshu.com` |
| 开停播中心（新） | `https://redobs.xiaohongshu.com` |
| 配置等 | `https://gemini-admin.xiaohongshu.com`、`https://ark.xiaohongshu.com` |

UA（助手）：

```
Mozilla/5.0 … live-helper/4.4.0 Chrome/118.0.5993.159 Electron/27.3.2 Safari/537.36
env/production platform/win32 appname/xhs-live …
```

公共 query / header 元数据（redobs）：

```
xy-common-params / xy-platform-info:
  platform=pc&build=4040000&version=4.4.0&isWin7=false
  &systemVersion=10.0.19045&cpuModel=…&gpu=…
```

签名：请求带 `x-s`、`x-t`（与 web x-s 同源算法族，app 侧 launcher anti-spam）。

---

## 2. 鉴权（access_token，不是短信 sid）

### 2.1 存储

```js
localStorage.access_token = accessToken   // robs /api/sns/login 返回
localStorage.userInfo = { userId, avatar, nickname, … }
```

### 2.2 请求注入（`/api/sns/` 前缀）

静态拦截器：

```js
if (url includes "/api/sns/") {
  headers["device-id"] = jsbridge.macAddress || ""
  if (access_token) {
    headers["auth"] = access_token
    headers["access-token"] = access_token
    headers["subsystem"] = <build constant>
  }
}
```

抓包里 cookie 往往只有 `xsecappid=…`；**真正会话在 header `auth` / `access-token`**（mitm 若未展开全部 header 时注意看 raw）。

---

## 3. 登录

### 3.1 CAS（customer）

| 步骤 | 方法 | Path | 说明 |
|------|------|------|------|
| 区号列表 | GET | `/api/cas/customer/pc/zones?service=https://robs.xiaohongshu.com` | |
| 出码 | POST | `/api/cas/customer/pc/qr-code` | body 含 service 等 |
| 轮询 | GET | `/api/cas/customer/pc/qr-code` | 同 id |

轮询 `data.status`（抓包）：

| status | 含义（结合抓包） |
|--------|------------------|
| （出码后） | 等待 |
| 2 | 已扫/确认中 |
| 3 | 过渡 |
| 成功包 | 带 `user_id` / avatar 等，随后换票 |

出码响应含 `data.url` → 手机确认页  
`https://customer.xiaohongshu.com/loginconfirm?…&qrCode=…`

### 3.2 换票（robs）

```http
POST https://robs.xiaohongshu.com/api/sns/login
Content-Type: application/json

{"ticket":"ST-…","service":"https://robs.xiaohongshu.com"}
```

成功：`data.access_token`（形如 `AT-…`）、`user_id`、`nickname`、`avatar`。

静态：`handleSsoSuccess(ticket)` → `AUTH_LOGIN` → `localStorage.access_token` → 进 Main。

### 3.3 会话检查

```http
GET https://robs.xiaohongshu.com/api/sns/check_login
→ { "result": 0, "success": true }
```

助手约 **10s** 轮询一次。

### 3.4 开播权限

```http
GET https://robs.xiaohongshu.com/api/sns/live/check_live
→ data.allow_live, detail.live_auth, allow_pc_obs, real_name_status, …
```

---

## 4. 开播状态机（redobs center）

静态封装（`appId` 恒 `"1"`）：

```js
pre()            → POST ROOM_PRE           { appId: "1" }
beforeStart(id)  → POST ROOM_BEFORE_START  { roomId, appId: "1" }
start(body)      → POST ROOM_START         { ...body, appId:"1", style: PC, joinLimit }
stop(id)         → POST ROOM_STOP          { roomId, appId: "1" }
reportPush(info) → POST REPORT_PUSH_INFO   // host = robs
obsPushUrl(id)   → GET  OBS_PUSH_URL       ?room_id=
streamInfo(id)   → GET  OBS_STREAM_EXISTS  ?room_id=
```

### 4.1 Path 常量（生产）

| 常量 | Method | Path | Host |
|------|--------|------|------|
| ROOM_PRE | POST | `/api/sns/redobs/live/app/v1/center/room/pre` | redobs |
| ROOM_BEFORE_START | POST | `/api/sns/redobs/live/app/v1/center/room/before/start` | redobs |
| **ROOM_START** | POST | `/api/sns/redobs/live/app/v1/center/room/start` | redobs |
| **ROOM_STOP** | POST | `/api/sns/redobs/live/app/v1/center/room/stop` | redobs |
| ROOM_LOCK_RELEASE | POST | `/api/sns/redobs/live/app/v1/center/room/lock/release` | redobs |
| REPORT_PUSH_INFO | POST | `/api/sns/live/room/report_push_info` | **robs** |
| ROOM_JOIN_CONFIG | GET | `/api/sns/red/live/redobs/base/v1/get_sdk_join_config` | redobs |
| OBS_PUSH_URL | GET | `/api/sns/redobs/live/app/v1/center/room/push_url` | redobs |
| OBS_STREAM_EXISTS | GET | `/api/sns/redobs/live/app/v1/center/room/get_stream_info` | redobs |
| LAST_ROOM_INFO | GET | `/api/sns/redobs/live/app/v1/room/last_room_info` | redobs |
| ROOM_AUTH | GET | `/api/sns/red/live/redobs/base/v1/room/room_auth` | redobs |
| STOP_INFO | GET | `/api/sns/live/stop_info` | robs |
| CHECK_LOGIN | GET | `/api/sns/check_login` | robs |
| CHECK_LIVE_AUTH | GET | `/api/sns/live/check_live` | robs |
| AUTH_LOGIN | POST | `/api/sns/login` | robs |

### 4.2 抓包已证实顺序（卡在「开播中」前）

```
1) POST redobs …/center/room/pre
   body: {"app_id":"1"}
   → room_id
   → stream_info.push_info … push_url =
        rtmp://live-push-hw.xhscdn.com/live/{room_id}?txSecret=…&txTime=…&vendor=hw
     （dispatch JSON 内嵌，多档码率）

2) POST redobs …/center/room/before/start
   body: {"room_id":"{id}","app_id":"1"}
   → { allow_start: true, not_allowed_reason: 0 }

3) POST robs …/live/room/report_push_info?build=4040000&platform=pc&…
   body: {
     "room_id":"…",
     "push_result": "{ codec, push_type:3, bitrate, resolution, fps, height, width,
                       push_url, camera_type, voice_type }"
   }
   → { result:0, success:true }

4) GET redobs …/get_sdk_join_config?room_id=&app_id=1
   → kasa_config.available_urls[] 多条 rtmp
   → trtc_config（可选）

5) 期望但本次抓包未出现：
   POST redobs …/center/room/start
   body 静态：{ ...业务字段, appId:"1", style: PC(=1), joinLimit }
```

UI「开播中」= 本地推流 / SDK 建连阶段；**真正的 ROOM_START 在推流就绪后由客户端再发**（本次卡在推流/设备，故未发出）。

### 4.3 ROOM_START 静态参数

```js
l("ROOM_START", {
  ...e,           // cover / title / distribute / categories 等业务字段
  appId: "1",
  style: PC,      // enum：PC = 1（LiveStudio=2）
  joinLimit: e.joinLimit
})
```

### 4.4 ROOM_STOP

```js
l("ROOM_STOP", { roomId, appId: "1" })
// POST https://redobs.xiaohongshu.com/api/sns/redobs/live/app/v1/center/room/stop
```

### 4.5 推流地址形态

```
server = rtmp://live-push-hw.xhscdn.com/live
       | rtmp://live-push.xhscdn.com/live
key    = {room_id}?txSecret=…&txTime=…&redExpire=…&vendor=hw|tencent
```

一场一码；`pre` 与 `get_sdk_join_config` 都可能给 URL（secret 可能刷新）。

---

## 5. 状态查询（给 webcast-mate status）

| 目的 | 接口 |
|------|------|
| 登录是否有效 | `GET robs/api/sns/check_login` → `result==0` |
| 是否允许开播 | `GET robs/api/sns/live/check_live` → `allow_live` |
| 上次/当前房 | `GET redobs/…/last_room_info` |
| 流是否在推 | `GET redobs/…/center/room/get_stream_info?room_id=` |
| 再取推流码 | `GET redobs/…/center/room/push_url?room_id=` |
| 关播后信息 | `GET robs/api/sns/live/stop_info?roomId=` |

---

## 6. 与旧文档对照

| | 旧 3.A（XiaoHongShu_OBS） | 助手 4.4.0（本文件） |
|--|--------------------------|---------------------|
| 登录 | 短信 → Header `sid` | CAS 扫码 → Header `auth`/`access-token` |
| 开播 | `GET live/pre` + `POST live/{id}/start` | `redobs center/room/pre` → `before/start` → **`start`** |
| 关播 | `POST live/{id}/stop` | `POST redobs …/center/room/stop` |
| 推流 | pre 的 `url.push_url` | pre / join_config / push_url |
| Host | 仅 robs | **customer + robs + redobs** |

网页 6 位码 OBS（`www` + `spectrum` + `push_url?code=`）仍是另一条产品线；助手 4.4.0 **不依赖**六位码。

---

## 7. webcast-mate 实现要点（待写 adapter）

```
login:
  CAS qr-code create/poll → ticket
  POST robs/api/sns/login {ticket, service: robs}
  secrets: access_token + user

start:
  POST redobs …/pre {app_id:1}
  parse push_url → server/key, room_id
  POST …/before/start {room_id, app_id:1}
  （可选）report_push_info
  POST …/start {room_id, app_id:1, style:1, title/cover/…}
  live.json

stop:
  POST redobs …/stop {room_id, app_id:1}
  clear live.json

status:
  check_login + check_live + get_stream_info/last_room_info
```

依赖：`x-s` 签名 + `device-id` + `access-token` 头；body 字段名注意 camelCase（`appId`）与抓包 `app_id` 混用，以实际 wire 为准（抓包 pre 为 `app_id`）。

---

## 8. 本地产物路径

| 文件 | 说明 |
|------|------|
| `~/.wine-xhs/drive_c/Program Files/xhs-live-helper/` | 4.4.0 安装 |
| `…/resources/app/public/js/5522.466b291.js` | API 与封装 |
| `/tmp/xhs_live_flows.jsonl` | 抓包全量 |
| `/tmp/xhs_live_capture.log` | 抓包摘要 |

---

## 9. 缺口

1. **ROOM_START / ROOM_STOP 完整成功响应** — 本次 UI 卡在开播中未发出 start；需再抓一轮或只靠静态 body。  
2. **`x-s` 是否与 web xhs-pc-web 完全同一套** — 助手 build/xy-common-params 不同，需对拍。  
3. **`subsystem` header 常量值** — 拦截器有，精确字符串待从包内枚举。  
4. **report_push_info 是否 start 前置必选** — 助手会发；纯协议可先 pre→before/start→start 最小集再补。

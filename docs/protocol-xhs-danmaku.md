# 小红书直播弹幕协议（RWP）

> 目标消费方：`UniBarrage` 平台适配器 `xiaohongshu`（`POST /xiaohongshu {rid,cookie?}`）。  
> 抓包样本：[`samples/xhs-proto-capture.json`](./samples/xhs-proto-capture.json)  
> 抓包房间：`570409528162863169`（2026-08-15，游客 headless Chromium + CDP）。  
> **本协议可游客收听**，不强制登录 Cookie；登录 Cookie 可提高稳定性，但非硬依赖。

---

## 0. 结论摘要

| 项 | 值 |
|----|----|
| WSS | `wss://apppush-rws.xiaohongshu.com/rwp` |
| 帧格式 | **JSON 文本**（非 protobuf 业务体） |
| 鉴权 | `sid = aLt`，来自 `GET /api/sns/web/v1/celestial/lt`（header `c_device_id`） |
| 业务域 | `bizName: "room"`，`roomType: "LIVE"` |
| 消息路径 | `recv.t=4` → `b.d.b[]` → `item.d` base64 → JSON → `customData` 再 JSON |
| 心跳 | 业务 `liveHeartBeat`（`t=3`）+ 链路 ping（`t=2,s=6`） |
| 游客 | **可听**；`uid` 为游客 id（形如 `6a80…`），`deviceId` 为 UUID |

**不需要**本地复刻 SM4 / `x-s` 来建弹幕长连接（那是 edith 业务 API 签名）。弹幕链路核心是：

1. 拿游客/`web_session` 会话 + `deviceId`
2. 调 `celestial/lt` 拿 `aLt`（= wss `sid`）
3. 连 RWP → auth → register room → join room → 心跳 → 收帧

---

## 1. 前置身份（游客 Cookie 生成）

### 1.1 本地可生成（不用服务端）

| Cookie/字段 | 算法 | 例 |
|-------------|------|----|
| `a1` | 52 字符：`hex(ts_ms) + random36(30) + "50" + "000" + crc32`，截断到 52 | `1a0060bf8c57dx44y…450000472097` |
| `webId` | `md5(a1)` → 32 hex | `f3e8834c2ddfbc2e3826872d8080cfd8` |
| `xsecappid` | 固定 | `xhs-pc-web` |
| `webBuild` | 页面版本字符串（可固定近期值） | `6.41.3` |
| `loadts` | `Date.now()` | |
| `deviceId` | UUID v4（页面存 `sessionStorage.XHS_TAB_DEVICE_ID`） | |
| `fingerprint` | `String(Date.now())`（`XHS_RWP_FINGERPRINT`） | |

`a1` / `webId` 算法已在 `Cloxl/xhshow` 实现（`Xhshow.generate_a1()` / `generate_web_id(a1)`），并与真实页面 cookie 格式一致（已用页面 `webId == md5(a1)` 校验）。

### 1.2 服务端下发：`web_session` + `user_id`

**不是本地生成**。来自游客激活接口：

```
POST https://edith.xiaohongshu.com/api/sns/web/v1/login/activate
Content-Type: application/json;charset=UTF-8
Cookie: a1=…; webId=…; xsecappid=xhs-pc-web; …
必需签名头: X-s / X-t / X-S-Common（及 traceid）

Body:
{"client_public_key_base64":"<32字节随机 base64>"}
```

成功响应（已纯协议实测）：

```json
{
  "code": 0,
  "success": true,
  "data": {
    "user_id": "6a808623000000001400c800",
    "session": "030037adb3d257348d776fc0f82d4a61ef652d",
    "secure_session": "X185d5session.030037adb3d257348d776fc0f82d4a61ef652d",
    "ssk": "…"
  }
}
```

同时 `Set-Cookie: web_session=<session>; Domain=xiaohongshu.com; HttpOnly`。

| 字段 | 用途 |
|------|------|
| `data.session` / cookie `web_session` | 后续 edith API 会话（**`lt` 必需**） |
| `data.user_id` | WSS `authInfo.uid`（游客 uid） |
| `client_public_key_base64` | 32 字节随机 base64 即可（抓包为 X25519 形式；纯随机也通） |

> 之前 `lt` 返 `code=-101 无登录信息`，就是缺 `web_session`（没走 activate）。

### 1.3 deviceId / fingerprint

- `deviceId`：UUID，会话内固定；传 `lt` 的 header `c_device_id` 与 WSS `deviceInfo.deviceId`
- `fingerprint`：`String(Date.now())`，仅 WSS `deviceInfo.fingerprint`

### 1.4 Cookie 清单（最小可用集）

| Cookie | 必需？ | 来源 |
|--------|--------|------|
| `a1` | **是** | 本地生成 |
| `webId` | **是** | `md5(a1)` |
| `web_session` | **是**（调 `lt`） | `login/activate` |
| `xsecappid` | 建议 | 固定 `xhs-pc-web` |
| `webBuild` / `loadts` | 可选 | 本地 |
| `gid` | 可选 | `as.xiaohongshu.com/.../webprofile` 下发（弹幕链路可跳） |
| `websectiga` / `sec_poison_id` | 可选 | 风控 SDK（弹幕链路可跳） |
| `acw_tc` | 自动 | 各主机 Set-Cookie |

弹幕 WSS 本身只吃 `sid(=a_lt)/uid/deviceId`；HTTP 侧最小依赖是 **`a1+webId+web_session` + x-s 签名**。

### 1.5 推荐序列（纯协议，已实测）

```text
1. a1 = generate_a1()                 # 52 chars
2. webId = md5(a1)
3. deviceId = uuid4()
4. cookies = {a1, webId, xsecappid, webBuild?, loadts?}
5. POST /api/sns/web/v1/login/activate
     body: {client_public_key_base64: random32B_b64}
     headers: Cookie + x-s/x-t/x-s-common  (xhshow.sign_headers_post)
   → user_id, session(=web_session)
6. cookies.web_session = session
7. GET  /api/sns/web/v1/celestial/lt
     header c_device_id: deviceId
     headers: Cookie + x-s/x-t/x-s-common  (xhshow.sign_headers_get)
   → data.a_lt, data.r_lt, data.expired_time
8. WSS wss://apppush-rws.xiaohongshu.com/rwp
     auth.sid = a_lt, auth.uid = user_id
     register room → join roomId/LIVE → heartbeat → recv t=4
```

> 实测：上述步骤在无浏览器环境一次跑通，25s 内收到 50+条 `text` 弹幕（房间 `570409528162863169`）。

---

## 2. 取 sid（aLt）

前端 `defineRwpConfig` → `authInfoHooks`：

```text
sid = getLoginToken(uid, deviceId).aLt
```

### 2.1 API

```
GET https://edith.xiaohongshu.com/api/sns/web/v1/celestial/lt
Header:
  c_device_id: {deviceId}
  Cookie: …（页面 cookie，建议带）
  User-Agent: Chrome 桌面 UA
```

### 2.2 成功响应（逻辑字段）

```json
{
  "aLt": "a1:Cl8K…base64…",
  "rLt": "…",
  "expiredTime": 10080
}
```

- `aLt` → WSS 握手 `authInfo.sid`（**原样使用，不要本地重签**）
- `rLt` → 刷新用（本监听链路可先忽略，过期重拉 `lt`）
- `expiredTime`：秒；前端缓存半寿期：`expiredAt = now + expiredTime*1000/2`

### 2.3 aLt / sid 内部结构（只读说明）

`sid = "a1:" + base64(protobuf)`，外层大致：

```
1: {
     1: { 1: uid, 2: "red" },
     2: { 1: "browser", 2: "web", 3: deviceId },
     3: "xhs-pc"
   }
2: 2
3: expiredTime   // 例 10080
4: ts_ms
5: signature     // 服务端签，客户端勿伪造
```

**实现时直接用接口返回的 `aLt` 字符串**，无需自编码 protobuf。

### 2.4 纯协议已打通（重要）

2026-08-15 已用 **零浏览器** 路径实测成功：

```
generate a1/webId  → POST login/activate  → GET celestial/lt  → WSS auth/register/join  → 收到 text 弹幕
```

关键依赖：

- Cookie：本地生成 `a1` + `webId=md5(a1)`，再用 `activate` 换 `web_session` + `user_id`
- 签名：edith 请求带 `x-s` / `x-t` / `x-s-common`（`xhshow` 可直接用）
- WSS 本身**不需** `x-s`

详见下节 §1.5。

---

## 3. WSS 连接

```
wss://apppush-rws.xiaohongshu.com/rwp
```

- 子协议：无
- 消息：UTF-8 JSON 文本帧
- 公共 envelope：

```json
{
  "v": 1,
  "t": 2,
  "m": "<clientMsgId>",
  "b": { }
}
```

| 字段 | 含义 |
|------|------|
| `v` | 版本，恒 `1` |
| `t` | 帧类型：`2`=信令/RPC，`3`=业务上行，`4`=业务下行推送 |
| `m` | 消息 id；请求-响应配对用同一 `m` |
| `b` | body |

客户端 `m` 生成方式（页面）：近似 `{randomHex}-{timestampish}`，唯一即可。

---

## 4. 握手顺序（已抓包验证）

### 4.1 Auth（`t=2`, 内层 `a=1,s=0`）

**SENT：**

```json
{
  "v": 1,
  "t": 2,
  "m": "<mid>",
  "b": {
    "d": {
      "a": 1,
      "s": 0,
      "b": {
        "appId": "xhs-pc",
        "authInfo": {
          "authType": "generic",
          "sid": "<aLt>",
          "uid": "<uid>",
          "domain": "red"
        },
        "deviceInfo": {
          "deviceId": "<uuid>",
          "fingerprint": "<fp>",
          "platform": "browser",
          "os": "web",
          "osVersion": "10.15",
          "deviceName": "Chrome",
          "appVersion": "131.0.0.0",
          "userAgent": "<UA>"
        },
        "serviceTag": "",
        "bizInfos": [{ "bizName": "push", "serializeType": "json" }],
        "roomInfo": [],
        "roomInfos": [],
        "tagInfo": [],
        "extInfo": {},
        "state": 1
      }
    }
  }
}
```

**RECV（成功）：**

```json
{
  "v": 1,
  "t": 2,
  "m": "<same mid>",
  "b": {
    "a": {
      "b": { "socketId": "socket#…#prod#rwp", "time": 1786806860971 },
      "c": 0,
      "m": "success"
    }
  }
}
```

`b.a.c == 0` 且 `m == "success"` 才继续。

### 4.2 Register biz room（`t=2`, `a=1,s=1`）

```json
{
  "v": 1, "t": 2, "m": "<mid>",
  "b": {
    "d": {
      "a": 1,
      "s": 1,
      "b": {
        "bizInfo": { "bizName": "room", "serializeType": "json" },
        "register": true
      }
    }
  }
}
```

→ `success`

### 4.3 Join room（`t=2`, `a=1,s=8`）

```json
{
  "v": 1, "t": 2, "m": "<mid>",
  "b": {
    "d": {
      "a": 1,
      "s": 8,
      "b": {
        "info": {
          "bizName": "room",
          "roomId": "<roomId>",
          "roomType": "LIVE"
        }
      }
    }
  }
}
```

→ `success` 后开始收 `t=4` 推送。

### 4.4 业务心跳 liveHeartBeat（`t=3`）

约每 30s+（页面还会配合 HTTP `viewer_heart`）：

```json
{
  "v": 1,
  "t": 3,
  "m": "<mid>",
  "b": {
    "d": {
      "a": 0,
      "c": "liveHeartBeat",
      "biz": "room",
      "b": "<base64>",
      "e": {},
      "s": "rrmp.o.l"
    }
  }
}
```

`b` 解码后：

```json
{
  "roomId": "<roomId>",
  "roomType": "LIVE",
  "command": 1,
  "customData": "{\"type\":\"viewer_heart\",\"priority\":0,\"profile\":{\"nickname\":\"\",\"avatar\":\"\",\"user_id\":\"<uid>\",\"role\":0},\"source\":\"web_live\",\"desc\":\"\"}"
}
```

### 4.5 链路 ping（`t=2`, `a=1,s=6`）

高频空 body：

```json
{"v":1,"t":2,"m":"<mid>","b":{"d":{"a":1,"s":6,"b":{}}}}
```

实现：每 ~5–10s 发一次；或对服务端同类帧回 pong（若后续抓到对端 ping 再对齐）。

---

## 5. 下行弹幕帧

### 5.1 Envelope（`t=4`）

```json
{
  "v": 1,
  "t": 4,
  "m": "<serverMid>",
  "b": {
    "d": {
      "a": 0,
      "b": [
        {
          "d": "<base64>",
          "e": {},
          "m": "<msgId>"
        }
      ],
      "biz": "room",
      "t": 1786806861143
    }
  }
}
```

兼容路径（扩展/旧代码）：`outer.b.d.b`（本抓包确认）。

### 5.2 item.d 解码

```
base64(item.d) → UTF-8 JSON
```

```json
{
  "command": 1,
  "customData": "<JSON 字符串>",
  "msgId": "1405932642060263425",
  "priority": 2,
  "roomId": "570409528162863169",
  "roomType": "LIVE",
  "ts": 1786806862881,
  "uuid": "1afe3b94-33a4-4b34-9f1f-d4d29650b578"
}
```

再：

```
customData = JSON.parse(decoded.customData)
```

### 5.3 customData.type → UniBarrage

| type（已抓到 / 参考扩展） | 建议 MessageType | 关键字段 |
|---------------------------|------------------|----------|
| `text` / `text_message` | Chat | `desc`, `profile.nickname`, `profile.user_id`, `commentId` |
| `audience_join_v2` | EnterRoom | `profile.*` |
| `fansgroup_join_room_effect` | EnterRoom（可附粉丝团） | `user_info.*`, `text.postfix` |
| `praise` / `like` / `combo_praise` / `light` / `like_comment` / `live_like` / `live_common_msg_action` | Like | `praise_info.count`, `profile.*` |
| `gift_dock_and_effect` | Gift | `send_user_info`, `base_gift_info.{name,coins,icon}`, `receive_user_info` |
| `follow_emcee` | Subscribe | `profile.*` |
| `letter_refresh` | 可忽略 | 系统信 |
| `viewer_heart` | 可忽略 | 心跳回显类 |

### 5.4 真实 text 样例（抓包）

```json
{
  "type": "text",
  "current_time": 1786806862704,
  "source": "search_onebox",
  "translated": false,
  "at_users": [],
  "origin_language": "zh_cn",
  "commentId": "7674277032444974081",
  "profile": {
    "avatar": "https://sns-avatar-qc.xhscdn.com/avatar/…",
    "nickname": "夏果酸",
    "role": 0,
    "user_id": "678cb1f6000000000e01df65",
    "follow_status": 3
  },
  "desc": "终于来女生了",
  "comment_type": 0,
  "aggregate": false,
  "goods_status": false
}
```

### 5.5 真实 gift 样例（抓包）

```json
{
  "type": "gift_dock_and_effect",
  "send_user_info": {
    "id": "649cddab0000000025035f4e",
    "nick_name": "江南雨",
    "avatar": "…"
  },
  "receive_user_info": {
    "id": "64ccb022000000000e0241a5",
    "nick_name": "活不腻来相亲"
  },
  "base_gift_info": {
    "id": 137925940106684781,
    "name": "比心",
    "icon": "…",
    "coins": 1
  }
}
```

注意 gift 用户字段是 `nick_name`（下划线），text/join 是 `profile.nickname`——适配层要兼容。

---

## 6. 辅助 HTTP（非 WSS，进房前后）

页面会打（监听器**可不依赖**，但可用来校验房间在播）：

| API | 用途 |
|-----|------|
| `GET https://live-room.xiaohongshu.com/api/sns/red/live/web/v1/room/current_room_info?room_id={id}&request_user_id={uid}&source=web_live&client_type=1` | 房间状态 |
| `POST/GET …/v1/center/room/join/room` | 进房业务 |
| `GET …/v1/room/join_comment_info?room_id={id}&source=web_live&…` | 评论区元信息 |
| `GET …/v1/web/room_user/viewer_heart` | HTTP 侧心跳 |
| FLV `https://live-source-play.xhscdn.com/live/{roomId}.flv?userId={uid}&…` | 拉流（弹幕链不需要） |

`roomId`：URL path `/livestream/{roomId}` 即 UniBarrage `rid`。

---

## 7. UniBarrage 接入草图

```
StartListen(roomId, cookie?):
  deviceId = uuid()
  fingerprint = str(now_ms)
  session = HTTP client with cookie (optional) + desktop UA
  uid = resolveUid(session, roomId)          # 见 §1.3
  aLt = GET celestial/lt (c_device_id=deviceId, signed if needed)
  ws = dial wss://apppush-rws.xiaohongshu.com/rwp
  send auth(s=0) with sid=aLt, uid, deviceInfo
  wait success
  send register room (s=1)
  wait success
  send join LIVE (s=8, roomId)
  wait success
  loop:
    on t=4: decode b.d.b[] → map customData.type → emit
    ticker: liveHeartBeat + ping s=6
    on auth expire / close: refresh aLt, re-handshake
```

映射时：

- Chat：`content=desc`, `user.name=profile.nickname`, `user.id=profile.user_id`
- Gift：`gift.name=base_gift_info.name`, `gift.price=coins`, `user` from `send_user_info`
- 去重键：`msgId` / `uuid` / `commentId`

---

## 8. 与「签名库」的关系

| 能力 | 是否需要 xhshow/`x-s` | 说明 |
|------|----------------------|------|
| WSS 帧收发 | 否 | 纯 JSON |
| `celestial/lt` | **很可能要** | edith API，抓包后二次裸 fetch 失败 |
| `current_room_info` 等 | 视风控 | 可先试 cookie-only |
| 弹幕内容解密 | 否 | base64 + JSON |

推荐依赖：

- Python 参考签名：`Cloxl/xhshow` → port Go 或 goja 跑官方 JS
- 消息解析参考：`llg1634/xhs-live-recorder-extension`（`b.d.b` 三层）

---

## 9. 未决 / 风险

1. ~~`celestial/lt` 纯协议~~ **已打通**（需 `web_session` + `x-s`）。  
2. ~~游客 uid~~ **已明确**：`login/activate` → `data.user_id`。  
3. `expired_time` 后续刷新是否必须用 `r_lt` 未测（可简单重走 `lt`）。  
4. `websectiga` / 完整 shield SDK 未接入；当前弹幕链路可跳过。  
5. `client_public_key_base64` 是否必须真实 X25519 密钥对未深究（随机 32B 已通）。  
6. 房间未开播时 `join` 行为未系统测。  
7. WSS 是否多机房域名（仅 `apppush-rws.xiaohongshu.com`）。

---

## 10. 探活记录

| 时间 | 方法 | 结果 |
|------|------|------|
| 2026-08-15 | CDP headless → 房间 `570409528162863169` | WSS 建连成功；auth/register/join success；收到 text/praise/join/gift |
| 2026-08-15 | 仅 `a1+webId` + xhshow 签 `lt` | `code=-101` 无登录（缺 `web_session`） |
| 2026-08-15 | CDP 追 cookie | `web_session` 来自 `POST login/activate` Set-Cookie；`webId=md5(a1)` |
| 2026-08-15 | **纯协议** `generate_a1 → activate → lt → WSS` | 全成功；25s 收 54 条 text 等 |
| 2026-08-15 | 旧房间 `570409416236943654` | `roomStatus:-1`，无有效弹幕 WSS 业务 |

样本：

- `docs/samples/xhs-proto-capture.json` — WSS 帧
- `docs/samples/xhs-lt-capture.json` — `lt` 请求头
- `docs/samples/xhs-activate-capture.json` — activate / shield 引导

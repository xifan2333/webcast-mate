# 三平台直播协议对照（登录 / 开播 / 关播 / 推流码）

> 产品与扩展规范见 [SPEC.md](./SPEC.md)。本文只记 **HTTP 级协议**，不规定实现语言。  
> 推流侧继续用静态 `~/.config/livestream/platforms.conf` + `gpu-screen-recorder`。  
> **一场直播内推流码不变**；关播后再开才换新码。开播成功后把 `server`/`key` 写进 conf 即可推。  
> 无浏览器约束见 SPEC §6。

---

## 0. 和推流栈的边界

```
协议层（本文）                 推流层（已有，不动）
─────────────                 ────────────────
登录 → 会话 Cookie/token
开播 → 本场 rtmp server+key  →  写入 platforms.conf
关播 → 结束房间               →  capture-router livestream stop（杀 gsr）
```

`gpu-screen-recorder` 的 `-o` 启动时固定，**不需要热更新 URL**。  
动态只发生在「开播瞬间生成码 → 写入 conf → 再 start gsr」。

---

## 1. B 站 bilibili

权威实现：本机油猴 `~/Code/userscripts/scripts/bilibili-live-helper.user.js`（你已验证的链路）。

### 1.1 登录（纯协议扫码 — 油猴没有，需补）

油猴只读浏览器 Cookie，**没有登录逻辑**。Passport 扫码是公开接口，已探活：

| 步骤 | 方法 | URL |
|------|------|-----|
| 申请码 | GET | `https://passport.bilibili.com/x/passport-login/web/qrcode/generate` |
| 轮询 | GET | `https://passport.bilibili.com/x/passport-login/web/qrcode/poll?qrcode_key={key}` |

**generate 响应（已实测 code=0）：**

```json
{
  "code": 0,
  "data": {
    "url": "https://account.bilibili.com/h5/account-h5/auth/scan-web?…&qrcode_key=…",
    "qrcode_key": "8b05b9017655c20217667939e017d83f"
  }
}
```

- 手机 B 站扫 `data.url`（或把 url 打成二维码图）。
- 必须用**同一个 HTTP 客户端/Cookie 罐** poll，登录成功时 `Set-Cookie` 才写得回来。

**poll `data.code`：**

| code | 含义 |
|------|------|
| 86101 | 未扫码（已实测） |
| 86090 | 已扫码未确认（常见约定） |
| 86038 | 二维码过期 |
| 0 | 成功 → 响应带 Cookie，且 `data.url` 含跳转、`data.refresh_token` |

**登录成功后关键 Cookie：**

| Cookie | 用途 |
|--------|------|
| `SESSDATA` | 会话 |
| `bili_jct` | CSRF（= csrf / csrf_token） |
| `DedeUserID` | uid |
| `DedeUserID__ckMd5` | 校验 |
| `sid` | 可选 |

校验登录：

```
GET https://api.bilibili.com/x/web-interface/nav
Cookie: SESSDATA=…
→ data.isLogin == true
```

未登录时：`code=-101`（已实测）。

> 旧接口 `GET /qrcode/getLoginUrl` + oauthKey 仍可用，优先用 `x/passport-login/web/qrcode/*`。

### 1.2 开播（= 拿推流码）

公共 Header（油猴原样）：

```
accept: application/json, text/plain, */*
content-type: application/x-www-form-urlencoded; charset=UTF-8
origin: https://link.bilibili.com
referer: https://link.bilibili.com/p/center/index
user-agent: Mozilla/5.0 … Chrome/…
Cookie: SESSDATA=…; bili_jct=…
```

| 步骤 | 接口 | Body（form） |
|------|------|----------------|
| 可选改标题 | `POST https://api.live.bilibili.com/room/v1/Room/update` | `room_id, title, platform=pc_link, csrf_token, csrf` |
| 可选改分区 | 同上 update / 或 area 专用 | `room_id, area_id, platform=pc_link, csrf*` |
| **开播** | `POST https://api.live.bilibili.com/room/v1/Room/startLive` | 见下 |

**startLive body：**

```
room_id={房间号}
platform=pc_link
area_v2={二级分区 id}
backup_stream=0
csrf_token={bili_jct}
csrf={bili_jct}
```

**成功 `code=0`：**

```json
{
  "code": 0,
  "data": {
    "rtmp": {
      "addr": "rtmp://live-push.bilivideo.com/live-bvc/",
      "code": "?streamname=live_UID_xxx&key=…&schedule=rtmp&pflag=2"
    }
  }
}
```

→ conf：

```ini
[bilibili]
server = {data.rtmp.addr}    # 注意保留末尾 /
key    = {data.rtmp.code}    # 通常以 ?streamname= 开头
video_bitrate = 3200
audio_bitrate = 128
```

你当前 conf 就是某次 startLive 的产物；**每开一场应更新 key（addr 也可能变）**。

### 1.3 人脸验证（可能挡开播）

| | |
|--|--|
| 触发 | startLive `code=60024`，或 `data.qr` 有值 |
| 动作 | 展示 `data.qr` 给手机 B 站扫 |
| 轮询 | `POST https://api.live.bilibili.com/xlive/app-blink/v1/preLive/IsUserIdentifiedByFaceAuth` |
| body | `room_id, face_auth_code=60024, csrf_token, csrf, visit_id=` |
| 成功 | `data.is_identified == true` → **再调一次 startLive** |

### 1.4 关播

```
POST https://api.live.bilibili.com/room/v1/Room/stopLive
room_id={room_id}&platform=pc_link&csrf_token={bili_jct}&csrf={bili_jct}
```

### 1.5 其它

| 接口 | 用途 |
|------|------|
| `GET room/v1/Room/get_info?room_id=` | 房间信息 / live_status（0未开 1直播 2轮播） |
| `GET room/v1/Area/getList?show_pinyin=1` | 分区列表 → `area_v2` |
| 房间号 | 直播姬/link 中心可见；与短号不同，startLive 要用真实 `room_id` |

### 1.6 状态机

```
扫码登录 → Cookie(SESSDATA,bili_jct)
    → (update 标题/分区)
    → startLive
         ├─ 0 → rtmp.addr + rtmp.code → 写 conf → gsr 推
         └─ 60024 → 扫脸 → 再 startLive
    → stopLive
```

---

## 2. 抖音 webcast_mate（aid=2079）

详见 `~/douyin-live/reference/protocol-params.md` 与本仓库 [SPEC.md](./SPEC.md)；此处只列登录+开播要点。

### 2.1 登录（已实现 `login.py`，纯协议）

| 步骤 | 接口 |
|------|------|
| ttwid | `POST https://streamingtool.douyin.com/ttwid/check/` → 不够再 `/ttwid/register/` |
| 出码 | `GET https://streamingtool.douyin.com/passport/web/get_qrcode/` |
| 轮询 | `POST …/passport/web/check_qrconnect/` |

出码 query 必须带 jssdk，否则 **4031 版本过低**：

```
passport_jssdk_version=2.4.13
passport_jssdk_type=normal
is_from_ttaccountsdk=1
aid=2079
language=zh
is_from_iesaccountsaas=1
account_sdk_source=web
next=https://streamingtool.douyin.com
need_logo=false&need_short_url=false&is_new_login=1
```

Header：`x-tt-passport-csrf-token`（首次 get_qrcode 会 Set-Cookie `passport_csrf_token`）。

**check 状态：** `new/1 → scanned/2 → confirmed/3`；`refused/4`、`expired/5` 重新出码。  
可重试错误：`2156` 系统繁忙、`7` 访问太频繁（确认后放慢轮询）。

成功 Cookie（桌面会话，网页 session 开播会 4003166）：

`sessionid, sessionid_ss, sid_tt, sid_guard, sid_ucp_v1, ssid_ucp_v1, uid_tt, odin_tt, passport_assist_user, passport_csrf_token, ttwid, …`

### 2.2 开播

| 步骤 | 接口 | 说明 |
|------|------|------|
| ① | `POST webcast-pc.amemv.com/webcast/room/create_info/` | → `preschedule_key` + `cover.uri` |
| ② | bdms 1.0.1.20 签 **完整 create body** | → `a_bogus`（Chromium+CDP，~184–188 字符） |
| ③ | `POST webcast-pc…/webcast/room/create/?…&a_bogus=` | body 23 参数 |
| ④ | `POST webcast.amemv.com/webcast/room/ping/anchor/` | **`status=2` LIVING** ← 缺这步观众看不到 |

`ROOM_STATUS = {PREPARE:1, LIVING:2, PAUSE:3, FINISH:4}`

create 成功 → `data.stream_url.rtmp_push_url`（完整 URL，一场内不变）。  
可拆 conf，或 gsr `-o` 直接吃完整 URL：

```ini
[douyin]
server = rtmp://push-rtmp-….douyincdn.com/third
key    = stream-xxx?…&sign=…&pri=…
```

（`pri=unix-31104000` 伴侣会加，非必须。）

### 2.3 关播

```
POST webcast.amemv.com/webcast/room/ping/anchor/
room_id=&stream_id=&status=4
```

### 2.4 Host 分工

| Host | 用途 |
|------|------|
| `streamingtool.douyin.com` | 登录 passport / ttwid |
| `webcast-pc.amemv.com` | create_info / create |
| `webcast.amemv.com` | ping/anchor、check_exist、get_pc_obs_status、user/me（query 带 os_username 等） |

### 2.5 状态机

```
ttwid → get_qrcode → check_qrconnect(confirmed) → 桌面 Cookie
  → create_info → a_bogus → room/create → ping LIVING=2
  → 推流
  → ping FINISH=4
```

---

## 3. 小红书 xhs

> 弹幕/长连接协议（RWP）见独立文档 [protocol-xhs-danmaku.md](./protocol-xhs-danmaku.md)（与开播/推流码无关）。

两条线：**电脑助手 robs**（协议完整）和 **网页 6 位码换密钥**（半手动）。

### 3.A 电脑助手协议（推荐实现）

Host：`https://robs.xiaohongshu.com`  
UA（第三方助手）：

```
Mozilla/5.0 … live-helper/2.6.6 Chrome/89.0.4389.128 Electron/12.0.11 …
env/production platform/win32 appname/xhs-live
```

公共 Header：`device-id: {mac-like}`；登录后加 `sid: {access_token}`。

#### 登录（短信）

| 步骤 | 接口 | Body |
|------|------|------|
| 发码 | `POST /api/sns/send_sms` | form: `phone_number, phone_country=86` |
| 登录 | `POST /api/sns/login_by_sms` | form: `phone_number, phone_country=86, sms_code` |

成功：`result==0`，`data.access_token` → 之后所有请求 Header `sid`。  
校验：`GET /api/sns/check_login`（`result!=0` 则会话失效）。

#### 开播

Query 常挂在 PC 信息上（可简化，但助手带了）：

```
build=2200002&platform=pc&system_version=10.0.22000&cpu_model=…&gpu=…&is_win_7=false
```

| 步骤 | 接口 | 说明 |
|------|------|------|
| 准备 | `GET /api/sns/live/pre?{PC_QS}` | Header `sid` |
| 开播 | `POST /api/sns/live/{room_id}/start?{PC_QS}` | JSON body |

**pre 成功 `data`：**

- `room_id`
- `name`（标题）
- `cover`
- `url.push_url` ← **完整 rtmp 推流地址**

**start body：**

```json
{
  "name": "直播间名",
  "notice": "",
  "is_distribute": true,
  "cover": "https://picasso-static.xiaohongshu.com/…",
  "lesson_id": 0
}
```

`result==0` 才算开播。

#### 关播

```
POST /api/sns/live/{room_id}/stop
Header: sid
Body: {}
```

#### 推流 conf

官方固定 server（网页文档）：

```
rtmp://live-push.xhscdn.com/live
```

`push_url` 可能是完整 `rtmp://live-push.xhscdn.com/live/{key}?…`：

```ini
[xiaohongshu]
server = rtmp://live-push.xhscdn.com/live
key    = {从 push_url 拆出的 path 最后一段 + query}
video_bitrate = 4000
audio_bitrate = 128
```

或 gsr 直接 `-o "{完整 push_url}"`。

#### 状态

`GET /api/sns/live/check_live`（Header `sid`）

#### 封面（可选）

1. `GET https://www.xiaohongshu.com/fe_common_api/burdock/upload/v4/qcloud/token?bucket=picasso&bizKey=livehelper&…`
2. 表单上传 COS（key / Signature / x-cos-security-token / file）
3. cover = `https://picasso-static.xiaohongshu.com/{key}`

### 3.B 网页半自动（不依赖 robs 登录）

1. 手机小红书预直播页 →「电脑」tab → 6 位码（约 12h 有效）
2. 浏览器打开 `https://www.xiaohongshu.com/zhibo/robs` 登录
3. 页面 API（spectrum 前端，host 同站 / edith）：

| 常量 | Path |
|------|------|
| APPLY_OBS_AUTH | `/web_api/sns/v1/live/obs/apply_obs_auth` |
| USER_OBS_KEY | `/web_api/sns/v1/live/obs/push_url` |
| USER_OBS_AUTH | `/web_api/sns/v1/live/obs/host_auth_info` |
| GET_LIVING_PUSH_URL | `/web_api/sns/v1/live/obs/living_push_url` |

4. server 固定 `rtmp://live-push.xhscdn.com/live`，密钥页上展示  
5. 手机点「进入直播」；**每次关播再开都要新 6 位码**

实现优先级：**3.A robs 短信** 一条链做完；3.B 作备用。

### 3.C 状态机（3.A）

```
send_sms → login_by_sms → sid
  → live/pre → push_url + room_id
  → live/{id}/start
  → 推流
  → live/{id}/stop
```

---

## 4. 总表

| | B 站 | 抖音 | 小红书 (robs) |
|--|------|------|----------------|
| **登录** | Passport 扫码 → SESSDATA+bili_jct | streamingtool 扫码 → 桌面 session | 短信 → sid |
| **登录 Host** | passport.bilibili.com | streamingtool.douyin.com | robs.xiaohongshu.com |
| **开播** | startLive 一次返回 rtmp | create_info→a_bogus→create→**ping LIVING** | pre→start |
| **关播** | stopLive | ping FINISH=4 | stop |
| **推流码形态** | addr + code 两段 | 完整 rtmp_push_url | 完整 push_url |
| **码是否一场一变** | 是 | 是 | 是 |
| **额外门槛** | 人脸 60024 | 桌面会话；a_bogus | 无（短信） |
| **本仓库状态** | 协议清（油猴+扫码探活） | 登录+开播+关播已通 | 协议清（助手源码） |
| **写入 conf** | server=addr, key=code | 拆 url 或整段 | server 固定 + key |

---

## 5. 实现时注意（给以后 Rust CLI 用）

1. **Cookie 罐**：扫码/轮询/开播同一 client，否则 Set-Cookie 丢失。  
2. **CSRF**：B 站 `bili_jct` 同时作 `csrf` 与 `csrf_token`。  
3. **抖音 create 不够**：必须 `ping/anchor status=2`。  
4. **抖音 a_bogus**：绑 body + Win/Electron 环境；目前依赖 Chromium+bdms。  
5. **conf 更新时机**：协议开播成功后再写 `platforms.conf`，再 `capture-router livestream start`。  
6. **不要**为动态 URL 改 gsr；一场内地址静态。  
7. **密钥文件权限**：session/cookie 文件 `0600`。  
8. **错误码**：B 站 `-101` 未登录、`60024` 人脸；抖音 `4003166` 网页会话/签名、`4031` 缺 jssdk、`7`/`2156` 确认期频控；小红书 `result!=0` 看 `msg`。

---

## 6. 探活记录（2026-08-15）

| 探测 | 结果 |
|------|------|
| B 站 `qrcode/generate` | code=0，返回 url + qrcode_key |
| B 站 `qrcode/poll` 未扫 | data.code=86101 未扫码 |
| B 站 `nav` 无 Cookie | code=-101 账号未登录 |
| B 站 startLive 参数 | 与油猴一致（pc_link / area_v2 / csrf） |
| 抖音登录+create+ping+ffmpeg | 已实机通过 |
| 小红书 robs 路径 | 来自 XiaoHongShu_OBS + spectrum main.js |
| 小红书 web server | `rtmp://live-push.xhscdn.com/live`（官方 OBS 文档） |

---

## 7. 参考路径

| 材料 | 路径 |
|------|------|
| B 站油猴 | `~/Code/userscripts/scripts/bilibili-live-helper.user.js` |
| 抖音开播 | `~/douyin-live`：`pure_create.py` / `login.py`；参数表 `reference/protocol-params.md` |
| 小红书助手（第三方） | github.com/ShigemoriHakura/XiaoHongShu_OBS |
| 小红书 web OBS 页 | https://www.xiaohongshu.com/zhibo/robs |
| 推流 conf | `~/.config/livestream/platforms.conf` |
| gsr 推流 | `capture-router livestream start\|stop` |

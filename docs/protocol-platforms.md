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

**完整真链路（伴侣 12.7.3 全量抓包 + 静态 JS）见：[protocol-douyin-companion.md](./protocol-douyin-companion.md)。**

摘要：

| 段 | Host | app_name | 要点 |
|----|------|----------|------|
| 登录 | `streamingtool.douyin.com` | **`aweme_live_assistant`** | get/check 大 query + **每请求 a_bogus**；`did`/`iid`；check `is_frontier=true`；**confirmed 的 Set-Cookie = 桌面 session** |
| 开播/关播 | **`webcast.amemv.com`** | `webcast_mate` | `create_info` → `pre_schedule_key` → `room/create` → **`ping status=2`** → 关播 **`ping status=4`** |

```
ttwid → get_qrcode → check_qrconnect(new/scanned/confirmed)
  → create_info → room/create (PREPARE=1, rtmp)
  → ping LIVING=2 → 推流
  → ping FINISH=4 →（可选）anchor_finish_info
```

`ROOM_STATUS = {PREPARE:1, LIVING:2, PAUSE:3, FINISH:4}`。  
**不要**再用「小 jssdk 包、无 a_bogus、create 走 webcast-pc」的旧笔记。

---

## 3. 小红书 xhs

> 弹幕/长连接协议（RWP）见 [protocol-xhs-danmaku.md](./protocol-xhs-danmaku.md)（与开播/推流码无关）。  
> **直播助手 4.4.0 开停播/登录/状态**见 [protocol-xhs-live-helper.md](./protocol-xhs-live-helper.md)（CAS + redobs center/room；静态+抓包）。


### 3.1 网页 OBS（6 位连接码 → 推流地址）

人对人主路径（官方教程一致）：

1. 手机小红书预直播页 →「电脑」tab → **6 位连接码**（关播再开通常要新码）
2. 电脑侧已登录网页会话（见下）后：
3. `GET https://www.xiaohongshu.com/web_api/sns/v1/live/obs/push_url?code={6位码}`  
   - Cookie：登录后的 `a1` / `webId` / `web_session` 等  
   - `xsecappid=spectrum`，请求需 `x-s` / `x-t` 签名  
4. 解析完整 `push_url`（或等价字段）→ 拆 `server` + `key` 写入 `live.json`
5. OBS / gsr 推流；手机侧如需再点「进入直播」
6. **关播**：以手机结束直播为主；CLI `stop` 清本地 `live.json`（远端协议 stop 不走旧 robs）

相关只读 API（spectrum / 同站，探活用）：

| 用途 | Path |
|------|------|
| 六位码换密钥 | `/web_api/sns/v1/live/obs/push_url` |
| 直播中取码 | `/web_api/sns/v1/live/obs/living_push_url` |
| 主机 OBS 信息 | `/web_api/sns/v1/live/obs/host_auth_info` |
| OBS 授权 | `/web_api/sns/v1/live/obs/apply_obs_auth` |

固定 server（官方 OBS 文档）：

```
rtmp://live-push.xhscdn.com/live
```

`push_url` 常为完整 `rtmp://live-push.xhscdn.com/live/{key}?…`，拆法：

```
server = rtmp://live-push.xhscdn.com/live
key    = path 最后一段 + query
```

或 gsr 直接 `-o "{完整 push_url}"`。

页面入口（人用）：`https://www.xiaohongshu.com/zhibo/obs`（历史上也有 `zhibo/robs` 落地页名，**不等于**实现 robs 短信 API）。

#### webcastmate 目标行为

```
登录态（待定：须与 push_url 同源 cookie；edith 笔记扫码 ≠ 自动等于 OBS 登录）
  → 用户输入 6 位码（或 WEBCAST_MATE_XHS_CODE）
  → GET .../live/obs/push_url?code=
  → live.json + stdout {server,key,room_id,cookie}
```

非交互示例：

```bash
WEBCAST_MATE_XHS_CODE=254966 webcastmate start xiaohongshu -y
```

#### 状态机（OBS）

```
[可选登录] → 输入 6 位码 → push_url → 写 live.json → 推流
  → 手机关播 / CLI stop 清本地
```

#### 登录说明（开放问题）

- **edith** `login/qrcode/*`（`xhs-pc-web`）：笔记/Web 常见扫码；实测可 scanned，确认后会话升级不稳定，**不默认当作 2026 开播登录定论**。
- **redlive / customer CAS**（`omikuji` / `XYW_`）：另一套后台登录，**当前不采用**。

---

## 4. 总表

| | B 站 | 抖音 | 小红书 (OBS 6 位码) |
|--|------|------|---------------------|
| **登录** | Passport 扫码 → SESSDATA+bili_jct | streamingtool 扫码 → 桌面 session | 网页会话（与 spectrum/`push_url` 同源；**不做 robs 短信**） |
| **登录 Host** | passport.bilibili.com | streamingtool.douyin.com | www / edith（OBS）；**非** robs |
| **开播** | startLive 一次返回 rtmp | create_info→a_bogus→create→**ping LIVING** | 手机 6 位码 → `obs/push_url` |
| **关播** | stopLive | ping FINISH=4 | 手机关播 + 清 live.json |
| **推流码形态** | addr + code 两段 | 完整 rtmp_push_url | 完整 push_url / 固定 server+key |
| **码是否一场一变** | 是 | 是 | 是（常换 6 位码） |
| **额外门槛** | 人脸 60024 | 桌面会话；a_bogus | 手机预直播出码 |
| **本仓库状态** | 协议清（油猴+扫码探活） | webcastmate：登录+create+ping（a_bogus 经 chromium/bdms） | OBS 取码路径；登录来源待实机；伴侣逆向后补 status |
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
8. **错误码**：B 站 `-101` 未登录、`60024` 人脸；抖音 `4003166` 网页会话/签名、`4031` 缺 jssdk、`7`/`2156` 确认期频控；小红书 OBS `code!=0` / `-104` 无权限看 `msg`。

---

## 6. 探活记录（2026-08-15）

| 探测 | 结果 |
|------|------|
| B 站 `qrcode/generate` | code=0，返回 url + qrcode_key |
| B 站 `qrcode/poll` 未扫 | data.code=86101 未扫码 |
| B 站 `nav` 无 Cookie | code=-101 账号未登录 |
| B 站 startLive 参数 | 与油猴一致（pc_link / area_v2 / csrf） |
| 抖音登录+create+ping+ffmpeg | 已实机通过 |
| 小红书 OBS `push_url` | 用户 curl / 官方 6 位码流程；`xsecappid=spectrum` |
| 小红书 web server | `rtmp://live-push.xhscdn.com/live`（官方 OBS 文档） |

---

## 7. 参考路径

| 材料 | 路径 |
|------|------|
| B 站油猴 | `~/Code/userscripts/scripts/bilibili-live-helper.user.js` |
| 抖音开播 | [protocol-douyin-companion.md](./protocol-douyin-companion.md) |
| 小红书 web OBS 页 | https://www.xiaohongshu.com/zhibo/obs |
| 小红书旧助手复刻（参考 only，不实现） | github.com/ShigemoriHakura/XiaoHongShu_OBS |
| 推流 conf | `~/.config/livestream/platforms.conf` |
| gsr 推流 | `capture-router livestream start\|stop` |

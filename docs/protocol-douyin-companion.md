# 抖音直播伴侣协议（12.7.3）

> 状态：已用 **Wine 伴侣全量 mitm + 静态 JS** 对齐（2026-08-16 会话：登录 → 开播 → 关播）。  
> 产品名：StreamingTool / `webcast_mate` · `aid=2079` · 版本钉 `12.7.3`。  
> 静态入口：`resources/app/app.content/resource/js/`  
> 抓包样例（本机）：`/tmp/douyin_login_flows.jsonl`（全量、无 path 过滤）。

本文描述 **官方伴侣真实链路**，供 `webcast-mate` Douyin adapter 实现对照。  
**不要**再沿用早期「小 query、无 a_bogus、host 写成 webcast-pc」的猜测稿。

---

## 0. 总览

```text
┌────────────── 登录（passport）──────────────┐
│ Host: https://streamingtool.douyin.com       │
│ app_name = aweme_live_assistant              │
│ 设备字段: did + iid                          │
│ 几乎每个请求 query 带 a_bogus（bdms 1.0.1.20）│
│ 源码: 38514（UI 状态机）+ 29382（账号参数）   │
└───────────────────┬──────────────────────────┘
                    │ confirmed → Set-Cookie(session*)
                    ▼
┌────────────── 开播 / 关播（webcast）─────────┐
│ Host: https://webcast.amemv.com              │
│ app_name = webcast_mate                      │
│ 设备字段: device_id + iid                    │
│ query 同样带 a_bogus；写接口带 x-secsdk-csrf │
│ 源码: 93230（进房资料）+ 96458（create/ping） │
└──────────────────────────────────────────────┘
```

### 0.1 房间状态常量

源码（`93230`）：

```text
ROOM_STATUS = { PREPARE: 1, LIVING: 2, PAUSE: 3, FINISH: 4 }
APP_ID.AWEME = 2079
```

### 0.2 端到端状态机

```text
ttwid/check
   → get_qrcode
   → check_qrconnect 循环 (new → scanned → confirmed)
         confirmed 响应 Set-Cookie → 桌面 session
   → [主界面] get_latest_room ∥ create_info → pre_schedule_key + cover
   → room/create → room_id + stream_id + rtmp_push_url  (status 仍为 PREPARE=1)
   → 本地开始推 RTMP
   → ping/anchor status=2 (LIVING) 并周期保活
   → 关播: ping/anchor status=4 (FINISH)
   →（UI）anchor_finish_info / replay 等收尾
```

旁路（权限、礼物、连麦、banner、IM…）数量很大，**推流闭环不需要**。

---

## 1. 登录

### 1.1 源码模块

| 文件 | 职责 |
|------|------|
| `38514.8645c02a.js` | 扫码 UI：出码 `z()`、轮询 `W/Q`、status 分支 |
| `29382.a632aa2b.js` | `APP_ID.AWEME → app_name=aweme_live_assistant`；did/iid/fp |
| passport jssdk + bdms | `commonRequest` 合并大 query、签 **a_bogus**（不在 38514 字面量里） |

### 1.2 `POST /ttwid/check/`

```http
POST https://streamingtool.douyin.com/ttwid/check/
Content-Type: application/json
```

Body（完整形）示例：

```json
{
  "aid": 2079,
  "service": "streamingtool.douyin.com",
  "host": "https://streamingtool.douyin.com",
  "unionHost": "",
  "union": false,
  "needFid": false,
  "fid": "",
  "migrate_priority": 0
}
```

另有简化调用：`{"aid":2079,"service":""}`。  
成功：`status_code=0`，刷新 `ttwid` Cookie。不够再 `POST /ttwid/register/`（历史路径）。

### 1.3 `GET /passport/web/get_qrcode/`

源码（`getQrcodeRequest`，web scope）：

```text
GET get_qrcode
params 显式: { next, need_logo:false, need_short_url:false, is_new_login:"1" }
其余由 commonRequest / account SDK 合并
出码前: generateVerifyPortraitId()
```

#### Query（抓包完整键，实现应对齐语义）

| 键 | 样例 / 说明 |
|----|-------------|
| `passport_jssdk_version` | `2.4.13` |
| `passport_jssdk_type` | `normal` |
| `is_from_ttaccountsdk` | `1` |
| `is_from_iesaccountsaas` | `1` |
| `account_sdk_source` | `web` |
| `aid` | `2079` |
| **`app_name`** | **`aweme_live_assistant`**（不是 `webcast_mate`） |
| **`did`** | 设备 ID（字段名是 `did`） |
| **`iid`** | install id |
| `version_code` | `12.7.3` |
| `device_platform` | **`Windows`**（大写 W） |
| `device_type` | 机型串，如 `4291MU5` |
| `os_version` | `10.0.19045` |
| `channel` | `online` |
| `language` | `zh` |
| `next` / `host` / `domain` | `https://streamingtool.douyin.com` |
| `is_new_login` | `1` |
| `need_logo` / `need_short_url` | `false` |
| `p_bd` | `1.0.1.20`（bdms） |
| `p_js_v` / `p_js_t` | `2.4.13` / `pro` |
| `p_ver` / `p_zt` | `1.0.29` / `3.3.5` |
| `account_sdk_source_info` | 长 hex 风控画像 |
| `biz_trace_id` | 短 trace |
| `captchaHost` | `https://verify.snssdk.com` |
| `loginType[]` | `LOGIN_MOBILE_CODE` |
| `globalMobileSupport` / `unionLoginPanel` / `isBoe` | `true` / `true` / `false` |
| `request_host` | `file%3A%2F%2F`（Electron） |
| **`a_bogus`** | **每个请求新签** |

UA（与开播相同壳）：

```text
Mozilla/5.0 … webcast_mate/12.7.3 Chrome/136… Electron/36… TTElectron/36… Safari/537.36
```

#### 响应（业务）

```json
{
  "message": "success",
  "data": {
    "error_code": 0,
    "token": "…_lf",
    "qrcode": "<png base64>",
    "qrcode_index_url": "https://aweme.snssdk.com/…",
    "is_frontier": false,
    "expire_time": 1786869386
  }
}
```

- 响应里 `is_frontier` 常为 `false`；**轮询请求仍传 `is_frontier=true`**。  
- Set-Cookie：`passport_csrf_token`、`odin_tt` 等。

### 1.4 `POST /passport/web/check_qrconnect/`

源码（`checkQrconnectRequest` + 轮询 `Q`）：

```text
POST check_qrconnect
data: {
  need_logo: false,
  need_short_url: false,
  is_frontier: true,          // 轮询路径写死 true
  token: <get_qrcode.token>,
  is_new_login: "1",
  next: <UI next>             // 抓包 body 为 https://www.douyin.com
}
query: 与 get_qrcode 同套 SDK 大包 + 每次新 a_bogus
```

#### 轮询节奏（`38514`）

```text
W() 递归: await check; if 继续 then setTimeout(W, 1000 * G.current)

G.current:
  初期较小 → 抓包约 1.2s/次
  poll 次数 R ∈ [60,180) → 可升到 3s
  R ≥ 180 或开启 frontier WS → G=5（5s）
```

**不是固定 5 秒。** 前段密、后段才变稀。

#### status 状态机（源码 switch）

| status | 行为 | 是否继续轮询 |
|--------|------|----------------|
| `new` / `1` | 等待扫码 UI | 是 |
| `scanned` / `2` | 显示扫码头像/设备名 | 是 |
| `confirmed` / `3` | 成功回调；**停止轮询** | **否** |
| `refused` / `expired` / `4` / `5` | 若换码次数 &lt; 5：用响应内新 qr/token 换图并继续；否则失败 | 条件 |
| 网络/`error_code==6` | 最多再试 3 次 | 条件 |

#### confirmed 时会话如何落地

源码在 `confirmed` **不再调第二个 login 换票接口**。  
**桌面 session 来自该次 check 响应的 `Set-Cookie`**，包括：

```text
sessionid, sessionid_ss, sid_tt, sid_guard,
uid_tt, uid_tt_ss,
sid_ucp_v1, ssid_ucp_v1,
passport_assist_user, n_mh, session_tlb_tag, …
```

`data.redirect_url` 多为 `https://streamingtool.douyin.com`（落地）；  
随后 `GET webcast.amemv.com/webcast/user/me/` 校验登录。

Cookie 头注意：Electron 有时用 **`, `** 分隔多个 cookie（不是标准 `; `），解析时要兼容。

### 1.5 风控支路（本次成功路径未走）

| 现象 | 含义 | 伴侣行为 |
|------|------|----------|
| `error_code=7` | 访问太频繁 | 官方文档：等待，勿狂刷码；源码 catch 不全按 7 特判 |
| `2046` / `2032` | 手机号 / 推送验证 | UI：`verify_ticket` + sms / push（`38514` 验证页） |
| 开播头 `x-tt-verify-passport-decision` | 开播二次验证 | `createRoomResultHandle` 直接失败分支 |

---

## 2. 开播

### 2.1 源码模块

| 文件 | 职责 |
|------|------|
| `93230.93153c30.js` | `fetchLatestRoomInfo`：并行 latest_room + create_info |
| `96458.381eb576.js` | 组 create body、`createRoom`、`createRoomResultHandle`、`updateRoomStatus`(ping)、`overLiving` |

**Host 一律 `https://webcast.amemv.com`**（本次抓包；不是 `webcast-pc.amemv.com`）。

### 2.2 公共 query（开播段）

```text
ac=wifi
app_name=webcast_mate
version_code=12.7.3
device_platform=windows          # 小写
device_id=<did>
iid=<iid>
aid=2079
live_id=1
channel=online
language=zh
os_version=10.0.19045
resolution=1366*768
webcast_sdk_version=1520
extra_first_tag_id / extra_second_tag_id / extra_third_tag_id
extra_encoder_core / extra_codec_name / extra_codec_is_ex / extra_use_265
a_bogus=<每请求新签>
```

写接口额外 Header：`x-secsdk-csrf-token`（登录后出现 `csrf_session_id` cookie）。

### 2.3 进房资料：`fetchLatestRoomInfo`

```text
并行:
  POST /webcast/room/get_latest_room/     body {}
  POST /webcast/room/create_info/         body 可选预览参数
```

合并：

```text
preScheduleKey ← create_info.data.preview_stream.preschedule_key
cover / title  ← create_info
其它房间残留   ← get_latest_room.data
```

开播前 create_info 还可能带：

```text
enable_preview_stream=1
speed_test_info=[]
live_room_mode=1
orientation=1
```

测速（可选）：

```text
POST /webcast/room/push_stream/speed_test/
```

### 2.4 `POST /webcast/room/create/`

源码组 body `z` 后 `application/x-www-form-urlencoded` 提交。

#### Body 关键字段（源码 + 抓包）

| 字段 | 规则 / 样例 |
|------|-------------|
| `multi_resolution` | `true` |
| `title` | 直播标题 |
| `orientation` | `1` 等 |
| `base_category` / `category` | 分区 |
| `has_commerce_goods` | `false` |
| `disable_location_permission` | 0/1 |
| **`push_stream_type`** | **`obs` 模式 → 2，否则 → 1**（第三方/游戏类） |
| **`third_party`** | **固定 `1`** |
| **`pre_schedule_key`** | 来自 create_info 预览 |
| `auto_cover` | 自动封面 1，否则 2；非自动则带 `cover_uri` + thumb 尺寸 |
| `payload` | JSON 字符串，能力开关数组 |
| `visibility_range` | 可见范围 |
| `enable_health_score_check` | bool |
| `gift_auth` | 由 `/webcast/user/permission/` 结果映射 |
| `gen_replay` / `record_screen` | 回放相关权限 |
| `account` | `douyin` |
| `max_bit_rate` | 如 `4000000` |
| `audience_display_type` | 可选 |

#### 响应关键

```text
status_code = 0
data.id / id_str     → room_id
data.stream_id       → stream_id
data.status          → 1 (PREPARE)   ← 此时还不是 LIVING
data.stream_url.rtmp_push_url
data.stream_url.rtmp_push_url_params   # 编码建议 JSON
```

#### `createRoomResultHandle`（源码分支摘要）

| `status_code` | 行为 |
|---------------|------|
| `0` | 写入 roomStore；`processCreateRoomSuccessHook`；准备推流 |
| 响应头 `x-tt-verify-passport-decision` | 二次验证，开播失败 |
| `20054` | 入驻协议 / 实名 |
| `10018` / `20006` / `4003035` | 封禁弹窗 |
| `4003028` | 人脸检测 |
| 其它 | toast / 申诉 |

钩子链（默认可透传）：`beforeCreateRoom` → `processRoomParams` → create → success/fail → `processStreamParams` → start stream hooks。

### 2.5 `POST /webcast/room/ping/anchor/` = 更新房间状态

源码 `updateRoomStatus`：

```text
POST /webcast/room/ping/anchor/
body: stream_id & room_id & status [& reason_no]
```

| status | 含义 | 何时 |
|--------|------|------|
| `2` | LIVING | create 成功并开始推流后；周期保活 |
| `4` | FINISH | 关播 |

成功后用响应头 `webcast-ntp-t2/t3` 算 NTP 偏移写 mediasdk（推流侧）。

**要点：`create` 成功 ≠ 直播中。** 观众可见需要 **`ping status=2`**。

### 2.6 推流

- URL：`data.stream_url.rtmp_push_url`（可拆 server/key，或整 URL 给编码器）。  
- 伴侣本地 mediasdk 推流；`webcast-mate` 只产出 server/key 写入 `live.json`，编码仍走 gsr 等。  
- 首帧连通有打点（`firstPushStreamEvent`，约 30s 超时语义）。

---

## 3. 关播

源码 `overLiving`：

```text
beforeOverLivingHook
  → updateRoomStatus({ status: FINISH(4), reason_no?: 113 })
  → status_code ∈ {0, 30001, 30003} 视为可结束
  → 清空 currentRoomData、停延迟设置、派发 overLiving、停 SDK
```

关播前 `checkBeforeOverLiving` 可能拦截：未结算预言、回溯任务中等。

抓包顺序：

```text
ping/anchor  status=4
  → POST /webcast/room/anchor_finish_info/   body: room_id=…
  → replay / data_center 等 UI 数据（非推流必需）
```

**协议关播主开关是 ping FINISH；** `anchor_finish_info` 是结束后信息页。

---

## 4. 与错误实现的差异（实现时禁止再犯）

| 项 | 伴侣真实 | 错误猜测 |
|----|----------|----------|
| 登录 `app_name` | `aweme_live_assistant` | `webcast_mate` |
| 登录设备字段 | **`did`** + `iid` | 仅 `device_id` |
| 登录 query | **大包 + 每请求 a_bogus** | 小 jssdk 包、无 a_bogus |
| check `is_frontier` | body **true** | false |
| check body `next` | 抓包为 `https://www.douyin.com` | 仅 streamingtool |
| 轮询间隔 | **自适应 ~1s 起** | 固定 5s 或 1s 无 G 逻辑 |
| confirmed 后 | **Set-Cookie 即会话** | 再造换票步骤 |
| 开播 host | **`webcast.amemv.com`** | `webcast-pc.amemv.com` |
| create 后 | **必须 ping 2** | 只 create |
| 关播 | **ping 4**（+ finish_info） | 只调一个 stop |

「同时段 exe 能登、自研 7」优先解释为：**passport 请求画像不像伴侣**（缺 a_bogus / SDK 字段 / app_name / did），而不是单纯「等 30 分钟」。官方 7 的等待说明仍适用在已触发限流之后。

---

## 5. 最小实现清单（webcast-mate）

### 登录

1. `ttwid/check`（必要时 register）  
2. `get_qrcode`：对齐 §1.3 query（含 a_bogus）  
3. `check_qrconnect` 循环：§1.4 body + 同套 query；处理 new/scanned/confirmed/refused/expired  
4. 从 confirmed 响应收集 Set-Cookie → `secrets/douyin.json` 的 `cookie` 字段（统一 secrets schema）

### 开播

1. （建议）`create_info` → `pre_schedule_key` + cover  
2. `room/create` → `room_id` / `stream_id` / `rtmp_push_url`  
3. 写 `live.json` server/key  
4. `ping/anchor` `status=2`，并按需保活  

### 关播

1. `ping/anchor` `status=4`  
2. 可选 `anchor_finish_info`  
3. 清 `live.json` 本平台段  

### stdout

与 SPEC 一致：`platform, room_id, cookie, server, key`；  
`cookie` = secrets 中桌面会话串（弹幕链可复用）。

### a_bogus

登录与开播 **都需要**。伴侣 `p_bd=1.0.1.20`。  
实现可暂时：Chromium/bdms、或外部 `WEBCAST_MATE_DY_ABOGUS_CMD`；最终目标无浏览器。  
**开播 create 已有 a_bogus 路径时，登录 get/check 必须同一能力覆盖。**

---

## 6. 抓包复现

```bash
# 全量 mitm（无 path 过滤）
cd ~/douyin-live   # 或任意含 reference 的工作副本
uv run mitmdump -p 8080 --set flow_detail=0 \
  -s reference/mitm_login_capture.py
# → /tmp/douyin_login_flows.jsonl
# → /tmp/douyin_login_capture.log

# Wine 伴侣（已设 Proxy 127.0.0.1:8080 时）
WINEPREFIX=~/.wine-douyin wine "…/直播伴侣.exe" --ignore-certificate-errors
```

addon：`reference/mitm_login_capture.py`（全量 HTTP(S)；登录关键路径打 ★）。

---

## 7. 文档索引

| 文档 | 内容 |
|------|------|
| 本文 | 伴侣登录/开播/关播真链路 |
| [protocol-platforms.md](./protocol-platforms.md) | 多平台索引（抖音节指向本文） |
| [SPEC.md](./SPEC.md) | CLI / secrets / live.json 产品契约 |
| [protocol-xhs-live-helper.md](./protocol-xhs-live-helper.md) | 小红书 live-helper 对照结构 |

---

## 8. 变更规则

- 改 Douyin URL/字段/状态机 → **先改本文**，再改 `internal/adapter/douyin/`。  
- 新抓包若与本文冲突，以 **新全量抓包 + 对应 JS 符号** 为准并更新本节日期。  

---

## 9. 参数分层：每一层从哪来（静态）

> 目的：实现时知道「该自己生成 / 该缓存 / 该抄常量 / 必须调运行时 SDK」，避免再猜字段。

### 9.1 两套管线（不要混）

```text
┌─ 登录 HTTP ─────────────────────────────────────────────┐
│ UI 38514  getQrcodeRequest / checkQrconnectRequest        │
│   → commonRequest（38514）                                │
│       合并 aid + is_from_iesaccountsaas                   │
│   → account / webAccount SDK .request（18610 拦截器栈）   │
│       + 面板构造参数（29382：did/iid/app_name=assistant…） │
│       + browserInfo → account_sdk_source_info             │
│       + p_bd/p_ver/p_js_* / request_host / biz_trace_id   │
│       + bdms/sdk_glue 运行时 → a_bogus（源码无字面量）      │
└───────────────────────────────────────────────────────────┘

┌─ 开播 HTTP ─────────────────────────────────────────────┐
│ 业务 96458/93230  callAPI({api, method, param})           │
│   → getCommonParams / URL 拼装（93230 模块 80814）         │
│       ENV_INFO + appStore.appInfo                         │
│       + 路径相关 extra_*（仅 create/create_info/speed_test）│
│   → axios/fetch（credentials:include）                    │
│   → bdms 对 include 路径注入 a_bogus（/webcast/*）         │
│   → 写接口另带 x-secsdk-csrf-token（抓包可见；主进程 cookie）│
└───────────────────────────────────────────────────────────┘
```

**同一次进程里 `DEVICE_ID`/`INSTALL_ID` 共用**；登录 query 字段名是 `did`，开播是 `device_id`，值相同。

### 9.2 设备 ID / Install ID（最底层，主进程）

**文件：** `resources/app/index.js` · `deviceIdManage`

```text
启动 → deviceIdManage(appStore)
  读 appStore.appInfo.{DEVICE_ID, INSTALL_ID, CHECK_CODE, CHANNEL, APP_ID, …}
  CHECK_CODE = `${channel}-${appVersion}-${aid}-${deviceId}[-BOE]`
  若 did 有效且 CHECK_CODE 未变且 iid 长度>1 → 直接复用，不重新注册
  否则:
    POST {DOMAIN}/service/2/desktop/device_register/
      入参（经 native helper k(e)）:
        aid, channel, package="webcast_mate",
        version=3, sub_version=3,
        key="I+D&*76:j27kVH<us9&d",   // 固定盐，给 native 注册用
        app_name=encodeURIComponent("直播伴侣"),
        app_version, device_id, install_id, registryUrl
    响应: device_id_str, install_id_str, device_model, os_mac, …
    再 POST …/service/2/app_alert_check/ 做激活
    写回 appStore.appInfo + 新 CHECK_CODE
```

| 存储 | 路径 |
|------|------|
| 运行时 | `window.__STORE__.appStore.appInfo.DEVICE_ID` / `INSTALL_ID` |
| 落盘 | `AppData/Roaming/webcast_mate/WBStore/appStore.json`（及 storeBackup） |

抓包样例：`did=2064230385197255`，`iid=2064230385201351`。

实现含义：
- **did/iid 应稳定持久化**（对齐 companion 的 device 注册语义），不要每次随机新 ID 却又和 Wine 共用一个桶。
- 完整复刻可打 `device_register`；最小可用是「自管一对稳定 did/iid」，但风控画像会弱于真注册。

### 9.3 开播公共 query（`callAPI` → `getCommonParams`）

**文件：** `93230` 导出 `getCommonParams` / URL builder（模块约 80814）

```text
getCommonParams():
  ENV_INFO = main bridge call("ENV_INFO")   // 缓存
  取 ENV_INFO 子集:
    ac, app_name, version_code, device_platform,
    webcast_sdk_version, resolution, os_version, language
  再并上 appInfo:
    aid=APP_ID, live_id=LIVE_ID, channel=CHANNEL,
    device_id=DEVICE_ID, iid=INSTALL_ID
```

**ENV_INFO 主进程构造（`index.js`）近似：**

```text
ac: "wifi"
app_name: package.json name          // 实际抓包 app_name=webcast_mate
version_code: package version        // 12.7.3
device_platform: "windows"
resolution: primaryDisplay width*height
os_version: process.getSystemVersion()
os_username / os_arch / software_arch
webcast_sdk_version: 1520            // 写死
language: "zh"
device_id: appInfo.DEVICE_ID（可空）
```

**URL 拼装：**

```text
base = DOMAIN + api          // DOMAIN 默认 https://webcast.amemv.com
query = getCommonParams() ∪ customQuery ∪ pathExtra(api)
GET 时 body param 也可并进 query
最终: base + "?" + stringify(query)
```

**域名容灾（`callAPIV2`）：**
`webcast.amemv.com` 失败时可换 `webcast-pc.amemv.com` / `webcast-normal.amemv.com`（仅 GET 超时类）。  
**本次成功抓包主 host 是 `webcast.amemv.com`。**

### 9.4 开播路径附加 query（`parseExtraLiveStreamSchedulingQuery`）

**仅当 api ∈**

```text
/webcast/room/create_info/
/webcast/room/create/
/webcast/room/push_stream/speed_test/
```

才附加：

| 键 | 来源 |
|----|------|
| `extra_first_tag_id` | `userStore.anchorTags.first_tag_id`（vertical_label 等预拉） |
| `extra_second_tag_id` | 同上 second |
| `extra_third_tag_id` | 同上 third |
| `extra_encoder_core` | settings 视频编码 core，lower |
| `extra_codec_name` | 编码名 lower |
| `extra_codec_is_ex` | 名是否 `_EX` 后缀 |
| `extra_use_265` | 是否 265 |
| `extra_login_option` | 仅 PICO 登录源 |

普通 `ping/anchor`、`user/me` **没有**这组 extra_*（与抓包一致：ping query 更短）。

### 9.5 开播 Header

| Header | 来源 |
|--------|------|
| `Content-Type` | 默认 `application/x-www-form-urlencoded; charset=UTF-8` |
| `Cookie` | jar / `credentials:"include"`（桌面 session） |
| `User-Agent` | Electron 壳 `webcast_mate/12.7.3 …` |
| `x-secsdk-csrf-token` | 登录后写接口抓包可见；与 `csrf_session_id` cookie 同期出现（secsdk 运行时，非 38514） |
| `X-TT-ENV` / `X-USE-PPE` / `X-USE-BOE` | 仅非 prod / BOE |

`callAPINative`：`noCommonParams + noCommonHeaders`，给已拼好整 URL 的调用。

### 9.6 登录参数栈（passport）

#### 业务层（38514）

| 调用 | 显式 params/data |
|------|------------------|
| `getQrcodeRequest` | `next`, `need_logo=false`, `need_short_url=false`, `is_new_login=1` |
| `checkQrconnectRequest` | 上列 + `token`, `is_frontier`（轮询写死 true）；web scope 的 `next` |
| 出码前 | `generateVerifyPortraitId()` |

`commonRequest` 只保证再并：`aid`、`is_from_iesaccountsaas=1`（+ 可选 `is_vcd`）。

#### 面板 / 账号构造（29382 等）

```text
APP_ID.AWEME(2079) → app_name 映射 "aweme_live_assistant"
登录面板: aid, did=DEVICE_ID, iid=INSTALL_ID,
  host/domain = streamingtool 域名函数,
  loginType=["LOGIN_MOBILE_CODE"], captchaHost, …
```

抓包 passport query 用 **`did=`**（不是 device_id）。部分旧 helper 映射里有 `device_id: did` + `fp: verify_${did}`，**以抓包键名为准**。

#### SDK 拦截器（18610）

| 参数 | 来源（静态） |
|------|----------------|
| `account_sdk_source_info` | `JSON.stringify(browserInfo)` 再自定义编码 `c(...)`；`browserInfo` 异步收集 |
| `p_bd` | `window._sdkGlueVersionMap.bdmsVersion`（抓包 `1.0.1.20`） |
| `p_ver` / `p_js_v` / `p_js_t` / `p_zt` | SDK/`$SECURE_VERSION` 常量钉 |
| `request_host` | `encodeURIComponent(location.origin)` → Electron 下常为 `file://` |
| `biz_trace_id` | 本地 trace 对象；同时可打 header `x-tt-passport-trace-id` |
| `passport_jssdk_version` / `type` | 拦截器默认（抓包 2.4.13 / normal；包内另有 4.2.3/lite 分支） |

**源码字符串里没有 `a_bogus` 字面量。** 登录/开播 URL 上的 `a_bogus` 来自运行时 bdms（见 §9.7）。

### 9.7 签名：`a_bogus` / `X-Bogus` / bdms（运行时）

#### 加载（`2301.721857f5.js`）

```text
window._SdkGlueInit({
  self: { aid: 2079, pageId: 40236 },   // aid→pageId 表：2079→40236
  bdms: {
    paths: { include: ["/webcast/*", …按容器类型追加] },
    ddrt: 3,
    aid, pageId
  }
})
// 动态插入 script；版本进 window._sdkGlueVersionMap.bdmsVersion → 登录 p_bd
```

对 **webcast 业务**：bdms `include` 至少覆盖 `"/webcast/*"`，因此
`create_info` / `create` / `ping/anchor` 等都会被自动加签到 query（抓包每条都有 `a_bogus`）。

对 **passport**：同样走页面内 sdk_glue / 账号 SDK 请求栈，get_qrcode / check_qrconnect 的 query 也带 `a_bogus`（抓包证实）；**不是** 38514 手写字段。

#### 另一类：`window.byted_acrawler.frontierSign`（IM 等）

```text
// 12105 / 5914 / 71047 等
stub = { "X-MS-STUB": md5( selected_param_values_joined ) }
out  = byted_acrawler.frontierSign(stub)
// 若返回 X-Bogus → 改名为 signature 并进 query
```

这是 **长连接/IM fetch** 路径，与开播 `room/create` 主链的 `a_bogus` 不是同一处拼装，但同属字节 acrawler/bdms 家族。

#### 实现含义

| 能力 | 静态能否直接抄 | 说明 |
|------|----------------|------|
| did/iid | 半可以 | 应用 `device_register` 或稳定自管 |
| ENV 公共 query | 可以 | 常量 + 分辨率/系统 |
| extra_* | 可以 | 分区标签 + 本地编码设置 |
| create body | 可以 | 96458 组 `z` 的规则 |
| ping body | 可以 | room_id/stream_id/status |
| 登录业务 body | 可以 | token/is_frontier/next… |
| account_sdk_source_info | 难 | browserInfo 编码 |
| **a_bogus** | **否（纯静态）** | 必须 bdms/glue 或等价实现；登录+开播都要 |
| x-secsdk-csrf | 运行时 | 随登录 cookie/secsdk |

### 9.8 从「参数来源」看最小复刻顺序

1. **设备**：`device_register`（或持久 did/iid）→ 写入本地配置  
2. **会话**：passport 登录（业务字段按 §1 + §9.6；**a_bogus 必接**）→ secrets.cookie  
3. **开播 query**：getCommonParams 等价物 + create 路径 extra_* + **a_bogus**  
4. **开播 body**：create_info → pre_schedule_key → create body（§2.4）  
5. **状态**：ping 2 / ping 4（§2.5 / §3）  

没有第 2 步的 a_bogus，只改 `is_frontier` 或轮询间隔，**不能**称为对齐伴侣。

### 9.9 关键源码索引（参数层）

| 主题 | 位置 |
|------|------|
| device 注册 | `app/index.js` `deviceIdManage` → `…/desktop/device_register/` |
| ENV_INFO | `app/index.js`（ac/wifi/resolution/sdk 1520…） |
| getCommonParams / callAPI URL | `resource/js/93230.*.js` |
| extra_* 调度参数 | `93230` `parseExtraLiveStreamSchedulingQuery` |
| create body / ping / overLiving | `96458.*.js` |
| 扫码 UI / frontier 轮询 | `38514.*.js` |
| assistant app_name / did | `29382.*.js` |
| passport 拦截器 p_bd / source_info | `18610.*.js` |
| bdms glue 加载与 include | `2301.*.js` |
| IM X-Bogus frontierSign | `12105` / `5914` / `71047` 等 |

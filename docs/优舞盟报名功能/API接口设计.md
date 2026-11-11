# 优舞盟 API 接口设计文档

> 配套文档：[报名功能设计](优舞盟报名功能.md)、[小程序页面与功能设计](小程序页面/优舞盟小程序页面与功能设计.md)、[后台管理页面与功能设计](后台管理/优舞盟后台管理页面与功能设计.md)、[数据库表结构设计](数据库表结构设计.md)。
> 技术栈：Go + Gin，RESTful 风格。本文档覆盖**小程序端**、**后台管理端**、**免登录 H5** 三类接口。

---

## 0. 通用规范

### 0.1 基础信息

| 项 | 约定 |
|---|---|
| 基础路径 | 小程序端 `/api/v1`；后台端 `/api/v1/admin`；音频审核 H5 `/h5` |
| 请求/响应字段命名 | **snake_case** |
| 时间格式 | ISO 8601 本地时间：`2027-01-01T10:00:00+08:00` |
| 金额单位 | **分**（整数），如 `98000` 表示 ¥980 |
| 枚举值 | 一律使用英文小写蛇形（如 `classical`/`folk`），中文枚举仅在管理端展示层转换 |
| 分页参数 | Query：`page`（默认 1）、`page_size`（默认 20，最大 100） |
| 分页响应 | 列表统一包一层 `{"list": [], "total": 0, "page": 1, "page_size": 20}` |

### 0.2 认证与鉴权

| 端 | 方式 | 说明 |
|---|---|---|
| 小程序端 | `Authorization: Bearer <token>` | 微信静默登录换发的 JWT；除登录/榜单只读接口外均必填 |
| 后台端 | `Authorization: Bearer <token>` | 账号密码登录换发的 JWT，payload 含 `admin_id`、`role`（super_admin / admin / cs） |
| 音频审核 H5 | URL 内 token | 免登录，token 不可枚举，重新生成后旧链接立即失效 |

### 0.3 统一响应包装

```json
{
  "code": 0,
  "message": "success",
  "data": {}
}
```

### 0.4 全局错误码

| code | 含义 | HTTP Status |
|---|---|---|
| 0 | 成功 | 200 |
| 40001 | 参数错误 / 校验失败 | 400 |
| 40101 | 未登录或 token 失效 | 401 |
| 40301 | 无权限（角色不允许） | 403 |
| 40401 | 资源不存在 | 404 |
| 40901 | 业务冲突（重复报名、名额已满等） | 409 |
| 42901 | 触发限流 | 429 |
| 50001 | 服务器内部错误 | 500 |

### 0.5 通用业务错误码（报名域，供小程序端精确提示）

| code | 说明 |
|---|---|
| 41001 | 身份证号格式不合法 |
| 41002 | 不符合赛事年龄要求 |
| 41003 | 该组别已有有效报名（一人一组别） |
| 41004 | 该舞种已报名 |
| 41005 | 名额已满（引导候补） |
| 41006 | 赛事不在报名期 |
| 41007 | 组别已关闭 |
| 41008 | 已过资料上传截止时间（资料只读） |
| 41009 | 报名状态不允许修改资料（未支付/已取消/已退赛） |
| 41010 | 该证件号已在候补队列中（同报名单元不可重复排队） |
| 41011 | 候补已通知/已转报名，不可自助取消（需联系客服） |
| 41012 | 赛事状态不允许该操作（如重复发布、已下架后编辑/流转） |
| 41013 | 状态不允许该操作（候补队列 / 退款工单） |
| 41014 | 微信支付退款 API 调用失败（工单置 refund_failed，可重试） |
| 41015 | 当前订单不满足退款条件（非已支付 / 已过报名截止） |

---

# 第一部分：小程序端接口（`/api/v1`）

## 1. 认证模块

### 1.1 微信静默登录

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 微信静默登录 |
| 功能说明 | 小程序 `wx.login` 获取 code，后端换取 openid，按 openid 创建/匹配账号并签发 JWT |
| 请求方法 + URL | `POST /api/v1/auth/wx-login` |
| 鉴权 | 否 |

**请求参数（Body）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| code | string | 是 | wx.login 返回的临时登录凭证 | `"071XaZ...”` |
| nickname | string | 否 | 微信昵称 | `"小雨"` |
| avatar_url | string | 否 | 头像 URL | `"https://..."` |

**请求示例**

```bash
curl -X POST https://api.example.com/api/v1/auth/wx-login \
  -H "Content-Type: application/json" \
  -d '{"code":"071XaZ000abc","nickname":"小雨"}'
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| token | string | JWT，后续请求放 Authorization 头 |
| expires_in | int | 有效期（秒），默认 7200 |
| user | object | 用户信息 |
| user.id | int | 用户 ID |
| user.nickname | string | 昵称 |
| user.avatar_url | string | 头像 |
| user.phone | string | 绑定手机号，未绑定为 `null`（脱敏返回） |
| user.player_no | string | 默认选手编号，无则为 `null` |
| is_new | bool | 是否新注册 |

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "token": "eyJhbGciOi...",
    "expires_in": 7200,
    "is_new": false,
    "user": {
      "id": 1024,
      "nickname": "小雨",
      "avatar_url": "https://thirdwx.qlogo.cn/...",
      "phone": "138****5678",
      "player_no": "a8K2mP9x"
    }
  }
}
```

**失败响应**

| code | 说明 |
|---|---|
| 40001 | code 为空或已失效 |
| 50001 | 微信接口调用失败 |

**注意事项**

- 幂等：同一 code 重复提交第二次返回 40001（code 一次性）。
- 限流：单 IP 10 次/分钟。

---

### 1.2 绑定手机号

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 绑定/换绑手机号 |
| 功能说明 | 通过 `getPhoneNumber` 返回的 code 换取手机号并绑定到当前用户 |
| 请求方法 + URL | `PUT /api/v1/auth/phone` |
| 鉴权 | 是（小程序用户） |

**请求参数（Body）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| phone_code | string | 是 | 微信手机号授权 code | `"e5b3..."` |

**请求示例**

```bash
curl -X PUT https://api.example.com/api/v1/auth/phone \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"phone_code":"e5b3a1c2"}'
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| phone | string | 绑定后的手机号（脱敏） |

```json
{"code": 0, "message": "success", "data": {"phone": "138****5678"}}
```

**错误码**：40001（code 失效）；50001（微信接口失败）。

**注意事项**：不绑定手机号不影响任何功能；重复绑定即覆盖。

---

## 2. 赛事模块

### 2.1 赛事列表

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 赛事列表 |
| 功能说明 | 小程序赛事列表 Tab，按状态筛选，含各组别剩余名额热度 |
| 请求方法 + URL | `GET /api/v1/events` |
| 鉴权 | 否（可选 token，用于标记"已报名"角标） |

**请求参数（Query）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| status | string | 否 | 状态筛选：`registering`/`upcoming`/`reg_closed`/`finished`，缺省全部 | `registering` |
| page / page_size | int | 否 | 分页 | `1` / `20` |

**请求示例**

```bash
curl "https://api.example.com/api/v1/events?status=registering&page=1&page_size=20"
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| list[].id | int | 赛事 ID |
| list[].name | string | 赛事名称 |
| list[].level | string | `grade_a`（甲级赛）/ `super`（超级赛）/ `final`（年度总决赛） |
| list[].city | string | 举办城市 |
| list[].venue | string | 比赛地点 |
| list[].event_date | string | 比赛日期（YYYY-MM-DD） |
| list[].cover_url | string | 封面图 URL |
| list[].fee | int | 报名费（分），总决赛为 0 |
| list[].status | string | registering / upcoming / reg_closed / finished |
| list[].reg_end_at | string | 截止报名时间 |
| list[].group_summary | array | 各组别×舞种报名热度：`[{"level":"U11","dance_type":"classical","registered":37,"quota":40}]` |
| list[].registered_by_me | bool | 当前用户是否有有效报名（未登录恒 false） |

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "list": [{
      "id": 12,
      "name": "2027 上海甲级赛",
      "level": "grade_a",
      "city": "上海",
      "venue": "上海国际舞蹈中心",
      "event_date": "2027-03-15",
      "cover_url": "/static/events/12/cover.jpg",
      "fee": 98000,
      "status": "registering",
      "reg_end_at": "2027-02-01T18:00:00+08:00",
      "group_summary": [
        {"level": "U11", "dance_type": "classical", "registered": 37, "quota": 40},
        {"level": "U11", "dance_type": "folk", "registered": 20, "quota": 40}
      ],
      "registered_by_me": false
    }],
    "total": 3, "page": 1, "page_size": 20
  }
}
```

**错误码**：无业务错误码。

**注意事项**：只返回后台已发布（`status != draft/unpublished/offline`）的赛事；名额热度为近似值（读多写少场景不强制实时，缓存 30 秒）。

---

### 2.2 赛事详情

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 赛事详情 |
| 功能说明 | 展示头图、时间线、组别名额、费用与规则、荣誉奖金；报名页预校验数据 |
| 请求方法 + URL | `GET /api/v1/events/{event_id}` |
| 鉴权 | 否（可选 token） |

**请求参数（Path）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| event_id | int | 是 | 赛事 ID | `12` |

**请求示例**

```bash
curl "https://api.example.com/api/v1/events/12"
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| id / name / level / city / venue / event_date / cover_url / fee / status | — | 同列表 |
| timeline | object | 五节点：`reg_start_at`、`reg_end_at`、`lottery_at`、`material_check_at`、`start_at` |
| groups | array | 报名单元列表（赛事 × 组别 × 舞种） |
| groups[].group_id | int | 报名单元 ID |
| groups[].level | string | U11 / U13 / U15 |
| groups[].dance_type | string | classical / folk |
| groups[].birth_year_from / birth_year_to | int | 出生年份范围（只读展示） |
| groups[].quota / registered / remaining | int | 名额 / 已报 / 剩余 |
| groups[].enabled | bool | false 时不可报名 |
| refund_policy | string | 退费政策文案 |
| work_duration_limit | string | 作品时长要求 |
| process_desc / honor_desc | string | 比赛流程 / 荣誉奖金说明 |

```json
{
  "code": 0, "message": "success",
  "data": {
    "id": 12, "name": "2027 上海甲级赛", "level": "grade_a",
    "city": "上海", "venue": "上海国际舞蹈中心", "event_date": "2027-03-15",
    "cover_url": "/static/events/12/cover.jpg", "fee": 98000, "status": "registering",
    "timeline": {
      "reg_start_at": "2027-01-01T09:00:00+08:00",
      "reg_end_at": "2027-02-01T18:00:00+08:00",
      "lottery_at": "2027-02-05T10:00:00+08:00",
      "material_check_at": "2027-03-01T18:00:00+08:00",
      "start_at": "2027-03-15T09:00:00+08:00"
    },
    "groups": [{
      "group_id": 88, "level": "U11", "dance_type": "classical",
      "birth_year_from": 2016, "birth_year_to": 2018,
      "quota": 40, "registered": 37, "remaining": 3, "enabled": true
    }],
    "refund_policy": "抽签前可无偿退赛，全额原路退回；抽签后个人原因退赛扣除 50%……",
    "work_duration_limit": "≤3 分钟",
    "process_desc": "常规赛 → 20% 晋级决赛 → 决赛 → 当天颁奖",
    "honor_desc": "……"
  }
}
```

**错误码**：40401（赛事不存在或未发布）。

**注意事项**：已下架（offline）赛事对已持有有效报名用户的"我的报名"仍可见，但详情页访问返回 40401。

---

## 3. 报名与支付模块

### 3.1 提交报名（创建订单并拉起支付）

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 提交报名 |
| 功能说明 | 校验身份证 → 判定组别 → 校验重复/名额 → 事务内创建报名单 + 订单 + 微信支付预下单，返回收银台参数。点击支付即调起微信支付；用户关闭/取消/支付失败由回调落库为已取消 |
| 请求方法 + URL | `POST /api/v1/events/{event_id}/registrations` |
| 鉴权 | 是（小程序用户） |

**请求参数（Path）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| event_id | int | 是 | 赛事 ID | `12` |

**请求参数（Body）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| player_name | string | 是 | 选手姓名（≤30 字） | `"张小雨"` |
| id_card_no | string | 是 | 大陆身份证号（**仅本次传输使用明文，服务端立即哈希，不落明文、不进日志**） | `"3101...1234"` |
| institution | string | 否 | 机构名称 | `"小天鹅舞蹈工作室"` |
| dance_type | string | 是 | 舞种：`classical` / `folk` | `"classical"` |
| idempotency_key | string | 是 | 幂等键：客户端生成的 UUID，重试时复用；服务端按 `UNIQUE(user_id, idempotency_key)` 判重，命中返回首次创建的订单 | `"9f2c..."` |

**请求示例**

```bash
curl -X POST https://api.example.com/api/v1/events/12/registrations \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "player_name": "张小雨",
    "id_card_no": "310115201405123456",
    "institution": "小天鹅舞蹈工作室",
    "dance_type": "classical",
    "idempotency_key": "9f2c8a7e-1b3d-4e5f-9a0b-1234567890ab"
  }'
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| order_no | string | 商户订单号 |
| order_id | int | 订单 ID（T10 起返回，供直接调用预下单接口换取支付参数） |
| registration_id | int | 报名单 ID |
| amount | int | 应付金额（分） |
| level / dance_type | string | 服务端判定回填的组别 / 舞种 |
| material_deadline | string | 资料上传截止（= 核对舞蹈音频时间） |

```json
{
  "code": 0, "message": "success",
  "data": {
    "order_no": "2027010109300012345678",
    "order_id": 9001,
    "registration_id": 5001,
    "amount": 98000,
    "level": "U13",
    "dance_type": "classical",
    "material_deadline": "2027-03-01T18:00:00+08:00"
  }
}
```

**失败响应（业务错误）**

| code | 说明 | 前端处理 |
|---|---|---|
| 41001 | 身份证号不合法 | 停留表单页提示 |
| 41002 | 不符合年龄要求（附匹配到的组别区间） | 提示后返回赛事列表 |
| 41003 | 该赛事已报名其他级别组别（一人一组别） | 提示可新增其他舞种 |
| 41004 | 该舞种已报名 | 提示勿重复提交 |
| 41005 | 名额已满 | 引导候补表单 |
| 41006 | 不在报名期 | 返回详情页 |
| 41007 | 组别已关闭 | 返回详情页 |

```json
{"code": 41005, "message": "该组别名额已满，可提交候补信息", "data": {"group_id": 88, "level": "U13", "dance_type": "classical"}}
```

**注意事项**

- **幂等**：`idempotency_key` + 服务端唯一索引 `UNIQUE(user_id, idempotency_key)`，重复提交返回首次创建的订单；网络重试安全。判重永久生效（强于早期"5 分钟内"的表述）；若订单已结束（已支付/已取消），同键再提交返回 40901，客户端应换新幂等键重新发起报名。
- **支付参数拆分（T9 落地口径）**：本接口只创建报名单 + 订单，响应中 `pay_params` 由支付预下单接口 `POST /api/v1/orders/{order_id}/prepay`（T10）换取，小程序拿到后再拉起微信支付。组别 `level` 由服务端按身份证出生年份判定回填，请求体不携带。
- **并发抢名额**：名额校验与写单在同一事务内完成（`taken < quota` 原子条件 UPDATE），SQLite 单写入者天然串行，仅一人成功。
- **安全**：身份证明文仅存在于请求体内存，立即 HMAC-SHA256 哈希后丢弃；日志、错误信息、数据库中均无明文。
- **限流**：单用户 1 次/5 秒。

---

### 3.1b 支付预下单（换取 pay_params）

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 支付预下单 |
| 功能说明 | 对已创建的待支付订单调用微信支付 V3 预下单（JSAPI），返回小程序拉起微信支付所需的 pay_params。重复调用复用已持久化的 `prepay_id`，不向微信重复下单 |
| 请求方法 + URL | `POST /api/v1/orders/{order_id}/prepay` |
| 鉴权 | 是（小程序用户，仅订单本人可调） |

**请求参数（Path）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| order_id | int | 是 | 订单 ID（3.1 响应的 `order_id`） | `9001` |

无请求体。

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| order_no | string | 商户订单号 |
| pay_params | object | 微信支付参数（**微信协议字段为 camelCase，本文档级例外**）：`timeStamp` / `nonceStr` / `package`（`prepay_id=...`）/ `signType`（`RSA`）/ `paySign` |

```json
{
  "code": 0, "message": "success",
  "data": {
    "order_no": "2027010109300012345678",
    "pay_params": {
      "timeStamp": "1767221400",
      "nonceStr": "a1b2c3d4e5f6",
      "package": "prepay_id=wx20270101114000001234567890",
      "signType": "RSA",
      "paySign": "SHA256-RSA 签名"
    }
  }
}
```

**失败响应（业务错误）**

| code | 说明 | 前端处理 |
|---|---|---|
| 40001 | 参数错（order_id 非数字） | — |
| 40101 | 未登录 | 引导登录 |
| 40401 | 订单不存在或不属于当前用户 | 刷新报名列表 |
| 40901 | 订单状态不允许（非待支付） | 刷新报名状态 |
| 42901 | 触发限流（10 次/分钟） | 稍后重试 |
| 50001 | 微信支付网关失败 | 稍后重试 |

**注意事项**

- **幂等复用**：`prepay_id` 持久化在订单上（条件写入：`status='pending_payment' AND prepay_id IS NULL`），同一订单重复预下单直接复用，并发请求按订单串行化，网关只调用一次。
- **mock 通道**：`PAY_MOCK=true`（默认开启）时由 MockGateway 返回 `mock-prepay-*` 与 `MOCK-SIGN`，小程序可完整走通支付流程；接入真实微信支付后关闭。
- **限流**：单用户 10 次/分钟。
- **安全**：签名私钥只经环境变量注入，不落库、不进日志；日志中不打印支付密钥与完整回调报文之外的内容。

---

### 3.1c 订单状态查询（含主动查单兜底）

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 订单状态查询 |
| 功能说明 | 查询本人订单状态；订单仍待支付时主动向微信查单（回调丢失兜底），确认 SUCCESS 则执行与回调相同的结账事务，避免"钱扣了没报上" |
| 请求方法 + URL | `GET /api/v1/orders/{order_id}` |
| 鉴权 | 是（小程序用户，仅订单本人可调） |

**请求参数（Path）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| order_id | int | 是 | 订单 ID（3.1 响应的 `order_id`） | `9001` |

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| order_id | int | 订单 ID |
| order_no | string | 商户订单号 |
| status | string | 订单状态：`pending_payment` / `paid` / `canceled` / `refunding` / `refunded` |
| amount | int | 金额（分） |
| paid_at | string/null | 支付时间；未支付为 null |
| registration_id | int | 关联报名单 ID |

```json
{
  "code": 0, "message": "success",
  "data": {
    "order_id": 9001,
    "order_no": "2027010109300012345678",
    "status": "paid",
    "amount": 98000,
    "paid_at": "2027-01-01T10:34:56+08:00",
    "registration_id": 8001
  }
}
```

**失败响应（业务错误）**

| code | 说明 | 前端处理 |
|---|---|---|
| 40001 | 参数错（order_id 非数字） | — |
| 40101 | 未登录 | 引导登录 |
| 40401 | 订单不存在或不属于当前用户 | 刷新报名列表 |
| 42901 | 触发限流（30 次/分钟） | 稍后重试 |
| 50001 | 向微信查单失败 | 稍后重试，或先按待支付展示 |

**注意事项**

- 仅在订单为 `pending_payment` 时向微信发起查单；已结束订单直接返回库内状态，不调网关。
- 查单命中 SUCCESS 时执行与回调一致的幂等结账事务（写流水 → 订单 paid → 报名 active），重复查询安全。
- 查单命中 CLOSED / REVOKED / PAYERROR 时执行 T12 取消（订单 canceled、名额释放、关闭流水留档）；NOTPAY 保持待支付（用户仍可能完成支付）。
- mock 通道下查单恒返回 NOTPAY，本地联调支付成功请直接 POST 3.2 的明文回调报文。
- 限流：单用户 30 次/分钟。

---

### 3.2 微信支付结果回调

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 微信支付回调 |
| 功能说明 | 微信支付平台回调：验签 → 按 `transaction_id` 幂等 → 成功则订单置 paid、报名置 active；失败/关闭置 canceled（立即释放名额，全量留档） |
| 请求方法 + URL | `POST /api/v1/payments/wechat/notify` |
| 鉴权 | 微信平台（请求头带 `Wechatpay-Serial`/`Wechatpay-Signature`，验签鉴权） |

**请求参数（Header）**

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| Wechatpay-Timestamp / Wechatpay-Nonce / Wechatpay-Signature / Wechatpay-Serial | string | 是 | 微信支付回调签名串 |

**请求参数（Body）**：微信支付通知 JSON（resource 解密后含 `out_trade_no`、`transaction_id`、`trade_state`、`amount.total`）。

**请求示例**（解密后语义）

```json
{
  "id": "ev-20270101114000",
  "resource": {
    "out_trade_no": "2027010109300012345678",
    "transaction_id": "4200001234202701011234567890",
    "trade_state": "SUCCESS",
    "amount": {"total": 98000, "currency": "CNY"}
  }
}
```

**响应参数**：按微信支付协议返回（成功 `{"code":"SUCCESS","message":"成功"}`）。

**错误码**：验签失败返回 HTTP 401，微信会重推。

**注意事项**

- **验签**：用微信支付平台证书（`WECHAT_PAY_CERT`）公钥对 `时间戳\n随机串\n请求体\n` 做 SHA256-RSA 验签，并校验时间戳偏差 ≤5 分钟、`Wechatpay-Serial` 与平台证书序列号一致；失败返回 HTTP 401，微信重推。
- **解密**：用 APIv3 密钥（`WECHAT_PAY_KEY`，32 字节）对 `resource` 做 AES-256-GCM 解密；解密后再校验 appid/mchid 与商户配置一致。
- **幂等**：`payments.transaction_id` 唯一约束，重复回调直接返回 SUCCESS。
- **事务**：写 `payments` 流水（`raw_callback` 全量留档）→ `orders` 置 `paid`+`paid_at` → `registrations` 置 `active`，任一步失败整事务回滚并返回非 success，微信重试。
- **金额校验**：实付金额必须与订单金额一致；不一致拒绝（返回非 success）并记录错误日志告警，人工核查。
- **对账**：回调报文完整落 `payments.raw_callback`；`notified_at` 记录处理时间。查单兜底写入的流水 `raw_callback` 为 NULL（来源可区分）。
- 支付失败/关闭/用户取消：微信回调 `trade_state=CLOSED/REVOKED/PAYERROR` 时执行 T12 取消（订单 canceled、名额即时释放、关闭流水留档）；`NOTPAY` 仍可能完成支付，本接口仅确认收到、不做状态变更。用户主动取消走 3.5 接口。
- **mock 通道**：`PAY_MOCK=true` 时不做验签解密，直接接受明文交易 JSON（字段同解密后的 resource），供本地/联调模拟支付成功。

---

### 3.2b 微信退款结果回调

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 微信退款结果回调 |
| 功能说明 | 微信退款平台回调：验签 → 按 `refund_id` 幂等 → 成功则订单 refunded、报名 withdrawn、名额释放、工单 refunded |
| 请求方法 + URL | `POST /api/v1/payments/wechat/refund-notify` |
| 鉴权 | 微信平台（请求头签名串，验签鉴权，与支付回调相同） |

**请求参数（Body）**：微信退款通知 JSON（resource 解密后含 `out_trade_no`、`out_refund_no`、`refund_id`、`refund_status`、`amount.total/refund`）。

**响应参数**：按微信支付协议返回（成功 `{"code":"SUCCESS","message":"成功"}`）。

**错误码**：验签失败返回 HTTP 401，微信会重推。

**注意事项**

- 回调地址由环境变量 `WECHAT_REFUND_NOTIFY_URL` 配置；验签/解密规则与支付回调一致。
- **幂等**：`payments.refund_id` 部分唯一索引 + 工单终态判断；重复回调直接返回 SUCCESS，不重复释放名额。
- `refund_status=SUCCESS`：事务内 `refund_tickets` → refunded、`orders` → refunded、`registrations` → withdrawn、`event_groups.taken - 1`，补齐退款流水回调报文。
- `refund_status=PROCESSING`：不做状态变更，返回 SUCCESS。
- `refund_status=CLOSED/ABNORMAL`：工单回退 `refund_failed`、订单回退 `paid`（可重试或人工核查），退款流水置 failed。
- 退款接口调用超时但微信实际退款成功时，以本回调为准：即使工单已回退 refund_failed，成功回调仍会完成退款状态推进。
- **mock 通道**：`PAY_MOCK=true` 时直接接受明文退款 JSON，供本地/联调模拟退款成功。

---

### 3.3 我的报名列表

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 我的报名列表 |
| 功能说明 | 「我的-我的报名」：有效报名 + 退款处理中 + 已退款；**已取消订单不出现** |
| 请求方法 + URL | `GET /api/v1/registrations` |
| 鉴权 | 是 |

**请求参数（Query）**：无业务参数，仅分页（`page` / `page_size`）。

**请求示例**

```bash
curl "https://api.example.com/api/v1/registrations?page=1&page_size=20" \
  -H "Authorization: Bearer <token>"
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| list[].registration_id | int | 报名单 ID |
| list[].event_id / event_name / level | — | 赛事信息 |
| list[].event_date | string | 比赛日期 |
| list[].level_group | string | 组别：U11/U13/U15 |
| list[].dance_type | string | classical / folk |
| list[].player_name / player_no | string | 选手姓名 / 选手编号 |
| list[].order_no / amount | — | 订单号 / 金额（分） |
| list[].status | string | `paid`（已支付）/ `refunding`（退款处理中）/ `refunded`（已退款） |
| list[].material_status | object | 资料完成度：`{"total":8,"completed":3,"missing":["photo","audio","program_name"]}` |
| list[].material_deadline | string | 资料截止（核对舞蹈音频时间） |

```json
{
  "code": 0, "message": "success",
  "data": {
    "list": [{
      "registration_id": 5001, "event_id": 12, "event_name": "2027 上海甲级赛",
      "level": "grade_a", "event_date": "2027-03-15",
      "level_group": "U13", "dance_type": "classical",
      "player_name": "张小雨", "player_no": "a8K2mP9x",
      "order_no": "2027010109300012345678", "amount": 98000,
      "status": "paid",
      "material_status": {"total": 8, "completed": 3, "missing": ["photo", "audio", "program_name"]},
      "material_deadline": "2027-03-01T18:00:00+08:00"
    }],
    "total": 1, "page": 1, "page_size": 20
  }
}
```

**错误码**：无。

**注意事项**：`material_status.missing` 为空即"资料已齐"；资料是否传齐由字段是否为空实时计算，不存状态。

---

### 3.4 我的报名详情

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 报名详情 |
| 功能说明 | 报名状态卡 + 报名信息 + 参赛资料清单（含已传文件 URL / 文本摘要）+ 资料是否只读 |
| 请求方法 + URL | `GET /api/v1/registrations/{registration_id}` |
| 鉴权 | 是（仅本人可见） |

**请求参数（Path）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| registration_id | int | 是 | 报名单 ID | `5001` |

**请求示例**

```bash
curl "https://api.example.com/api/v1/registrations/5001" -H "Authorization: Bearer <token>"
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| status | string | paid / refunding / refunded |
| status_desc | string | 状态说明文案 |
| material_readonly | bool | 超过核对舞蹈音频时间后为 true，资料整体只读 |
| event / level_group / dance_type / player_name / player_no / institution | — | 报名信息 |
| order_no / amount / paid_at | — | 支付信息 |
| refund | object \| null | 退款信息：`{"ticket_no":"...","status":"processing","reason":"..."}` |
| materials | object | 8 项资料：`photo_url`、`audio_url`、`audio_filename`、`program_name`、`teacher_name`、`contact_phone`、`express_address`、`work_intro`、`special_req`、`updated_at`；未传字段为 null |

```json
{
  "code": 0, "message": "success",
  "data": {
    "status": "paid",
    "status_desc": "请在 2027 年 3 月 1 日前上传资料；不上传不影响参赛",
    "material_readonly": false,
    "event": {"id": 12, "name": "2027 上海甲级赛", "level": "grade_a", "event_date": "2027-03-15"},
    "level_group": "U13", "dance_type": "classical",
    "player_name": "张小雨", "player_no": "a8K2mP9x", "institution": "小天鹅舞蹈工作室",
    "order_no": "2027010109300012345678", "amount": 98000,
    "paid_at": "2027-01-01T11:40:12+08:00",
    "refund": null,
    "materials": {
      "photo_url": "/static/materials/12/a8K2mP9x/photo_7f3a9c2e.jpg",
      "audio_url": null, "audio_filename": null,
      "program_name": null, "teacher_name": "王老师",
      "contact_phone": "13812345678",
      "express_address": null, "work_intro": null, "special_req": null,
      "updated_at": "2027-01-02T20:11:00+08:00"
    }
  }
}
```

**错误码**：40401（不存在或非本人报名）。

**注意事项**：证件照/音频 URL 通过服务端随机文件名直接访问（`/static/materials/...`），文件名不可枚举、目录列表不公开；页面不得把他人 URL 外发。

---

### 3.5 取消订单

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 取消订单 |
| 功能说明 | 用户关闭收银台/取消支付/支付失败后取消待支付订单：立即释放名额、订单留档（仅后台对账可见） |
| 请求方法 + URL | `POST /api/v1/orders/{order_id}/cancel` |
| 鉴权 | 是（仅本人订单） |

**请求参数（Path）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| order_id | int | 是 | 订单 ID | `9001` |

**请求参数（Body，可选）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| reason | string | 否 | `user_cancel`（用户关闭/取消）/ `pay_failed`（支付失败）；缺省 `user_cancel`。`system` 为服务端内部原因，不接受客户端提交 | `user_cancel` |

空请求体等价于 `{"reason":"user_cancel"}`（小程序关闭收银台时可直接 POST）。

**请求示例**

```bash
curl -X POST "https://api.example.com/api/v1/orders/9001/cancel" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"reason":"user_cancel"}'
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| order_id | int | 订单 ID |
| order_no | string | 商户订单号 |
| status | string | 固定为 `canceled` |
| cancel_reason | string | `user_cancel` / `pay_failed` / `system` |
| canceled_at | string | 取消时间（ISO 8601 本地时间） |

```json
{
  "code": 0, "message": "success",
  "data": {
    "order_id": 9001,
    "order_no": "2027010109300012345678",
    "status": "canceled",
    "cancel_reason": "user_cancel",
    "canceled_at": "2027-01-01T09:35:12+08:00"
  }
}
```

**失败响应（业务错误）**

| code | 说明 | 前端处理 |
|---|---|---|
| 40001 | 参数错（order_id 非数字 / reason 非 user_cancel、pay_failed） | — |
| 40101 | 未登录 | 引导登录 |
| 40401 | 订单不存在或不属于当前用户 | 刷新订单 |
| 40901 | 订单已支付或已进入退款流程，无法取消 | 提示走客服退款（T15） |
| 42901 | 触发限流（30 次/分钟） | 稍后重试 |

**注意事项**

- **事务**：`orders → canceled`（写 `cancel_reason` / `canceled_at`）→ `registrations → canceled` → `event_groups.taken - 1`（名额即时释放）→ 写 `payments(type=pay, status=closed)` 留档；任一步失败整事务回滚。
- **幂等**：已取消订单重复调用返回同一留档状态，不重复释放名额、不重复写流水。
- **重新报名**：取消后可直接走 3.1 校验链重新下单，无任何超时/等待限制；取消单不在小程序「我的报名」展示。
- **并发**：与支付成功回调并发时以先到者为准。本地已取消后收到支付成功回调会拒绝结账并告警，按 T15 人工退款处理（防"钱到账但名额已释放"）。
- 已支付订单不可自助取消；退款统一由客服登记、后台人工处理（T15）。

---

### 3.6 我的历史订单

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 我的历史订单 |
| 功能说明 | 「我的-历史订单」：全部已结束订单（已支付/已取消/退款中/已退款），含已取消留档只读记录 |
| 请求方法 + URL | `GET /api/v1/orders` |
| 鉴权 | 是 |

**请求参数（Query）**：无业务参数，仅分页（`page` / `page_size`）。

**请求示例**

```bash
curl "https://api.example.com/api/v1/orders?page=1&page_size=20" \
  -H "Authorization: Bearer <token>"
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| list[].order_id / order_no | — | 订单 ID / 商户订单号 |
| list[].registration_id | int | 关联报名单 ID |
| list[].event_id / event_name | — | 赛事信息 |
| list[].level_group / dance_type | — | 组别（U11/U13/U15）/ 舞种（classical/folk） |
| list[].player_name | string | 选手姓名 |
| list[].amount | int | 金额（分） |
| list[].status | string | `paid` / `canceled` / `refunding` / `refunded` |
| list[].cancel_reason | string/null | 取消原因（仅 canceled） |
| list[].paid_at / canceled_at | string/null | 支付时间 / 取消时间 |
| list[].created_at | string | 下单时间 |

```json
{
  "code": 0, "message": "success",
  "data": {
    "list": [{
      "order_id": 9002, "order_no": "2027010109300012345679",
      "registration_id": 8002, "event_id": 12, "event_name": "2027 上海甲级赛",
      "level_group": "U13", "dance_type": "folk", "player_name": "张小雨",
      "amount": 98000, "status": "canceled",
      "cancel_reason": "user_cancel",
      "paid_at": null, "canceled_at": "2027-01-01T09:35:12+08:00",
      "created_at": "2027-01-01T09:30:00+08:00"
    }],
    "total": 1, "page": 1, "page_size": 20
  }
}
```

**失败响应**：40101（未登录）；42901（限流 30 次/分钟）。

**注意事项**

- **不含待支付订单**：`pending_payment` 是支付流程中间态，不在任何小程序页面展示（与 3.3 口径一致）。
- 已取消订单只读留档，用于对账与纠纷佐证；不提供任何修改操作。
- 按订单创建时间倒序（id 倒序）返回，分页遵循统一分页结构。

---

## 4. 资料模块

### 4.1 更新资料文本项

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 更新资料文本项 |
| 功能说明 | 逐项保存 6 项文本资料（支持分次提交、反复覆盖，以最后一次为准）；只更新显式提交的字段 |
| 请求方法 + URL | `PUT /api/v1/registrations/{registration_id}/materials` |
| 鉴权 | 是（仅本人，且报名为有效状态） |
| 请求体 | `application/json` |

**请求参数（Path）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| registration_id | int | 是 | 报名单 ID | `5001` |

**请求参数（Body，JSON）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| program_name | string | 否 | 节目名称（≤50 字）；空串表示清空，不传表示不修改 | `"春闺梦"` |
| teacher_name | string | 否 | 指导教师（≤30 字） | `"王老师"` |
| contact_phone | string | 否 | 联系电话（11 位手机号） | `"13812345678"` |
| express_address | string | 否 | 快递地址（≤120 字） | `"上海市..."` |
| work_intro | string | 否 | 作品说明（多行文本，≤500 字） | `"..."` |
| special_req | string | 否 | 特殊要求（多行文本，≤200 字） | `"需要道具：扇子×2"` |

**请求示例**

```bash
curl -X PUT https://api.example.com/api/v1/registrations/5001/materials \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"program_name":"春闺梦","contact_phone":"13812345678"}'
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| material_status | object | 同 3.3 的资料完成度 |
| materials | object | 更新后的资料快照（同 3.4） |

```json
{
  "code": 0, "message": "success",
  "data": {
    "material_status": {"total": 8, "completed": 4, "missing": ["audio", "photo"]},
    "materials": { "...": "同报名详情 materials 结构" }
  }
}
```

**失败响应**

| code | 说明 |
|---|---|
| 40001 | 文本超长、手机号格式错误、含敏感词或请求体不是合法 JSON |
| 40401 | 报名不存在或非本人 |
| 41008 | 已过资料上传截止时间（只读） |
| 41009 | 报名状态不允许修改资料（已退款/退款处理中） |

**注意事项**

- 只更新显式提交的字段；空串表示清空（写 NULL）；全部字段都不传时直接返回当前快照，不刷新 `updated_at`。
- 文本项做服务端关键词过滤（`settings.sensitive_words` 为可选 JSON 字符串数组），无人工审核。
- 截止时间 = 赛事 `material_check_at`，到达后文本与文件整体只读。

---

### 4.2 上传/替换证件照

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 上传/替换证件照 |
| 功能说明 | 选择照片后立即上传，失败当场返回原因；重复上传覆盖旧照片（不提供删除接口） |
| 请求方法 + URL | `PUT /api/v1/registrations/{registration_id}/materials/photo` |
| 鉴权 | 是（仅本人，且报名为有效状态） |
| 请求体 | `multipart/form-data` |

**请求参数（Body，multipart/form-data）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| photo | file | 是 | 证件照：png/jpg，≤10MB | （文件） |

**请求示例**

```bash
curl -X PUT https://api.example.com/api/v1/registrations/5001/materials/photo \
  -H "Authorization: Bearer <token>" \
  -F "photo=@id_photo.jpg"
```

**响应参数**：同 4.1（`material_status` + `materials`）。

**失败响应**

| code | 说明 |
|---|---|
| 40001 | 未选择文件、扩展名非 png/jpg、内容与扩展名不一致、超过 10MB 或尺寸异常 |
| 40401 | 报名不存在或非本人 |
| 41008 | 已过资料上传截止时间（只读） |
| 41009 | 报名状态不允许修改资料 |

**注意事项**

- 扩展名与实际图片内容同时校验（解码格式必须一致）；限制 4000 万像素，防止解压炸弹。
- 上传成功后同步生成最长边 512px 的 JPEG 头像副本到 `uploads/avatars/{player_no}/` 并更新 `players.avatar_path`；旧照片与旧头像在事务提交后清理。
- 先落临时文件、校验通过后才替换，失败不影响已有照片。

---

### 4.3 上传/替换舞蹈音频

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 上传/替换舞蹈音频 |
| 功能说明 | 选择音频后立即上传，失败当场返回原因；重复上传覆盖旧音频（不提供删除接口） |
| 请求方法 + URL | `PUT /api/v1/registrations/{registration_id}/materials/audio` |
| 鉴权 | 是（仅本人，且报名为有效状态） |
| 请求体 | `multipart/form-data` |

**请求参数（Body，multipart/form-data）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| audio | file | 是 | 舞蹈音频：mp3，≤100MB，时长 ≤4 分钟 | （文件） |

**请求示例**

```bash
curl -X PUT https://api.example.com/api/v1/registrations/5001/materials/audio \
  -H "Authorization: Bearer <token>" \
  -F "audio=@dance.mp3"
```

**响应参数**：同 4.1（`material_status` + `materials`），其中 `audio_filename` 为服务端保存的文件名。

**失败响应**

| code | 说明 |
|---|---|
| 40001 | 未选择文件、扩展名非 mp3、无法解析音频、超过 100MB 或时长超过 4 分钟 |
| 40401 | 报名不存在或非本人 |
| 41008 | 已过资料上传截止时间（只读） |
| 41009 | 报名状态不允许修改资料 |

**注意事项**

- 服务端解析 MPEG 帧头计算时长，无法解析的音频直接拒绝（需重新导出为有效 mp3 后上传，不做转码）。
- 替换成功后才清理旧音频文件；失败不影响已有音频。

---

**文件存储与访问（4.1–4.3 公共）**

- 媒体文件落磁盘 `uploads/{event_id}/{player_no}/`，文件名服务端随机生成；`photo_path`/`audio_path` 只存相对路径；`uploads/` 不入库、不进镜像，生产挂 volume。
- 对外访问前缀 `/static/materials/{event_id}/{player_no}/`、`/static/avatars/{player_no}/` 映射到对应目录；文件名不可枚举、目录列表不公开。
- 照片只用于报名资料、后台查看与榜单头像，不进入任何审核流程。
- 赛事结束后删除 `uploads/{event_id}/`，`photo_path`/`audio_path` 置 NULL；头像目录永久保留。
---

## 5. 候补模块

### 5.1 提交候补

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 提交候补 |
| 功能说明 | 名额已满时提交候补信息（含必填电话），进入候补队列；有退赛时管理员按序电话通知，确认后走正常报名支付流程 |
| 请求方法 + URL | `POST /api/v1/events/{event_id}/waitlists` |
| 鉴权 | 是 |

**请求参数（Path + Body）**

| 参数名 | 位置 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|---|
| event_id | Path | int | 是 | 赛事 ID | `12` |
| group_id | Body | int | 是 | 报名单元 ID（组别×舞种） | `88` |
| name | Body | string | 是 | 选手姓名 | `"张小雨"` |
| id_card_no | Body | string | 否 | 身份证号（哈希存储；无证件号的极端情况可只登记姓名+电话，R7） | `"3101...3456"` |
| institution | Body | string | 否 | 机构 | `"小天鹅舞蹈工作室"` |
| phone | Body | string | 是 | 联系电话（11 位） | `"13812345678"` |

**请求示例**

```bash
curl -X POST https://api.example.com/api/v1/events/12/waitlists \
  -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"group_id":88,"name":"张小雨","id_card_no":"310115201405123456","phone":"13812345678"}'
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| waitlist_id | int | 候补记录 ID |
| position_no | int | 队列序号（通知顺序依据） |
| status | string | `waiting` |

```json
{"code": 0, "message": "success", "data": {"waitlist_id": 301, "position_no": 4, "status": "waiting"}}
```

**失败响应**

| code | 说明 |
|---|---|
| 41001 | 身份证号不合法（提供了 id_card_no 时校验） |
| 41002 | 不符合该组别年龄要求 |
| 41010 | 该证件号已在候补队列中 |
| 40001 | 姓名/手机号不合法；名额未满时不允许提交候补（引导直接报名） |
| 41006 | 赛事不在报名期 |
| 41007 | 组别已关闭 |
| 40401 | 赛事/组别不存在 |

**注意事项**：同报名单元同证件号仅允许一条 `waiting` 记录（部分唯一索引），无证件号登记不做重复判断；候补不收费、不预占名额。候补记录按提交的微信会话归属，可通过 5.2/5.3 查看与取消。

---

### 5.2 我的候补列表

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 我的候补 |
| 功能说明 | 「我的-我的候补」：角标数量与列表 |
| 请求方法 + URL | `GET /api/v1/waitlists` |
| 鉴权 | 是 |

**请求参数**：仅分页。

**请求示例**

```bash
curl "https://api.example.com/api/v1/waitlists" -H "Authorization: Bearer <token>"
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| list[].waitlist_id | int | 候补 ID |
| list[].event_id / event_name | — | 赛事 |
| list[].level / dance_type | string | 组别 / 舞种 |
| list[].position_no | int | 队列序号 |
| list[].status | string | waiting / notified / converted / given_up |
| list[].created_at | string | 提交时间 |

```json
{
  "code": 0, "message": "success",
  "data": {
    "list": [{
      "waitlist_id": 301, "event_id": 12, "event_name": "2027 上海甲级赛",
      "level": "U13", "dance_type": "classical",
      "position_no": 4, "status": "waiting",
      "created_at": "2027-01-03T09:12:00+08:00"
    }],
    "total": 1, "page": 1, "page_size": 20
  }
}
```

**错误码**：无。

**注意事项**：仅返回当前微信用户提交的候补（候补以电话为主要联系方式，按会话归属展示）。

---

### 5.3 取消候补

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 取消候补 |
| 功能说明 | 用户自助移出候补队列（仅移出，不涉及退款） |
| 请求方法 + URL | `DELETE /api/v1/waitlists/{waitlist_id}` |
| 鉴权 | 是（仅本人） |

**请求参数（Path）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| waitlist_id | int | 是 | 候补 ID | `301` |

**请求示例**

```bash
curl -X DELETE https://api.example.com/api/v1/waitlists/301 \
  -H "Authorization: Bearer <token>"
```

**响应参数**：`{"code": 0, "message": "success", "data": null}`

**失败响应**：40401（不存在或非本人）；41011（已转报名/已通知后不可自助取消，需联系客服）。

**注意事项**：`given_up`/`removed` 记录保留留档，不物理删除。

---

## 6. 榜单模块（只读，免鉴权）

> 与报名系统无数据关联，数据全部来自榜单域快照表。

### 6.1 榜单配置

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 榜单配置 |
| 功能说明 | Banner 文案、年龄段/舞种选项、晋级名额、更新时间；页面所有选项动态获取，不写死 |
| 请求方法 + URL | `GET /api/v1/config` |
| 鉴权 | 否 |

**请求参数**：无。

**请求示例**

```bash
curl "https://api.example.com/api/v1/config"
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| season | string | 当前赛季名，如「2027 赛季」 |
| title / subtitle | string | Banner 主/副标题 |
| levels | array | 年龄段选项：`[{"value":"U11","label":"U11（2016-2018）"}]` |
| dance_styles | array | 舞种选项：`[{"value":"classical","label":"古典舞"}]` |
| advance_quota | int | 积分晋级名额（默认 20） |
| updated_at | string | 榜单数据更新时间 |

```json
{
  "code": 0, "message": "success",
  "data": {
    "season": "2027 赛季",
    "title": "国舞菁英独舞冠军赛",
    "subtitle": "直通榜 · 积分榜",
    "levels": [
      {"value": "U11", "label": "U11（2016-2018）"},
      {"value": "U13", "label": "U13（2014-2015）"},
      {"value": "U15", "label": "U15（2012-2013）"}
    ],
    "dance_styles": [
      {"value": "classical", "label": "古典舞"},
      {"value": "folk", "label": "民族民间舞"}
    ],
    "advance_quota": 20,
    "updated_at": "2027-01-08T12:00:00+08:00"
  }
}
```

**错误码**：无。

**注意事项**：单赛季，不提供切换；接口可缓存 60 秒。

---

### 6.2 榜单数据（直通榜 + 积分榜）

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 榜单数据 |
| 功能说明 | 一次性返回指定榜单的直通榜与积分榜（整榜，前端本地搜索过滤） |
| 请求方法 + URL | `GET /api/v1/ranking` |
| 鉴权 | 否 |

**请求参数（Query）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| level | string | 是 | U11 / U13 / U15 | `U13` |
| dance_type | string | 是 | classical / folk | `classical` |

**请求示例**

```bash
curl "https://api.example.com/api/v1/ranking?level=U13&dance_type=classical"
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| board_id | int | 榜单 ID |
| updated_at | string | 更新时间 |
| direct | array | 直通榜（总决赛直通卡选手） |
| direct[].id | int | 榜单内条目 ID（透传 dancer 详情） |
| direct[].name | string | 展示名（真名/化名/编号，按后台设置） |
| direct[].avatar_url | string | 头像 |
| direct[].awards | array | 奖项罗列：`[{"event_name":"上海甲级赛","award":"冠军","prize_amount":200000}]` |
| ranking | array | 正式积分榜（按 rank 升序） |
| ranking[].id | int | 榜单内条目 ID |
| ranking[].rank | int | 当前排名 |
| ranking[].prev_rank | int | 上期排名；0 = 上期不在榜（新上榜） |
| ranking[].rank_change | int | 排名变化：正=上升，负=下降 |
| ranking[].name / avatar_url | — | 同直通榜 |
| ranking[].total_points | int | 年度总积分（取最高 3 场之和） |
| ranking[].is_direct | bool | 是否同时持有直通卡 |

```json
{
  "code": 0, "message": "success",
  "data": {
    "board_id": 32,
    "updated_at": "2027-01-08T12:00:00+08:00",
    "direct": [{
      "id": 9001, "name": "李翩翩", "avatar_url": "/static/avatars/a8K2mP9x/avatar_7f3a9c2e.jpg",
      "awards": [{"event_name": "上海甲级赛", "award": "冠军", "prize_amount": 200000}]
    }],
    "ranking": [{
      "id": 9002, "rank": 1, "prev_rank": 3, "rank_change": 2,
      "name": "王楚楚", "avatar_url": "/static/avatars/b7J3nQ2y/avatar_91c4d8a0.jpg",
      "total_points": 285, "is_direct": false
    }]
  }
}
```

**失败响应**：40001（参数缺失或非法）。

**注意事项**：隐藏上榜的选手（`hidden=1`）不出现在任何列表；接口读快照表，O(1)，可缓存 60 秒。`avatar_url` 取 `players.avatar_path`（该选手最近一次报名上传的证件照压缩副本，赛事物料清理后仍保留），无照片时返回默认头像 URL；头像文件需公开可读（榜单接口免鉴权），敏感原图不公开。

---

### 6.3 机构年度榜

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 机构年度榜 |
| 功能说明 | 机构年度总积分排名 + 荣誉说明 |
| 请求方法 + URL | `GET /api/v1/institution/ranking` |
| 鉴权 | 否 |

**请求参数**：无。

**请求示例**

```bash
curl "https://api.example.com/api/v1/institution/ranking"
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| updated_at | string | 更新时间 |
| institutions | array | 机构榜 |
| institutions[].rank | int | 排名 |
| institutions[].name | string | 机构名称 |
| institutions[].total_points | int | 年度总积分 |
| institutions[].player_count | int | 上榜选手数 |
| institutions[].honor | string \| null | `elite_3`（年度菁英机构奖）/ `top_10`（年度十佳机构奖）/ null |

```json
{
  "code": 0, "message": "success",
  "data": {
    "updated_at": "2027-01-08T12:00:00+08:00",
    "institutions": [
      {"rank": 1, "name": "小天鹅舞蹈工作室", "total_points": 1520, "player_count": 12, "honor": "elite_3"}
    ]
  }
}
```

**错误码**：无。

---

### 6.4 选手积分详情

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 选手积分详情 |
| 功能说明 | 选手详情页：排名、总积分、积分构成（最高 3 场）、参赛历史、荣誉与晋级情况 |
| 请求方法 + URL | `GET /api/v1/dancers/{entry_id}` |
| 鉴权 | 否 |

**请求参数（Path）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| entry_id | int | 是 | 榜单条目 ID（列表行 `id` 原样透传，**按榜单独立**） | `9002` |

**请求示例**

```bash
curl "https://api.example.com/api/v1/dancers/9002"
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| id / name / avatar_url | — | 基本信息 |
| board | object | 所属榜单：`{"level":"U13","dance_type":"classical"}` |
| rank / total_points | — | 当前排名 / 总积分 |
| counted_events | array | 计入排名场次（最多 3 场）：`event_name/event_date/level/award/points` |
| history | array | 本年度全部场次成绩 |
| honors | array | 荣誉：`["冠军","菁英舞者"]` |
| qualification | object \| null | 晋级情况：`{"type":"direct","source":"甲级赛第一名直通"}` 或 `{"type":"points","source":"积分榜前 20 晋级"}` |

```json
{
  "code": 0, "message": "success",
  "data": {
    "id": 9002, "name": "王楚楚", "avatar_url": "/static/avatars/b7J3nQ2y/avatar_91c4d8a0.jpg",
    "board": {"level": "U13", "dance_type": "classical"},
    "rank": 1, "total_points": 285,
    "counted_events": [
      {"event_name": "上海甲级赛", "event_date": "2027-03-15", "level": "grade_a", "award": "冠军", "points": 120}
    ],
    "history": [{"event_name": "上海甲级赛", "award": "冠军", "points": 120}],
    "honors": ["冠军"],
    "qualification": {"type": "direct", "source": "甲级赛第一名直通"}
  }
}
```

**失败响应**：40401（条目不存在）。

**注意事项**：未上榜（隐藏/无快照）不提供此页入口，接口返回 40401。

---

## 7. 音频审核 H5（免登录）

### 7.1 音频审核列表

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 音频审核（H5） |
| 功能说明 | 每场赛事一个免登录 URL，选手打开后按姓名搜索定位、试听核对音频；仅含姓名/节目名/音频三类信息 |
| 请求方法 + URL | `GET /h5/audio-review/{token}` |
| 鉴权 | 免登录（URL token，8 位大小写字母+数字，不可枚举） |

**请求参数（Query）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| keyword | string | 否 | 按选手姓名搜索（本地过滤亦可，服务端支持） | `"张小雨"` |

**请求参数（Path）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| token | string | 是 | `events.audio_review_token` | `a8K2mP9x` |

**请求示例**

```bash
curl "https://api.example.com/h5/audio-review/a8K2mP9x?keyword=%E5%BC%A0%E5%B0%8F%E9%9B%A8"
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| event_name | string | 赛事名称 |
| entries | array | 已上传音频的有效报名 |
| entries[].player_name | string | 选手姓名 |
| entries[].program_name | string | 节目名称 |
| entries[].audio_url | string | 音频播放地址 |
| entries[].updated_at | string | 音频最近更新时间 |

```json
{
  "code": 0, "message": "success",
  "data": {
    "event_name": "2027 上海甲级赛",
    "entries": [{
      "player_name": "张小雨",
      "program_name": "春闺梦",
      "audio_url": "/static/materials/12/a8K2mP9x/audio_91c4d8a0.mp3",
      "updated_at": "2027-02-20T10:00:00+08:00"
    }]
  }
}
```

**失败响应**：40401（token 无效或赛事未生成）。

**注意事项**

- **安全边界**：仅返回 `status='active'` 且已上传音频的报名；**不含**证件号、电话、机构、订单等任何敏感数据。
- **审核范围（2026-11-11 定稿）**：核对 = 音频审核，节目名与音频一起展示核对；**照片不审核、不进入本页**，不存在"资料审核页"或图片审核 token。
- token 重新生成后旧链接立即失效；单 token 限流 60 次/分钟/IP。
- 审核完整 URL 由后台按配置 `PUBLIC_BASE_URL` 拼接（`/audio-review/{token}`），不依赖请求 Host；重新生成即失效旧链接。
- 音频 URL 建议加 referer/短期签名校验，防止被站外盗链。

---

# 第二部分：后台管理端接口（`/api/v1/admin`）

> 统一说明：全部接口需 `Authorization: Bearer <admin_token>`；角色为 `cs`（客服）的可用范围见各接口"注意事项"；所有写操作落 `admin_logs` 操作日志。列表类接口的分页、筛选约定与小程序端一致。

## 8. 认证模块

### 8.1 管理员登录

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 管理员登录 |
| 功能说明 | 账号 + 密码（bcrypt 校验）签发 JWT |
| 请求方法 + URL | `POST /api/v1/admin/auth/login` |
| 鉴权 | 否 |

**请求参数（Body）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| username | string | 是 | 账号 | `"admin"` |
| password | string | 是 | 密码 | `"********"` |

**请求示例**

```bash
curl -X POST https://api.example.com/api/v1/admin/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"********"}'
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| token | string | JWT（payload 含 `admin_id`、`role`） |
| expires_in | int | 有效期（秒），默认 7200 |
| admin | object | `{"id":1,"username":"admin","role":"super_admin","name":"张三"}` |

```json
{
  "code": 0, "message": "success",
  "data": {"token": "eyJhbGciOi...", "expires_in": 7200, "admin": {"id": 1, "username": "admin", "role": "super_admin", "name": "张三"}}
}
```

**失败响应**

| code | 说明 |
|---|---|
| 40001 | 账号或密码为空 |
| 40102 | 账号不存在 |
| 40103 | 密码错误 |
| 40104 | 账号已锁定（连续 5 次密码错误，message 含剩余锁定分钟数） |
| 40302 | 账号已停用 |
| 42901 | 触发限流（同一 IP 10 次/分钟） |

**注意事项**：
- 连续 5 次密码错误锁定账号 15 分钟；锁定期间即使密码正确也返回 40104，登录成功清零失败计数；对不存在账号的连续失败探测同样累计次数并触发锁定。
- 登录成功刷新 `last_login_at` 并记 `admin_logs`（action=`admin.login`）；触发锁定时记 `admin.login_locked`（`admin_id` 为空，表示系统动作）。
- 管理员被停用或删除后，其已签发的 token 在过期前仍然有效（不做服务端吊销）；如需立即失效需后续版本引入 token 版本号机制。

---

### 8.2 修改密码

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 管理员修改密码 |
| 功能说明 | 校验旧密码后设置新密码，需登录态 |
| 请求方法 + URL | `PUT /api/v1/admin/auth/password` |
| 鉴权 | 是（任意角色，只能改自己的密码） |

**请求参数（Body）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| old_password | string | 是 | 旧密码 | `"********"` |
| new_password | string | 是 | 新密码，8–72 字节 | `"NewPass#888"` |

**请求示例**

```bash
curl -X PUT https://api.example.com/api/v1/admin/auth/password \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"old_password":"********","new_password":"NewPass#888"}'
```

**响应参数**

```json
{
  "code": 0, "message": "success",
  "data": {"updated": true}
}
```

**失败响应**

| code | 说明 |
|---|---|
| 40001 | 参数缺失 / 新密码不足 8 位或超过 72 字节 / 旧密码不正确 |
| 40101 | 未登录或 token 失效（含管理员账号已被删除） |

**注意事项**：修改成功后旧密码立即失效，当前 token 仍然有效直到过期；操作记 `admin_logs`（action=`admin.change_password`）。

---

## 9. 数据看板

### 9.1 看板总览

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 数据看板 |
| 功能说明 | 核心指标卡、待办事项、近 30 日趋势 |
| 请求方法 + URL | `GET /api/v1/admin/dashboard` |
| 鉴权 | 是（管理员全量；客服仅待办工单相关，返回字段按角色裁剪） |

**请求参数**：无。

**请求示例**

```bash
curl "https://api.example.com/api/v1/admin/dashboard" -H "Authorization: Bearer <admin_token>"
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| metrics | object | `today_registrations`（今日有效报名笔数/金额）、`total_registrations`、`total_revenue`（分）、`pending_refund_tickets`、`waitlist_total`、`incomplete_materials`、`active_events` |
| todos | object | `pending_refunds`、`new_cs_tickets`、`closing_events`（临近截止赛事） |
| event_progress | array | 各赛事组别进度：`{"event_id","event_name","level","dance_type","registered","quota","waitlist_count"}` |
| trend | array | 近 30 日：`{"date":"2027-01-01","registrations":12,"revenue":1176000}` |

```json
{
  "code": 0, "message": "success",
  "data": {
    "metrics": {
      "today_registrations": 12, "today_revenue": 1176000,
      "total_registrations": 356, "total_revenue": 34888000,
      "pending_refund_tickets": 3, "waitlist_total": 17,
      "incomplete_materials": 41, "active_events": 2
    },
    "todos": {
      "pending_refunds": 3, "new_cs_tickets": 2,
      "closing_events": [{"event_id": 12, "event_name": "2027 上海甲级赛", "days_left": 2}]
    },
    "event_progress": [
      {"event_id": 12, "event_name": "2027 上海甲级赛", "level": "U11", "dance_type": "classical", "registered": 40, "quota": 40, "waitlist_count": 5}
    ],
    "trend": [{"date": "2027-01-01", "registrations": 12, "revenue": 1176000}]
  }
}
```

**错误码**：40101 / 40301。

**注意事项**：聚合查询走缓存（60 秒），不要求实时到秒。

---

## 10. 赛事管理

### 10.1 赛事列表

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 赛事列表（后台） |
| 功能说明 | 含草稿/未发布状态，各组别报名进度 |
| 请求方法 + URL | `GET /api/v1/admin/events` |
| 鉴权 | 是（管理员；客服只读） |

**请求参数（Query）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| status | string | 否 | draft/unpublished/registering/reg_closed/ongoing/finished/offline | `registering` |
| level | string | 否 | grade_a / super / final | — |
| keyword | string | 否 | 名称关键字 | — |
| page / page_size | int | 否 | 分页 | — |

**请求示例**

```bash
curl "https://api.example.com/api/v1/admin/events?status=registering" \
  -H "Authorization: Bearer <admin_token>"
```

**响应参数**：结构同小程序端 `GET /events`，另增：`season_id`、`reg_start_at` 等完整时间线、`groups`（各组别已报/名额汇总数组）、`exclude_from_prize`、`audio_review_token`（已生成时返回）。

**错误码**：无。

**注意事项**：草稿/未发布赛事仅后台可见。

---

### 10.1b 赛事详情

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 赛事详情（后台） |
| 功能说明 | 单条赛事完整字段 + 组别报名进度（同列表项结构） |
| 请求方法 + URL | `GET /api/v1/admin/events/{event_id}` |
| 鉴权 | 是（全部 admin 角色只读） |

**请求参数（Path）**：`event_id`（必填）。

**响应参数**：同 10.1 列表项。

**失败响应**：40401（赛事不存在）。

---

### 10.2 创建赛事

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 创建赛事 |
| 功能说明 | 基本信息 + 时间线 + 组别×舞种配置 + 规则文案；保存为草稿或直接发布 |
| 请求方法 + URL | `POST /api/v1/admin/events` |
| 鉴权 | 是（admin / super_admin；**cs 无权限 → 40301**） |

**请求参数（Body）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| name | string | 是 | 赛事名称 | `"2027 上海甲级赛"` |
| season_id | int | 是 | 所属赛季 | `1` |
| level | string | 是 | grade_a / super / final | `"grade_a"` |
| city / venue | string | 否 / 是 | 城市 / 地点 | `"上海"` |
| event_date | string | 是 | 比赛日期 YYYY-MM-DD | `"2027-03-15"` |
| fee | int | 是 | 报名费（分）；总决赛默认 0 | `98000` |
| cover | file | 否 | 封面图（multipart 或先传文件接口） | — |
| timeline | object | 是 | 五节点，强制先后：`reg_start_at < reg_end_at < lottery_at < material_check_at < start_at` | — |
| groups | array | 是 | 报名单元：`[{"level":"U11","dance_type":"classical","quota":40}]`，至少 1 个；出生年份范围从赛季规则自动复制 |
| refund_policy / work_duration_limit / process_desc / honor_desc | string | 否 | 规则文案，缺省用默认模板 | — |
| score_rules / prize_rules | string(JSON) | 否 | 积分/奖金规则（合法 JSON 文本），缺省用系统默认模板预填 | — |
| exclude_from_prize | bool | 否 | 特殊场次不参与自动核算奖金 | `false` |
| publish | bool | 否 | true = 保存并发布（校验时间线与名额必填） | `false` |

**请求示例**

```bash
curl -X POST https://api.example.com/api/v1/admin/events \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "2027 上海甲级赛", "season_id": 1, "level": "grade_a",
    "city": "上海", "venue": "上海国际舞蹈中心", "event_date": "2027-03-15",
    "fee": 98000,
    "timeline": {
      "reg_start_at": "2027-01-01T09:00:00+08:00",
      "reg_end_at": "2027-02-01T18:00:00+08:00",
      "lottery_at": "2027-02-05T10:00:00+08:00",
      "material_check_at": "2027-03-01T18:00:00+08:00",
      "start_at": "2027-03-15T09:00:00+08:00"
    },
    "groups": [
      {"level": "U11", "dance_type": "classical", "quota": 40},
      {"level": "U11", "dance_type": "folk", "quota": 40}
    ],
    "publish": false
  }'
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| event_id | int | 赛事 ID |
| status | string | draft / registering（publish=true 时） |

**失败响应**

| code | 说明 |
|---|---|
| 40001 | 时间线顺序错误 / 组别缺失 / 名额非法 |
| 40401 | 赛季不存在 |
| 40901 | 赛季非进行中（只有 active 赛季才能创建赛事） |

**注意事项**：发布（`publish=true`）后 `status=registering`（若未到 `reg_start_at` 则对外呈现为 upcoming，由时间线推导）；创建/发布均记操作日志。

---

### 10.3 编辑赛事

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 编辑赛事 |
| 功能说明 | 草稿/未发布可全量编辑；已发布赛事**时间线、级别不可改，名额只能调大** |
| 请求方法 + URL | `PUT /api/v1/admin/events/{event_id}` |
| 鉴权 | 是（admin / super_admin） |

**请求参数**：Path 传 `event_id`；Body 同 10.2（`publish` 字段除外）。

**请求示例**

```bash
curl -X PUT https://api.example.com/api/v1/admin/events/12 \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"name":"2027 上海甲级赛（春季）","fee":98000,"groups":[{"group_id":88,"quota":50}]}'
```

**响应参数**：`{"event_id": 12, "status": "registering"}`

**失败响应**

| code | 说明 |
|---|---|
| 40001 | 已发布赛事修改了时间线/级别，或名额小于当前已支付人数 |
| 40401 | 赛事不存在 |
| 41012 | 赛事状态不允许编辑（已结束） |

**注意事项**：已发布赛事调大名额即时生效（小程序端剩余名额刷新）；如需变更时间线/级别须「下架 + 新建」。`groups` 传法：带 `group_id` 按 ID 定位已有组别；不带 `group_id` 则按 `level + dance_type` 匹配已有组别（用于按勾选整体提交），匹配不到时草稿视为新增组合、已发布赛事返回 40001。

---

### 10.4 发布 / 下架赛事

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 发布赛事 / 下架赛事 |
| 功能说明 | 发布：小程序端立即可见（校验时间线/名额必填）；下架：小程序端立即不可见不可报，已支付报名不受影响 |
| 请求方法 + URL | `POST /api/v1/admin/events/{event_id}/publish`；`POST /api/v1/admin/events/{event_id}/offline` |
| 鉴权 | 是（admin / super_admin） |

**请求参数（Path）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| event_id | int | 是 | 赛事 ID | `12` |

**请求示例**

```bash
curl -X POST https://api.example.com/api/v1/admin/events/12/publish \
  -H "Authorization: Bearer <admin_token>"
```

**响应参数**：`{"event_id": 12, "status": "registering"}`（offline 后 status 为 `offline`）。

**失败响应**：40401；40001（发布校验未通过：时间线缺失/无组别）；41012（状态不允许，如重复发布）。

**注意事项**：均为不可逆敏感操作，前端必须二次确认；记操作日志（含操作人、时间）。发布来源状态为 draft / unpublished；下架为终止态，下架后不可再编辑或流转（须新建赛事）。

---

### 10.4b 关闭报名

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 关闭报名 |
| 功能说明 | registering → reg_closed：停止新报名（已支付报名不受影响），如报名截止后手动收尾 |
| 请求方法 + URL | `POST /api/v1/admin/events/{event_id}/close-registration` |
| 鉴权 | 是（admin / super_admin） |

**请求参数（Path）**：同 10.4（`event_id`）。

**响应参数**：`{"event_id": 12, "status": "reg_closed"}`

**失败响应**：40401；41012（非 registering 状态，如重复关闭）。

**注意事项**：记操作日志（前后状态）；关闭后小程序端该赛事不可再报名。

---

### 10.5 调整名额 / 关闭 / 重新开放组别

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 调整名额 |
| 功能说明 | 加名额（只能调大，不小于当前已支付人数） |
| 请求方法 + URL | `PUT /api/v1/admin/events/{event_id}/groups/{group_id}/quota` |
| 鉴权 | 是（admin / super_admin） |

**请求参数（Body）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| quota | int | 是 | 新名额总数 | `50` |

**请求示例**

```bash
curl -X PUT https://api.example.com/api/v1/admin/events/12/groups/88/quota \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" -d '{"quota":50}'
```

**响应参数**：`{"group_id": 88, "quota": 50, "registered": 40, "remaining": 10}`

**失败响应**：40001（quota ≤ 当前已支付人数或非法）。

**接口概述（关闭/重开组别）**

| 项 | 内容 |
|---|---|
| 接口名称 | 关闭组别 / 重新开放组别 |
| 功能说明 | 关闭后小程序端该组别×舞种立即不可报名，已支付报名不受影响；可恢复 |
| 请求方法 + URL | `POST /api/v1/admin/events/{event_id}/groups/{group_id}/close`；`POST /api/v1/admin/events/{event_id}/groups/{group_id}/reopen` |
| 鉴权 | 是（admin / super_admin） |

**请求示例**

```bash
curl -X POST https://api.example.com/api/v1/admin/events/12/groups/88/close \
  -H "Authorization: Bearer <admin_token>"
```

**响应参数**：`{"group_id": 88, "enabled": false}`

**失败响应**：40401；41012（已结束/已下架赛事不可操作）。

**注意事项**：关闭/重开/调名额即时生效且必须二次确认；记操作日志（前后值）。

---

### 10.6 音频审核链接

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 生成 / 重新生成音频审核链接 |
| 功能说明 | 生成 8 位随机 token；重新生成后旧链接立即失效 |
| 请求方法 + URL | `POST /api/v1/admin/events/{event_id}/audio-review-token` |
| 鉴权 | 是（admin / cs 均可） |

**请求示例**

```bash
curl -X POST https://api.example.com/api/v1/admin/events/12/audio-review-token \
  -H "Authorization: Bearer <admin_token>"
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| url | string | 完整 H5 地址 |
| token | string | token |

```json
{"code": 0, "message": "success", "data": {"url": "https://api.example.com/h5/audio-review/a8K2mP9x", "token": "a8K2mP9x"}}
```

**错误码**：40401。

**注意事项**：重新生成需二次确认（旧链接立即失效）；记操作日志。

---

### 10.7 赛季管理（简化）

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 赛季列表 / 创建 / 编辑 |
| 功能说明 | seasons CRUD（简化）：赛季只做时间范围与归档，组别年龄规则（group_rules）创建时从系统设置复制、只读 |
| 请求方法 + URL | `GET /api/v1/admin/seasons`；`POST /api/v1/admin/seasons`；`PUT /api/v1/admin/seasons/{season_id}` |
| 鉴权 | 读：全部 admin 角色；写：admin / super_admin（**cs 无权限 → 40301**） |

**请求参数（Body，创建/编辑共用）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| name | string | 是 | 赛季名称 | `"2027 赛季"` |
| start_date / end_date | string | 否 | YYYY-MM-DD，起始不得晚于结束 | `"2027-01-01"` |
| status | string | 编辑时可填 | upcoming / active / archived，只允许向前流转（upcoming → active → archived，可跳级归档） | `"active"` |

**创建响应**：完整赛季对象（含 `group_rules`、`archived_at`）；创建时状态恒为 `upcoming`，忽略请求中的 status。

**编辑响应**：完整赛季对象；转入 `archived` 时服务端自动写 `archived_at`。

**失败响应**

| code | 说明 |
|---|---|
| 40001 | 日期格式非法 / 起始晚于结束 |
| 40401 | 赛季不存在 |
| 40901 | 状态流转非法（回退、归档后变更）；激活时已有其他 active 赛季 |

**注意事项**：
- **全系统同时最多一个 active 赛季**；只有 active 赛季才能创建赛事（否则 40901）。
- 赛季被赛事引用后仅可改名/调整日期/推进状态，不可删除。
- 创建/编辑/状态流转均记 `admin_logs`（action=`admin.season.create` / `admin.season.update`）。

---

## 11. 订单中心

### 11.1 订单列表

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 订单管理列表 |
| 功能说明 | 全量订单查询（默认不含已取消，勾选后含留档）；支持导出 |
| 请求方法 + URL | `GET /api/v1/admin/orders` |
| 鉴权 | 是（admin 全量；cs 只读） |

**请求参数（Query）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| event_id | int | 否 | 赛事 | `12` |
| level | string | 否 | 组别 | `U13` |
| dance_type | string | 否 | 舞种 | `classical` |
| status | string | 否 | paid / canceled / refunding / refunded，缺省不含 canceled | `paid` |
| include_canceled | bool | 否 | true 时含已取消（对账留档） | `false` |
| source | string | 否 | miniapp / manual | — |
| paid_from / paid_to | string | 否 | 支付时间范围（ISO8601） | — |
| keyword | string | 否 | 姓名 / 订单号 / 微信支付单号 | — |
| export | bool | 否 | true 时按当前筛选导出 Excel 文件流 | `false` |

**请求示例**

```bash
curl "https://api.example.com/api/v1/admin/orders?event_id=12&status=paid&page=1&page_size=20" \
  -H "Authorization: Bearer <admin_token>"
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| list[].order_no | string | 商户订单号 |
| list[].transaction_id | string | 微信支付单号 |
| list[].event_name / level / dance_type | — | 赛事信息 |
| list[].player_name / id_card_masked | string | 选手姓名 / **脱敏证件号**（1101**********1234） |
| list[].amount / status / source | — | 金额（分）/ 状态 / 来源 |
| list[].paid_at / created_at | string | 支付时间 / 创建时间 |

```json
{
  "code": 0, "message": "success",
  "data": {
    "list": [{
      "order_no": "2027010109300012345678",
      "transaction_id": "4200001234202701011234567890",
      "event_name": "2027 上海甲级赛", "level": "U13", "dance_type": "classical",
      "player_name": "张小雨", "id_card_masked": "3101**********3456",
      "amount": 98000, "status": "paid", "source": "miniapp",
      "paid_at": "2027-01-01T11:40:12+08:00", "created_at": "2027-01-01T09:30:00+08:00"
    }],
    "total": 356, "page": 1, "page_size": 20
  }
}
```

**错误码**：无。

**注意事项**

- **证件号永不返回明文**，本接口连"明文查看"都不提供（系统仅存不可逆哈希，物理上无法还原）。
- 导出 `export=true` 时为 `text/csv` 文件流；cs 角色导出权限见 0.2 权限矩阵（不含证件号列，因本就没有）。
- 已取消订单标注"留档"，只读不可修改。

---

### 11.2 订单详情

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 订单详情 |
| 功能说明 | 报名/支付/资料/操作记录/备注 五区；资料含证件照缩略图、音频试听 |
| 请求方法 + URL | `GET /api/v1/admin/orders/{order_no}` |
| 鉴权 | 是（admin；cs 只读） |

**请求参数（Path）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| order_no | string | 是 | 商户订单号 | `"2027010109300012345678"` |

**请求示例**

```bash
curl "https://api.example.com/api/v1/admin/orders/2027010109300012345678" \
  -H "Authorization: Bearer <admin_token>"
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| order | object | 订单主信息（同列表 + `cancel_reason`、`registration_id`） |
| registration | object | 报名信息：赛事/组别/舞种/选手/机构/`contact_phone` |
| payments | array | 支付/退款流水：`type/prepay_id/transaction_id/refund_id/amount/status/notified_at` |
| refund_ticket | object \| null | 退款工单摘要 |
| materials | object | 资料清单（photo_url/audio_url/各文本项 + `updated_at` + `readonly`） |
| operation_logs | array | 状态流转：`action/from_status/to_status/operator/created_at` |
| remarks | array | 管理员备注（增删改，留痕） |

**失败响应**：40401。

**注意事项**：资料区超过 `material_check_at` 后 `readonly=true`；联系电话默认脱敏展示，明文查看走 11.3。

---

### 11.3 敏感信息明文查看（手机号）

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 明文查看联系电话 |
| 功能说明 | 手机号脱敏展示场景下的明文查看（证件号系统无此能力——只存哈希） |
| 请求方法 + URL | `POST /api/v1/admin/orders/{order_no}/reveal-phone` |
| 鉴权 | 是（**需单独权限位**，任意角色默认拒绝 → 40301） |

**请求示例**

```bash
curl -X POST "https://api.example.com/api/v1/admin/orders/2027010109300012345678/reveal-phone" \
  -H "Authorization: Bearer <admin_token>"
```

**响应参数**：`{"contact_phone": "13812345678"}`

**错误码**：40301（无权限）；40401。

**注意事项**：每次调用必须落 `admin_logs`（谁、何时、看了谁的），日志不可删除；限流 10 次/分钟/账号。

---

### 11.4 退款工单

**11.4.1 登记退款工单（后台代录）**

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 退款工单登记 |
| 功能说明 | 客服/管理员按用户诉求代录工单（pending）；订单须为已支付且未过报名截止 |
| 请求方法 + URL | `POST /api/v1/admin/refund-tickets` |
| 鉴权 | 是（admin / super_admin / cs） |

**请求参数（Body）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| order_no | string | 是 | 关联订单号 | `"2027010109300012345678"` |
| reason | string | 是 | 退款诉求（≤500 字） | `"孩子伤病需退赛"` |
| applicant | string | 否 | 登记来源（用户本人 / 客服代录），缺省「客服代录」 | `"客服代录"` |

**响应参数**：`{"ticket_id":900,"ticket_no":"TK202701...","status":"pending"}`

**失败响应**

| code | 说明 |
|---|---|
| 40001 | order_no/reason 缺失或超长 |
| 40401 | 订单不存在 |
| 40901 | 该订单已有进行中的退款工单 |
| 41015 | 订单非已支付，或已过报名截止时间 |

**11.4.2 工单列表**

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 退款工单列表 |
| 功能说明 | 客服登记 + 后台处理统一队列 |
| 请求方法 + URL | `GET /api/v1/admin/refund-tickets` |
| 鉴权 | 是（admin 全量；cs 可查看自己登记的） |

**请求参数（Query）**：`status`（pending/processing/approved/rejected/refunded/refund_failed）、`event_id`、`created_from/to`、`keyword`（工单号/订单号/姓名）、分页。

**请求示例**

```bash
curl "https://api.example.com/api/v1/admin/refund-tickets?status=pending" \
  -H "Authorization: Bearer <admin_token>"
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| list[].ticket_no | string | 工单号 |
| list[].order_no / event_name / player_name | — | 关联信息 |
| list[].reason | string | 诉求摘要 |
| list[].applicant | string | 用户本人 / 客服代录 |
| list[].status | string | pending / processing / approved / rejected / refunded / refund_failed |
| list[].handler_name | string \| null | 处理人 |
| list[].created_at / handled_at | string | 登记/处理时间 |

**错误码**：无。

**11.4.3 处理退款（同意）**

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 同意退款 |
| 功能说明 | 按退费政策自动带出可退金额（可调整），调微信支付退款 API 原路退回；成功 → 订单 refunded、报名 withdrawn、名额释放（候补队列提示） |
| 请求方法 + URL | `POST /api/v1/admin/refund-tickets/{ticket_id}/approve` |
| 鉴权 | 是（admin / super_admin；**cs 无权限**） |

**请求参数（Path + Body）**

| 参数名 | 位置 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|---|
| ticket_id | Path | int | 是 | 工单 ID | `900` |
| amount | Body | int | 是 | 实际退款金额（分）；与政策默认金额不一致时必须填 adjust_reason | `98000` |
| adjust_reason | Body | string | 条件 | 调整原因 | `"伤病凭诊断证明全额退"` |

**请求示例**

```bash
curl -X POST https://api.example.com/api/v1/admin/refund-tickets/900/approve \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" -d '{"amount":98000}'
```

**响应参数**：`{"ticket_no":"TK202701...","status":"approved","refund_id":"500000...","order_status":"refunding"}`。
退款接口受理后订单进入 `refunding`；退款成功回调到达后订单 `refunded`、报名 `withdrawn`、名额释放。

**失败响应**

| code | 说明 |
|---|---|
| 40001 | amount 缺失/≤0/超过实付金额；调整金额未填 adjust_reason |
| 40401 | 工单不存在 |
| 41013 | 工单状态不允许（非 pending/processing/refund_failed） |
| 41014 | 微信支付退款 API 调用失败（工单置 refund_failed，可重试） |
| 41015 | 订单当前状态不允许退款 |

**11.4.4 驳回退款**

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 驳回退款 |
| 功能说明 | 必填驳回原因 → 工单 rejected、订单回退 `paid` |
| 请求方法 + URL | `POST /api/v1/admin/refund-tickets/{ticket_id}/reject` |
| 鉴权 | 是（admin / super_admin） |

**请求参数（Body）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| reject_reason | string | 是 | 驳回原因（≤200 字） | `"已过退费期"` |

**请求示例**

```bash
curl -X POST https://api.example.com/api/v1/admin/refund-tickets/900/reject \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" -d '{"reject_reason":"已过退费期"}'
```

**响应参数**：`{"ticket_no":"TK202701...","status":"rejected","order_status":"paid"}`

**失败响应**：40001（驳回原因缺失/超长）；40401；41013（非待处理或订单状态不允许）。

**注意事项（退款域通用）**

- 退费政策阶段由订单赛事时间线自动判断（抽签前全额 / 抽签后个人原因扣 50% / 伤病凭证明全额），处理页提示；后端校验金额 ≤ 实付，金额与默认政策不一致时必须填调整原因。
- 退款回调按 `refund_id` 幂等；全程记操作日志（登记、同意/驳回、退款失败、回调结果）。
- 同意退款为资金操作：前端二次确认（金额+订单+选手三要素），后端以 `processing` 认领工单后在事务内推进状态。
- 截止报名后不可退款（reg_end_at 之后提交的工单直接提示不可处理）。
- 退款接口失败（41014）后工单为 `refund_failed`，可直接再次同意重试；微信侧按 `out_refund_no`（工单号）幂等。

---

### 11.5 人工报名（特殊通道）

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 新建人工报名 |
| 功能说明 | 无大陆身份证号选手（国际友人等）由客服登记后，管理员后台人工创建；不产生在线支付，订单直接置已支付 |
| 请求方法 + URL | `POST /api/v1/admin/manual-registrations` |
| 鉴权 | 是（admin / super_admin） |

**请求参数（Body）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| event_id | int | 是 | 赛事（限报名中/未开始） | `12` |
| group_id | int | 是 | 报名单元 ID | `88` |
| player_name | string | 是 | 选手姓名 | `"Emma Wilson"` |
| id_type | string | 是 | passport / hk_macau / other（缺省时按证件号格式自动判定，合法身份证为 id_card） | `"passport"` |
| id_card_no | string | 是 | 证件号（哈希存储；**不与大陆身份证判重库混用**） | `"P1234567"` |
| birth_date | string | 条件 | 出生日期（YYYY-MM-DD）；非身份证证件必填，身份证可由号码解析 | `"2017-05-01"` |
| gender | string | 否 | male / female（特殊证件可填） | `"female"` |
| nationality | string | 否 | 国籍/地区 | `"美国"` |
| institution | string | 否 | 机构 | `"..."` |
| contact_phone | string | 是 | 联系电话 | `"13812345678"` |
| charge_type | string | 是 | `offline_paid`（线下已收款）/ `free` | `"free"` |
| amount | int | 条件 | 线下收款金额（分）；free 时忽略/为 0 | `0` |
| cs_ticket_id | int | 否 | 关联客服工单 | `501` |
| remark | string | 否 | 特殊情况说明 | `"..."` |

**请求示例**

```bash
curl -X POST https://api.example.com/api/v1/admin/manual-registrations \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"event_id":12,"group_id":88,"player_name":"Emma Wilson","id_type":"passport","id_card_no":"P1234567","contact_phone":"13812345678","charge_type":"free","cs_ticket_id":501}'
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| registration_id | int | 报名单 ID |
| order_no | string | 订单号（status=paid，source=manual） |
| player_no | string | 选手编号（8 位，系统内唯一标识） |

**失败响应**

| code | 说明 |
|---|---|
| 40001 | 参数缺失/证件类型非法 |
| 41005 | 名额已满 |
| 41006 | 赛事不在可报名状态 |
| 40401 | 赛事/组别/工单不存在 |

**注意事项**

- 名额校验与小程序一致（事务内）；满员时可改报其他组别或先登记候补。
- 支付信息标注「后台人工录入」+ 操作人；记操作日志；资料上传、催收、成绩录入与小程序报名同等对待。
- 人工报名没有微信提交人，报名单/订单归属保留系统用户（`users.openid='__manual__'`，不可登录）；`free` 时订单金额固定 0。

---

## 12. 报名运营

### 12.1 候补管理列表

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 候补队列 |
| 功能说明 | 按 赛事+组别+舞种 分组，队列内按提交时间排序显示序号 |
| 请求方法 + URL | `GET /api/v1/admin/waitlists` |
| 鉴权 | 是（admin 全量；cs 只读） |

**请求参数（Query）**：`event_id`、`level`、`dance_type`、`status`（waiting/notified/converted/given_up/removed）、分页。

**请求示例**

```bash
curl "https://api.example.com/api/v1/admin/waitlists?event_id=12&status=waiting" \
  -H "Authorization: Bearer <admin_token>"
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| list[].waitlist_id | int | 候补 ID（操作用） |
| list[].event_id / event_name | — | 赛事 |
| list[].group_id / level / dance_type | — | 报名单元 |
| list[].position_no | int | 队列序号 |
| list[].name / id_card_masked | string | 姓名 / 脱敏证件号（无证件号特殊登记为 null） |
| list[].institution / phone | string | 机构 / **联系电话（明文，仅本接口与导出）** |
| list[].status | string | waiting / notified / converted / given_up / removed |
| list[].created_at / notified_at / handled_at | string | 提交/通知/终态处理时间 |
| released_quota | int | 筛选范围内报名单元当前可用名额合计（`quota - taken`，负值按 0）；按单一报名单元筛选时用于队列顶部"已释放 N 个名额"提示，未按赛事筛选时为 0 |

**错误码**：无。

### 12.2 标记已通知 / 移出队列 / 手动关联报名

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 候补队列操作 |
| 功能说明 | 通知留痕、移出队列、手动将候补记录与已支付订单关联（置 converted） |
| 请求方法 + URL | `POST /api/v1/admin/waitlists/{waitlist_id}/notify`；`POST /api/v1/admin/waitlists/{waitlist_id}/remove`；`POST /api/v1/admin/waitlists/{waitlist_id}/convert` |
| 鉴权 | 是（admin；**cs 无写权限**） |

**请求参数**

| 接口 | 位置 | 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|---|---|
| notify | Body | note | string | 是 | 通知结果：`confirmed_intent`（有意向）/ `no_answer` / `callback_later` | `"confirmed_intent"` |
| remove | Body | reason | string | 是 | 移出原因（放弃/联系不上/不符合条件） | `"多次联系不上"` |
| convert | Body | order_no | string | 是 | 用户完成支付后的订单号 | `"2027..."` |

**请求示例**

```bash
curl -X POST https://api.example.com/api/v1/admin/waitlists/301/notify \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" -d '{"note":"confirmed_intent"}'
```

**响应参数**：`{"waitlist_id": 301, "status": "notified"}`

**失败响应**：40401；40001（convert 时订单不存在或非本人/该单元）；41013（状态不允许，如已 removed）。

**注意事项**：移出队列仅改状态不删除；notify/remove/convert 均记操作日志且前端二次确认（remove）。notify 可对 waiting/notified 重复调用（刷新 notified_at，通知结果记审计日志）；convert 对已 converted 记录幂等返回；`write` 操作仅 admin / super_admin，cs 返回 40301。

---

### 12.3 资料催收名单

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 未传齐资料名单 |
| 功能说明 | 按缺失资料项筛选，支持导出催收名单、标记已联系 |
| 请求方法 + URL | `GET /api/v1/admin/materials/missing` |
| 鉴权 | 是（admin 全量；cs 只读） |

**请求参数（Query）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| event_id / level / dance_type | — | 否 | 常规筛选 | — |
| missing_item | string | 否 | photo / audio / program_name / contact_phone | `"audio"` |
| deadline_state | string | 否 | `open`（未截止）/ `closed`（已截止锁定） | `open` |
| export | bool | 否 | true 导出 Excel（含电话） | `false` |
| page / page_size | int | 否 | 分页 | — |

**请求示例**

```bash
curl "https://api.example.com/api/v1/admin/materials/missing?event_id=12&missing_item=audio" \
  -H "Authorization: Bearer <admin_token>"
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| list[].registration_id / order_no | — | 定位信息 |
| list[].event_name / level / dance_type | — | 赛事 |
| list[].player_name / contact_phone | string | 选手 / 联系电话 |
| list[].completed / total | int | 已传项 / 总项（8） |
| list[].missing_items | array | 缺失项明细 |
| list[].material_deadline | string | 资料截止时间 |
| list[].material_updated_at | string | 最后上传时间 |
| list[].contacted | bool | 是否已标记联系 |

**错误码**：无。

### 12.4 标记已联系

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 标记已联系 |
| 功能说明 | 催收留痕：记录催收时间与备注，避免重复催 |
| 请求方法 + URL | `POST /api/v1/admin/materials/missing/{registration_id}/contacted` |
| 鉴权 | 是（admin / cs 均可） |

**请求参数（Body）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| note | string | 否 | 催收备注 | `"已电话提醒，3 天内传"` |

**请求示例**

```bash
curl -X POST https://api.example.com/api/v1/admin/materials/missing/5001/contacted \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" -d '{"note":"已电话提醒"}'
```

**响应参数**：`{"registration_id": 5001, "contacted": true}`

**错误码**：40401。

**注意事项**：到达 `material_check_at` 后名单整体只读（`deadline_state=closed`），不可再标记。

---

### 12.5 导出选手名单

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 导出名单 |
| 功能说明 | 按赛事导出有效报名名单（签到表/对账），可选列 |
| 请求方法 + URL | `GET /api/v1/admin/events/{event_id}/registrations/export` |
| 鉴权 | 是（admin；**cs 不可导出**） |

**请求参数（Query）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| include_canceled | bool | 否 | 含已取消（对账） | `false` |
| columns | string | 否 | 逗号分隔：`order_no,player_name,id_card_masked,level,dance_type,institution,contact_phone,materials`，缺省全量 | — |

**请求示例**

```bash
curl "https://api.example.com/api/v1/admin/events/12/registrations/export" \
  -H "Authorization: Bearer <admin_token>" -o registrations.xlsx
```

**响应参数**：`application/vnd.openxmlformats-officedocument.spreadsheetml.sheet` 文件流。

**错误码**：40301（cs）；40401。

**注意事项**：证件号列仅脱敏；导出记操作日志（导出人、赛事、行数）。

---

### 12.6 媒体资源归档导出（后续扩展，2026-11-11 讨论新增）

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 赛事媒体归档导出 |
| 功能说明 | 按赛事打包导出选手证件照/音频，默认按选手分目录，可按选手筛选；用于赛事结束后的资料归档 |
| 请求方法 + URL | `GET /api/v1/admin/events/{event_id}/materials/archive` |
| 鉴权 | 是（admin；**cs 不可导出**） |

**请求参数（Query）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| player_nos | string | 否 | 逗号分隔的选手编号，只导出指定选手；缺省导出全赛事 | `a8K2mP9x,b7J3nQ2y` |
| include_canceled | bool | 否 | 含已取消报名（对账） | `false` |

**请求示例**

```bash
curl "https://api.example.com/api/v1/admin/events/12/materials/archive?player_nos=a8K2mP9x" \
  -H "Authorization: Bearer <admin_token>" -o materials.zip
```

**响应参数**：`application/zip` 文件流（`Content-Disposition: attachment`），目录结构：

```
2027上海甲级赛_12/
├── 张小雨_a8K2mP9x/
│   ├── 证件照.jpg
│   └── 春闺梦.mp3
└── 李小舞_b7J3nQ2y/
    ├── 证件照.jpg
    └── 春闺梦.mp3
```

**失败响应**：40301（cs）；40401（赛事不存在）。

**注意事项**

- **流式打包**：`archive/zip` + `io.Copy` 直接写响应流，禁止把全部文件读入内存（单音频可达 100MB）。
- 导出名由服务端生成（原文件名未入库）：`选手姓名_player_no/`，文件用资料类型/节目名命名；中文名设置 zip UTF-8 标志。
- 仅导出 `active` 报名（可选含已取消）；无资料时返回空 zip。
- 导出记操作日志（导出人、赛事、选手数、文件数）。
- 建议在赛事媒体清理（T33）前先执行本导出归档。

---

## 13. 用户与内容

### 13.1 用户列表（微信维度）

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 用户列表 |
| 功能说明 | 微信用户管理：昵称、脱敏手机号、报名次数、活跃 |
| 请求方法 + URL | `GET /api/v1/admin/users` |
| 鉴权 | 是（admin / cs 只读） |

**请求参数（Query）**：`keyword`（昵称/手机号）、`status`（active/blocked）、分页。

**请求示例**

```bash
curl "https://api.example.com/api/v1/admin/users?keyword=138" \
  -H "Authorization: Bearer <admin_token>"
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| list[].id | int | 用户 ID |
| list[].nickname / avatar_url | string | 微信信息 |
| list[].phone_masked | string | 脱敏手机号 |
| list[].registration_count | int | 报名次数 |
| list[].status | string | active / blocked |
| list[].created_at / last_login_at | string | 注册/最近活跃 |

**错误码**：无。

### 13.2 选手档案（证件号维度聚合）

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 选手档案 |
| 功能说明 | 以证件号哈希为唯一键聚合：历史报名（跨微信）、成绩与积分汇总、重复档案提示 |
| 请求方法 + URL | `GET /api/v1/admin/players`；`GET /api/v1/admin/players/{player_id}` |
| 鉴权 | 是（admin / cs 只读） |

**请求参数（Query，列表）**：`keyword`（姓名 / 选手编号 player_no）、分页。

**请求示例**

```bash
curl "https://api.example.com/api/v1/admin/players?keyword=%E5%BC%A0%E5%B0%8F%E9%9B%A8" \
  -H "Authorization: Bearer <admin_token>"
```

**响应参数（列表）**

| 参数名 | 类型 | 说明 |
|---|---|---|
| list[].player_id / player_no / name | — | 主标识（系统内所有引用走 player_no） |
| list[].id_type | string | id_card / passport |
| list[].id_card_masked | string | 脱敏（或 null） |
| list[].birth_date / gender | string | 出生日期 / 性别 |
| list[].merged_into | int \| null | 重复档案合并指向 |
| list[].registration_count / result_count | int | 历史报名/成绩数 |

**响应参数（详情）**：基本信息 + `registrations[]`（历史报名）+ `results_summary`（奖项汇总）+ `points_summary` + `duplicates[]`（同证件号哈希的重复档案，可手动合并）。

**错误码**：40401。

**注意事项**：合并档案为敏感操作（二次确认 + 留痕）；**不存在"明文查看证件号"功能**（哈希不可逆）。

### 13.3 客服工单

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 客服工单登记 / 流转 |
| 功能说明 | cs 登记（退款诉求/无证件号报名/候补咨询/其他），管理员处理流转 |
| 请求方法 + URL | `GET /api/v1/admin/cs-tickets`；`POST /api/v1/admin/cs-tickets`；`PUT /api/v1/admin/cs-tickets/{ticket_id}` |
| 鉴权 | 是（cs 可登记、查看自己登记的；admin 全量） |

**请求参数（POST Body，登记）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| type | string | 是 | refund / no_id_card / waitlist / other | `"refund"` |
| user_name / phone | string | 否 / 是 | 用户标识 | `"张小雨"` / `"13812345678"` |
| content | string | 是 | 诉求内容（≤1000 字） | `"要求退款"` |
| related_order_no | string | 否 | 关联订单号 | `"2027..."` |

**请求参数（PUT Body，流转）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| status | string | 是 | pending / processing / done | `"processing"` |
| remark | string | 否 | 跟进备注 | `"已转管理员退款"` |

**请求示例**

```bash
curl -X POST https://api.example.com/api/v1/admin/cs-tickets \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"type":"refund","user_name":"张小雨","phone":"13812345678","content":"孩子伤病需退赛","related_order_no":"2027010109300012345678"}'
```

**响应参数**：登记 → `{"ticket_id": 501, "status": "pending"}`；列表含 `type/status/created_by/related_order_no/created_at`。

**失败响应**：40001（type 非法）；40401（PUT 时工单不存在）；41013（流转状态不合法）。

**注意事项**：超 24 小时未处理的 pending 工单列表标红；done 的退款类工单应关联 refund_tickets。

### 13.4 内容管理

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 内容配置读写 |
| 功能说明 | 积分规则说明、赛事规则/FAQ、公众号二维码、公告 等键值内容 |
| 请求方法 + URL | `GET /api/v1/admin/contents/{key}`；`PUT /api/v1/admin/contents/{key}` |
| 鉴权 | 是（GET：admin/cs；PUT：admin） |

**请求参数（Path + Body）**

| 参数名 | 位置 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|---|
| key | Path | string | 是 | points_rules_desc / event_faq / official_account / announcement | `"points_rules_desc"` |
| value | Body | object | 是 | 内容 JSON（文案、图片 URL、上下线时间） | `{"text":"..."}` |

**请求示例**

```bash
curl -X PUT https://api.example.com/api/v1/admin/contents/announcement \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"value":{"text":"1 月 20 日抽签","online_at":"2027-01-15T00:00:00+08:00","offline_at":"2027-01-21T00:00:00+08:00"}}'
```

**响应参数**：`{"key": "announcement", "value": {...}, "updated_at": "..."}`

**错误码**：40401（key 未注册）；40301（cs 尝试写入）。

**注意事项**：小程序端对应展示接口从同一份 `settings` 表读取（见 6.1 config 的扩展字段）；修改记操作日志。

---

## 14. 系统管理

### 14.1 账号管理

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 管理员/客服账号管理 |
| 功能说明 | 创建、停用、重置密码（super_admin 专属） |
| 请求方法 + URL | `GET /api/v1/admin/accounts`；`POST /api/v1/admin/accounts`；`PUT /api/v1/admin/accounts/{admin_id}` |
| 鉴权 | 是（**super_admin 专属**，其余 40301） |

**请求参数（POST Body）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| username | string | 是 | 登录账号（唯一） | `"cs01"` |
| password | string | 是 | 初始密码（≥8 位） | `"********"` |
| role | string | 是 | super_admin / admin / cs | `"cs"` |
| name | string | 是 | 姓名 | `"客服小李"` |

**请求参数（PUT Body）**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|---|---|---|---|---|
| status | string | 否 | active / disabled | `"disabled"` |
| password | string | 否 | 重置密码 | `"********"` |
| role | string | 否 | 调整角色 | `"admin"` |

**请求示例**

```bash
curl -X POST https://api.example.com/api/v1/admin/accounts \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"username":"cs01","password":"Init@12345","role":"cs","name":"客服小李"}'
```

**响应参数**：`{"admin_id": 7, "username": "cs01", "role": "cs"}`

**失败响应**：40001（用户名重复/密码过弱）；40301（非 super_admin）。

**注意事项**：停用立即踢下线（token 黑名单或有效期 ≤5 分钟）；所有变更记操作日志。

### 14.2 操作日志

**接口概述**

| 项 | 内容 |
|---|---|
| 接口名称 | 操作日志查询 |
| 功能说明 | 全站关键操作留痕：登录、明文查看、退款、名额调整、关闭组别、发布赛事等 |
| 请求方法 + URL | `GET /api/v1/admin/logs` |
| 鉴权 | 是（super_admin；admin 可查） |

**请求参数（Query）**：`admin_id`、`action`（publish_event/reveal_phone/refund/quota_change/close_group/merge_player…）、`target_type`、`created_from/to`、分页。

**请求示例**

```bash
curl "https://api.example.com/api/v1/admin/logs?action=refund&page=1&page_size=50" \
  -H "Authorization: Bearer <admin_token>"
```

**响应参数**

| 参数名 | 类型 | 说明 |
|---|---|---|
| list[].admin_name | string | 操作人 |
| list[].action | string | 操作类型 |
| list[].target_type / target_id | string / int | 操作对象 |
| list[].detail | object | 操作详情 JSON（含前后值） |
| list[].created_at | string | 时间 |

```json
{
  "code": 0, "message": "success",
  "data": {
    "list": [{
      "admin_name": "张三", "action": "refund",
      "target_type": "refund_ticket", "target_id": 900,
      "detail": {"order_no": "2027010109300012345678", "amount": 98000, "before": "paid", "after": "refunded"},
      "created_at": "2027-01-10T14:22:00+08:00"
    }],
    "total": 128, "page": 1, "page_size": 50
  }
}
```

**错误码**：无。

**注意事项**：日志只增不删、不可修改；导出仅限 super_admin。

---

# 第三部分：附录

## A. 状态机对照

### A.1 订单状态机

```
[*] --支付成功--> paid
[*] --关闭/取消/支付失败--> canceled（立即释放名额，留档）
canceled --重新发起报名--> 新订单（原单仍留档）
paid --客服登记+管理员发起--> refunding
refunding --驳回--> paid
refunding --同意+微信退款成功--> refunded（报名 withdrawn，名额释放）
```

> **不存在"待支付"状态。**

### A.2 报名单状态机

```
paid（有效）-- 资料由字段是否为空实时计算完成度，不设子状态 --
paid -- 管理员驳回退款 --> active 保持
paid -- 退款完成 --> withdrawn（退赛）
```

### A.3 候补状态机

```
waiting --管理员电话通知--> notified --用户完成支付--> converted
waiting/notified --用户放弃/管理员移出--> given_up / removed
```

## B. 枚举速查

| 字段 | 取值 |
|---|---|
| event.level | grade_a（甲级赛）/ super（超级赛）/ final（年度总决赛） |
| dance_type | classical（古典舞）/ folk（民族民间舞） |
| level（组别） | U11 / U13 / U15 |
| order.status | paid / canceled / refunding / refunded |
| registration.status | active / canceled / withdrawn |
| registration.source | miniapp / manual |
| refund_ticket.status | pending / processing / approved / rejected / refund_failed |
| waitlist.status | waiting / notified / converted / given_up / removed |
| admin.role | super_admin / admin / cs |
| id_type | id_card / passport / hk_macau / other |
| charge_type（人工报名） | offline_paid / free |
| cs_ticket.type | refund / no_id_card / waitlist / other |

## C. 安全与合规清单

1. **证件号**：HMAC-SHA256（服务端密钥，环境变量注入）哈希存储，不可逆；接口、日志、数据库、导出物全程无明文。
2. **手机号**：明文存储、脱敏展示；后台明文查看需独立权限 + 留痕 + 限流。
3. **文件**：音频 mp3/≤100MB/≤4 分钟，照片 png|jpg/≤10MB；服务端 magic number 校验（不只信扩展名）；存储路径不可遍历。
4. **音频审核 H5**：仅姓名/节目名/音频三类信息，token 不可枚举、可失效、限流。
5. **支付**：回调验签 + transaction_id/refund_id 幂等；退款金额 ≤ 实付金额；全流水留档（含 raw_callback）对账。
6. **越权**：小程序端所有查询强制按 `user_id` 过滤；后台接口按 role 鉴权中间件统一拦截。

## D. 幂等与并发设计汇总

| 场景 | 机制 |
|---|---|
| 提交报名 | 客户端 `idempotency_key` + 服务端部分唯一索引 |
| 支付回调 | `transaction_id` 唯一约束，重复回调返回 SUCCESS |
| 退款回调 | `refund_id` 唯一约束 |
| 抢名额 | 事务内 `count(active) < quota` 校验（SQLite 单写者串行） |
| 资料覆盖上传 | 先传临时文件校验通过才替换，失败不动旧文件 |
| 候补提交 | 证件号哈希 × 报名单元 部分唯一索引 |

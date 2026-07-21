# K2Board 支付与套餐定价设计

> 状态：Phase 1 已落地；Phase 2a（BEpusdt 代码就绪，部署可后置）  
> 关联版本：自 v1.4.0 起；BEpusdt 网关自 v1.4.1  
> 原则：订单与履约不感知渠道细节；渠道插件可扩展

---

## 1. 背景与现状

| 维度 | 改造前 | 目标 |
|------|--------|------|
| 支付 / 订单 | 无 | 订单状态机 + 支付插件 |
| 套餐价格 | `plans` 无价格字段 | 方案 A：Plan 上 `price`（分）+ `currency` + `show_on_shop` |
| 用户获套餐 | 管理员手动绑 `plan_id` | 支付成功自动履约（复用权益写入逻辑） |
| 用户端 | 浏览规格 + 订阅链接 | 商城 → 下单 → 选支付 → 开通 |
| 扩展性 | — | `PaymentGateway` 注册表，加渠道不改订单核心 |

### 当前数据关系（权益侧，保持）

```
Group ←→ Node
  ↑
 Plan（时长/流量/限速/设备）
  ↑ plan_id
 User（traffic_* / expire_at / group_id）
```

支付只负责 **收钱 + 触发履约**，不改节点鉴权与订阅导出。

---

## 2. 目标用户路径

```
用户登录 → 套餐商城（规格+价格）→ 创建订单(pending)
  → 选择支付方式 → 收银台（跳转/二维码/Mock 确认）
  → 回调或查单 → paid → 履约写 User → 可用订阅链接
```

管理端：

```
配置支付方式 → 配置套餐价格/上架 → 订单列表（关单/补单/手动确认）
```

---

## 3. 领域模型

### 3.1 Plan 定价（方案 A，首版）

| 字段 | 类型 | 说明 |
|------|------|------|
| `price` | int64 | 标价，**最小货币单位（分）**，避免浮点 |
| `currency` | string | 默认 `CNY` |
| `show_on_shop` | bool | 是否在用户商城展示 |

一个 Plan = 一个可售 SKU（「VIP 月付」「VIP 年付」可拆两条）。  
中期可演进为 `plan_prices` 多周期表（方案 B），订单始终存**价格快照**。

### 3.2 Order

| 字段 | 说明 |
|------|------|
| `trade_no` | 面板唯一订单号 |
| `user_id` | 买家 |
| 快照 | plan_id/name、duration、limits、group_id、amount、currency |
| `status` | `pending` / `paid` / `cancelled` / `failed` |
| `payment_method` | 渠道 code |
| `paid_at` / `expired_at` / `fulfilled_at` | 时间 |
| `callback_no` | 渠道流水 |
| `meta` | JSON 扩展 |

**履约只认订单状态机**，前端不可信。

### 3.3 PaymentMethod

| 字段 | 说明 |
|------|------|
| `code` | 稳定插件 ID：`mock` / `alipay` / `wechat` / `stripe` / `usdt_*` |
| `name` | 展示名 |
| `enable` / `sort` | 开关与排序 |
| `config` | JSON，密钥等，**永不完整回显给用户端** |

---

## 4. 支付插件契约

```text
PaymentGateway
  Code() / Name()
  ConfigSchema()           # 管理端表单（后续）
  CreatePayment(order, cfg) → PaymentIntent
      type: redirect | qrcode | address | mock | completed
  HandleNotify(req, cfg) → NotifyResult   # 验签 + 金额
  QueryPayment(order, cfg) → QueryResult  # 补单
```

注册：`internal/payment/gateways/*` + `registry.Register`。

| 渠道 | 交互 | 备注 |
|------|------|------|
| Mock | 一键确认支付 | **Phase 1 默认**，打通闭环 |
| 支付宝 / 微信 | URL / 二维码 | Phase 2 |
| Stripe | Checkout / Webhook | Phase 2 |
| USDT | 地址+金额或第三方 | Phase 3，无官方回调 |

### 回调安全

- 公开：`POST /api/v1/payment/notify/:code`
- 幂等：`pending → paid` CAS，只履约一次
- 校验金额与币种
- 超时关单：创建时写 `expired_at`，访问/定时将过期 `pending → cancelled`

---

## 5. 履约规则（Phase 1 默认）

```text
FulfillOrder(order):
  1. CAS: status pending→paid（或已 paid 且未 fulfilled）
  2. 更新 user：
       plan_id, group_id（若 plan 有）
       traffic_limit / speed_limit / device_limit（来自订单快照）
       traffic_used = 0
       expire_at:
         - 若原 expire_at > now：expire_at + duration
         - 否则：now + duration
       enable = true
  3. fulfilled_at = now
  4. BumpConfigVersion()
```

后续可配置：续费是否清零流量、升级差价等。

---

## 6. API 规划

### 用户（token 与现有门户一致）

```text
GET    /user/plans                      # enable + show_on_shop
POST   /user/orders                     # { token, plan_id }
GET    /user/orders                     # { token }
GET    /user/orders/:trade_no           # { token }
POST   /user/orders/:trade_no/checkout  # { token, method }
POST   /user/orders/:trade_no/cancel    # { token } 用户取消 pending
POST   /user/orders/:trade_no/confirm-mock
GET    /user/payment-methods            # 无密钥
```

用户取消 remark=`closed by user`，迟到支付回调**不会**自动恢复开通（与 `auto-expired` 不同）。

### 下单防刷（v1.4.15+）

| 层 | 规则 |
|----|------|
| IP | 创建订单 10 次/分钟；checkout/cancel 30 次/分钟；读订单 60 次/分钟 |
| 用户 | 同时 pending ≤ 3；两次创建间隔 ≥ 5 秒；创建前批量过期超时 pending |

### 免费/试用套餐（双重防护）

对 `price <= 0` 的商城套餐：

1. **验证邮箱终身一次**：表 `free_plan_claims` 以规范化邮箱唯一索引；履约成功写入。防删号重注册再领。  
2. **订单审计**：该 `user_id` 已有 `paid` 且 `total_amount<=0` 的订单 → 拒绝。  
3. **有效期内不可再买免费**：用户已有 `plan_id` 或 `group_id` 且未过期（`expire_at=0` 视为永久）→ 拒绝（防流量用完再点免费清零）。  

付费套餐续费不受影响。

登录/发码另有 IP 限流（10 次/分钟）。

### 权限与越权（审计）

| 操作 | 归属校验 |
|------|----------|
| 列表订单 | `WHERE user_id = 当前用户` |
| 查单 / 取消 | `trade_no` + `order.UserID == token 用户`，失败统一 404 |
| checkout / mock 确认 | 服务层强制 `userID`，非本人等同不存在 |
| 管理关单 / 补单 | 仅 admin JWT |
| 支付回调 | 网关签名 + 金额校验；`closed by user` **不**自动开通（仅 admin 补单可恢复）；`auto-expired` 迟到到账可恢复 |
| return_url | 仅允许 `site_url` 同 host 或站内相对路径，防开放重定向 |

用户端认证依赖长期 **subscribe token**（非 JWT）；泄露等同账号权限，需防 XSS / 日志泄露 query token。

用户端订单 DTO（`toUserOrderView`）会剥离完整 `meta`，仅暴露：

- 套餐快照权益 `benefits`（时长/流量/速率/设备文案）
- `payment_url` / `pay_address` / 链上金额（pending 时用于重开收银台）
- `remaining_seconds`、`status_hint`、`cancel_hint`、`can_reopen_cashier`

### 公开回调（Phase 2+）

```text
POST   /payment/notify/:code
GET    /payment/return/:code
```

### 管理端

```text
CRUD   /admin/payment-methods
GET    /admin/orders
POST   /admin/orders/:id/close
POST   /admin/orders/:id/mark-paid      # 人工确认 / Mock 运维
# plans 扩展 price / currency / show_on_shop
```

---

## 7. 分期

| 阶段 | 内容 | 状态 |
|------|------|------|
| **Phase 1** | Plan 定价 + Order + Mock 网关 + 履约 + 用户商城/下单 + 管理订单与支付方式 | **已完成** |
| **Phase 2a** | BEpusdt 网关代码 + 公开 notify + redirect/回跳页 + site_url 拼装（**部署 BEpusdt 可后置**） | **代码已就绪** |
| **Phase 2a+** | 支付宝 OpenAPI（page/wap）+ RSA2 异步通知 | **代码已就绪** |
| **Phase 2a++** | 支付宝 query 对账兜底、trade.close、app_id/subject 校验 | **v1.4.16** |
| **Phase 2g** | 发卡收银台网关 `giftcard`（mock server 单测就绪；平台本体另仓） | **v1.4.17** |
| Phase 2b | 部署 BEpusdt / 配置支付宝密钥后小额实付验收 | 待运维 |
| **Phase 2e** | 彩虹易支付 `epay`（单商户 V1 MD5 submit.php + 查单） | **v1.4.18** |
| Phase 2c | Stripe / 微信 | 计划中 |
| Phase 3 | 优惠券、邮件通知 | 计划中 |
| Phase 4 | 多周期价表、余额、返利 | 计划中 |

### Phase 2a：BEpusdt（无实例可完成部分）

| 组件 | 路径 / 说明 |
|------|-------------|
| 网关 | `internal/payment/gateways/bepusdt.go`（code=`bepusdt`） |
| 签名 | `internal/payment/sign.go` + 单测（对齐官方文档示例） |
| 回调 | `POST /api/v1/payment/notify/:code` → 验签 → 金额校验 → 履约；应答 `ok` |
| 配置 | `payment_methods.config`：`base_url`, `api_token`, `trade_type`, `timeout`, `fiat` |
| 站点 | 系统设置 `site_url` 必须为面板公网 origin，用于 `notify_url` |
| 用户 | checkout `redirect` → 跳转 `payment_url`；`/#/user/order-result?trade_no=` 轮询状态 |

**上线 BEpusdt 后只需：**

1. 部署 [BEpusdt](https://github.com/v03413/bepusdt)  
2. 管理端 → 支付方式 → 添加 `bepusdt`，填写 `base_url` + `api_token`，启用  
3. 系统设置填写 `site_url`（如 `https://panel.example.com`）  
4. 确保 `{site_url}/api/v1/payment/notify/bepusdt` 公网可达  
5. 小额下单实付验收  

**无实例时可测：** Mock 支付；`go test ./internal/payment/...`；curl 伪造 notify（自签 MD5）。

### 支付宝（商家 OpenAPI）

| 项 | 说明 |
|----|------|
| 网关 code | `alipay` |
| 实现 | `internal/payment/gateways/alipay.go`（`smartwalle/alipay/v3`） |
| 电脑网站 | `product: "page"` → `alipay.trade.page.pay`（`FAST_INSTANT_TRADE_PAY`） |
| 手机网站 | `product: "wap"` → `alipay.trade.wap.pay`（`QUICK_WAP_WAY`） |
| 异步通知 | `POST /api/v1/payment/notify/alipay`，验签 + **app_id** 校验，应答 **`success`** |
| 主动查单 | `alipay.trade.query`：用户 `POST .../sync`、管理端「查单同步」、定时 2min 对账 |
| 关闭交易 | `alipay.trade.close`：用户取消 / 管理关单 / 本地超时 尽力调用 |
| 金额 | 订单分 → 元（两位小数）；回调 `total_amount` 校验 |
| subject | 过滤 `/ = &` 等禁用字符 |

配置 JSON 字段：

```json
{
  "app_id": "20xxx",
  "private_key": "-----BEGIN RSA PRIVATE KEY-----\\n...\\n-----END RSA PRIVATE KEY-----",
  "alipay_public_key": "-----BEGIN PUBLIC KEY-----\\n...\\n-----END PUBLIC KEY-----",
  "is_production": true,
  "product": "page",
  "timeout_express": "30m"
}
```

注意：`alipay_public_key` 必须是开放平台「支付宝公钥」，不是应用公钥。

### 彩虹易支付（epay，单商户）

| 项 | 说明 |
|----|------|
| 网关 code | `epay` |
| 实现 | `internal/payment/gateways/epay.go` |
| 创建 | 跳转 `{base_url}/submit.php`（GET 签名参数，V1 MD5） |
| 支付方式 | config `type` 空 → 易支付收银台自选；`alipay`/`wxpay` 等固定通道 |
| 异步通知 | `GET|POST /api/v1/payment/notify/epay`，验签 + pid 校验，应答 **`success`** |
| 主动查单 | `{base_url}/api.php?act=order`：用户 sync / 管理补单 / 定时对账 |
| 金额 | 订单分 → 元两位小数；回调 `money` 校验 |

配置 JSON：

```json
{
  "base_url": "https://pay.example.com",
  "pid": "1001",
  "key": "<商户密钥>",
  "type": "",
  "product_name": "数字商品"
}
```

> 方案 A：仅支持**一套**易支付商户（`code` 唯一）。多通道需后续实例化改造。

### Giftcard / 发卡收银台（支付宝中转）

> 完整规格：`docs/GIFTCARD_PAYMENT_PLATFORM.md`  
> 支付宝商户密钥落在 **发卡平台**；K2 只持有平台 `app_id` + `api_secret`。

| 项 | 说明 |
|----|------|
| 网关 code | `giftcard` |
| 实现 | `internal/payment/gateways/giftcard.go`（`Gateway` + `Closer`） |
| 创建 | `POST {base_url}/api/v1/orders`，Header 签名 `SignMD5({app_id,timestamp,nonce,body_sha256})` |
| 金额 | 商户 API / 回调全程 **整数分**（与 bepusdt 的「元」不同，勿混用） |
| 异步通知 | `POST /api/v1/payment/notify/giftcard`，body 内 `SignMD5` + **app_id 必须匹配**；ACK **`ok`** |
| 查单 | `GET {base_url}/api/v1/orders/{trade_no}`；`40401` → `Paid:false` |
| 关单 | `POST .../close`；`0/40401/40901/40902` 软成功 |
| 已付重下 | Create 返回 `giftcard: already_paid:` 时 Checkout 自动 `ReconcileFromGateway` |

配置 JSON：

```json
{
  "base_url": "https://pay.example.com",
  "app_id": "k2-main",
  "api_secret": "<platform merchant secret>",
  "timeout_sec": 20,
  "product_name_template": "数字商品",
  "sign_version": "v1"
}
```

> **subject 脱敏**：默认 `product_name_template` 为 **`数字商品`**（不再默认 `{plan_name}`），避免套餐名/敏感词出现在发卡台与支付宝账单。  
> 可选占位：`{trade_tail}`（订单号后 6 位）、`{trade_no}`；仍可用 `{plan_name}` 但会经敏感词过滤。

本地/CI：`go test ./internal/payment/gateways/ -run Giftcard`（内置 httptest mock 平台）。

**平台本体（独立目录）**：`/Users/han/Documents/giftcard-platform`  
- 本地：`go run ./cmd/server -config configs/config.local.yaml`（默认 `mock_pay`）  
- 联调：K2 `payment_methods` 配置 `base_url=http://127.0.0.1:8088`，`app_id=k2-main`，`api_secret=test_secret`

### Epusdt / GM Pay（GMWalletApp/epusdt）

| 项 | 说明 |
|----|------|
| 网关 code | `epusdt` |
| 实现 | `internal/payment/gateways/epusdt.go` |
| 创建 | `POST {base_url}/payments/gmpay/v1/order/create-transaction` |
| 回调 | `POST {site_url}/api/v1/payment/notify/epusdt`，验签后返回 `ok` |
| 鉴权 | `pid` + `secret_key`（MD5，同 wiki/API.md） |

配置示例（推荐多链：不传 token/network，收银台自选）：

```json
{
  "base_url": "https://upay.example.com",
  "pid": "1000",
  "secret_key": "<从支付端 API Keys 查看>",
  "currency": "cny",
  "rewrite_payment_host": true
}
```

仅允许单一链时再锁定：

```json
{
  "base_url": "https://upay.example.com",
  "pid": "1000",
  "secret_key": "<secret>",
  "token": "usdt",
  "network": "tron",
  "currency": "cny"
}
```

#### 父子订单与回调（支付端设计，非面板 bug）

- `token`+`network` **同传**：创建即锁定该链（状态 1）。用户在收银台切到另一条链时，支付端最多生成 **1 条子订单**；子单标注「子订单不参与回调，交由父订单回调」。
- `token`+`network` **同缺**：创建状态 **4** 占位单；用户**首次**选链会**原地补全父订单**，不产生子单；之后再切链才可能产生唯一子单。
- 支付成功后 **只有父订单** 向商户 `notify_url` 发异步通知（`order_id` = 面板 `trade_no`）。子单已付款也由父单回调。
- 后台列表里「未回调/失败」在未付款时为正常；已付款仍失败则检查面板 `site_url` 公网可达与 notify 返回 `ok`。

#### 面板关单 vs 支付端状态

- 面板 `CloseOrder` / 超时取消 **只改本地面板订单**。
- GMPay **公开 API 无商户关单接口**；支付端关单仅管理后台 JWT：`POST /admin/api/v1/orders/{trade_id}/close`，或等待支付端过期任务将待付款改为状态 3。
- 因此：用户/管理员在面板取消后，upay 仍可能短暂显示「待付款」，属预期，不是回调故障。

支付端前置条件：

1. 管理后台登录后 **API Keys** 获取 `pid` / `secret_key`（登录密码 ≠ secret）
2. **Wallets** 至少添加一条收款地址（如 TRON / Solana），否则 `supported_assets` 为空无法下单
3. 建议配置对外 `app_uri` 为公网域名，否则 `payment_url` 可能返回内网 IP；面板侧 `rewrite_payment_host` 会用 `base_url` 改写 Host
4. 面板 **site_url** 必须为公网可达地址，供异步 notify

---

## 8. 风险

| 风险 | 缓解 |
|------|------|
| 重复开通 | 订单 CAS + `fulfilled_at` |
| 价格篡改 | 服务端按 plan 现价建单，快照落库 |
| 回调丢失 | 轮询订单 + 管理 mark-paid / 后续 Query；迟到回调可恢复 auto-expired |
| 密钥泄露 | config 仅管理员；用户 API 脱敏 |
| 金额浮点 | 全程整数「分」；notify 严格等额校验 |
| Mock 白嫖 | mock 默认禁用；无公开 notify；confirm-mock 校验 method=mock |

### 审计修复纪要（v1.4.4）

- ConfirmMock 必须 `payment_method=mock` 且 mock 启用；禁用 mock 公开 notify
- `expireIfNeeded` CAS，禁止覆盖已 paid；notify 路径不自动取消订单
- paid 未履约可恢复；auto-expired 可由回调/管理员补单
- 履约始终写入 `group_id`（含 0，避免降级残留 VIP 组）
- 计划 `price >= 0` 校验

---

## 9. 架构总览

```text
Admin: Plan 定价 · PaymentMethod · 订单补单
              │
User: 商城 → OrderService → GatewayRegistry (mock/…)
              │ paid
              ▼
         Fulfill → users.plan_id / expire / limits
              │
         BumpConfigVersion → 节点侧收敛
```

---

## 10. Phase 1 验收清单

- [x] 设计文档（本文件）
- [x] Plan 可配置价格与上架
- [x] 用户可见上架套餐与价格
- [x] 创建订单 → Mock 支付 → 用户权益生效
- [x] 管理端订单列表、关单、手动确认支付
- [x] Mock 支付方式默认可启用
- [x] 过期 pending 订单不可再支付

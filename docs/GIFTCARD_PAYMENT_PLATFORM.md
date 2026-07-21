# 支付宝迁移到自建发卡/收银台平台 — 完美对接 K2Board

| 字段 | 值 |
|------|-----|
| **文档标题** | 自建 GiftCard/Cashier 支付中转平台设计与实现规格 |
| **作者** | （待填） |
| **日期** | 2026-07-12 |
| **状态** | Approved（设计评审通过，R4） |
| **受众** | AI 实现者 + 人类工程师 |
| **关联仓库** | K2Board：`/Users/han/Documents/k2board_anytls`；平台：`/Users/han/Documents/giftcard-platform`（**mock 版已实现**） |
| **本地路径** | `docs/GIFTCARD_PAYMENT_PLATFORM.md` |
| **网关 code** | **`giftcard`**（K2 `payment_methods.code` 与公开 notify 路径均用此标识） |

---

## Overview

当前 K2Board 通过 `internal/payment/gateways/alipay.go` **直连**支付宝 OpenAPI（page/wap + notify + query + close）。业务上希望 **支付宝商户身份与密钥从面板剥离**，改为：

1. 用户在 K2 下单并 checkout → 跳转到 **自建发卡/收银台平台**；
2. 平台展示订单并引导支付宝付款，支付宝异步回调 **只打到平台**；
3. 平台验签成功后，**签名回调 K2** 的 `POST /api/v1/payment/notify/giftcard`，触发既有 `ProcessNotify` → `MarkPaidAndFulfill` 履约链路；
4. 可选：K2 `QueryPayment` / `ClosePayment` 反向调用平台，完整对齐现有对账与关单语义。

本方案 **不魔改独角数卡**，而是自建极简平台，领域模型与 API **专为 K2 `PaymentGateway` 契约设计**，金额全程 **分（int64）**，签名算法复用 `payment.SignMD5` 心智，使 K2 侧变更收敛为一个新 Gateway 插件。

> ### ⚠ 金额单位硬约束（禁止抄 bepusdt 元路径）
>
> | 路径 | `amount` / `paid_amount` 单位 |
> |------|------------------------------|
> | **giftcard 商户 API / notify JSON** | **整数分（cents）**，例：`2888` = ¥28.88 |
> | bepusdt / epusdt 上游 API | **元（major）** 字符串/数字，经 `FormatFiatFromCents` / `FiatToCents` 转换 |
> | K2 `order.TotalAmount` / `NotifyResult.PaidAmount` | **分** |
>
> **禁止**在 giftcard Create/Notify 中发送 `28.88` 或 `"28.88"`。单测必须锁定：`amount=2888` → 履约成功；若误传 `28`/`28.88` 则金额校验失败。  
> 仅在平台调用 **支付宝 OpenAPI** 时把分格式化为元字符串（`FormatFiatFromCents`）。

**本期只交付设计/实现规格文档，不实现平台代码。**

---

## Background & Motivation

### 当前状态（已验证代码）

K2 支付体系已完整落地（见 `docs/PAYMENT_DESIGN.md`）：

| 组件 | 路径 | 职责 |
|------|------|------|
| Gateway 接口 | `internal/payment/gateway.go` | `CreatePayment` / `HandleNotify` / `QueryPayment` |
| Closer（可选） | `internal/payment/closer.go` | `ClosePayment`；`payment.AsCloser` |
| CreateOptions | `internal/payment/options.go` | `NotifyURL`, `RedirectURL` |
| 签名工具 | `internal/payment/sign.go` | `SignMD5`、`FormatFiatFromCents`、`FiatToCents` |
| 支付宝直连 | `internal/payment/gateways/alipay.go` | OpenAPI page/wap + notify + query + close |
| 中转参考 | `bepusdt.go` / `epusdt.go` | 外部收银台 HTTP API + MD5 回调（**金额单位是元，勿抄**） |
| 订单服务 | `internal/services/order_svc.go` | Checkout / ProcessNotify / MarkPaidAndFulfill / ReconcileFromGateway / tryCloseRemote |
| 公开回调 | `internal/handlers/payment/notify.go` | `POST /api/v1/payment/notify/:code`，ACK `ok`/`success` |
| 用户补单 | `POST /user/orders/:trade_no/sync` | `handlers/user/order.go` → `ReconcileFromGateway` |

**订单真相源在 K2**：`models.Order`（`internal/models/order.go`）以 `trade_no` 唯一，金额 `total_amount` 为分，状态 `pending/paid/cancelled/failed`。默认订单 TTL：`defaultOrderTTL = 30 * time.Minute`（`order_svc.go`）。

**履约核心不变**：`OrderService.MarkPaidAndFulfill` 做 CAS + 写用户权益；`closed by user` **永不**自动履约（仅 admin 人工 mark-paid，`method=="manual"` 或 `callbackNo` 前缀 `admin`）；`auto-expired` 迟到到账可恢复。

### 痛点

1. **支付宝密钥与面板耦合**：面板被攻破或配置误导出即泄露商户私钥；多站点难共享同一支付宝商户身份。
2. **直连限制扩展**：若未来加「卡密展示 / 独立收银台域名」需在 K2 内堆业务。
3. **通用发卡系统摩擦大**：独角/Dujiao-Next 商品预建、金额单位、回调语义、关单/补单与 K2 状态机不完全对齐，魔改成本高于自建薄层。

### 目标形态

```
用户 ──► K2 商城/下单 ──checkout(giftcard)──► GiftCard 平台收银台
                                                    │
                                              支付宝 page/wap
                                                    │
                                          支付宝 notify ──► 平台
                                                    │
                                          签名 notify ──► K2 giftcard Gateway
                                                    │
                                          MarkPaidAndFulfill ──► 用户权益
```

---

## Goals & Non-Goals

### Goals

1. **K2 侧新增 `giftcard` Gateway**，完整实现 `Gateway` + `Closer`，行为对齐支付宝直连在订单层的语义（redirect 收银台、异步履约、查单补单、关单）。
2. **发卡平台** 持有支付宝商户身份；对 K2 暴露稳定 REST 商户 API + 异步回调。
3. **金额严格等额（分）**；签名可验证；notify 幂等。
4. **用户取消（closed by user）** 与 **超时 auto-expired** 策略与 K2 现网一致，并在平台侧定义 orphan 到账处理。
5. 文档粒度足以让 AI **按 checklist 从零实现两个仓库**，无需再猜接口。

### Non-Goals

| 项 | 说明 |
|----|------|
| 本期不实现平台/网关代码 | 仅规格文档 |
| 退款 API | `POST .../refund` 明确不做；与现 K2 支付宝决策一致 |
| 支付宝对账单下载 | 不做 |
| 完整「库存发卡」 | v1 无库存扣减；可选展示虚拟凭证，**履约不依赖卡密** |
| 微信/Stripe | 平台 v1 只接支付宝；架构预留 channel 字段即可 |
| 多租户 SaaS / 多支付宝子账户 | v1 **单部署共用一套支付宝密钥**；`merchants` 只隔离 K2 面板（secret/allowlist），不是多支付宝商户 |
| 修改 K2 订单状态机 / 履约规则 | 只加插件，不改 `MarkPaidAndFulfill` 核心语义 |
| 强制删除 `alipay` 代码 | 可保留代码但生产禁用；切换通过 payment_methods 开关 |
| 远程 HSM 仅代签 / 易支付等聚合 | 无独立收银台与 orphan 域模型，不满足产品目标（见 Alternatives） |

---

## Proposed Design

### A. 系统边界与命名

#### 两个系统

| 系统 | 角色 | 持有密钥 | 订单真相 |
|------|------|----------|----------|
| **K2Board（面板）** | 套餐、用户、订单状态机、权益履约 | **不再持有**支付宝 RSA 密钥；仅持有对接平台的 `api_secret` | **K2 `orders` 是业务真相源** |
| **GiftCard Platform（发卡/收银台）** | 虚拟商品展示、拉起支付宝、收款、回调 K2 | **全局一套**支付宝 `app_id`/私钥/公钥；各 Merchant 的 `api_secret` | 平台支付订单以 `out_trade_no`(=K2 `trade_no`) 关联 |

#### 命名决策

| 名称 | 选定值 | 说明 |
|------|--------|------|
| K2 method / gateway code | **`giftcard`** | notify 路径固定 |
| 平台仓库名建议 | `giftcard-platform` | 独立 Git 仓库 |
| 平台内部订单号 | `platform_trade_no` | 如 `GC` + snowflake/ULID；**支付宝 out_trade_no** |
| K2 侧商户订单号 | `out_trade_no` | **恒等于** `order.TradeNo` |

#### 架构总览

```mermaid
flowchart TB
  subgraph UserSide["用户侧"]
    U[用户浏览器]
  end

  subgraph K2["K2Board 面板"]
    Shop[商城 / 下单]
    OS[OrderService]
    GW[giftcard Gateway]
    Fulfill[MarkPaidAndFulfill]
    NotifyH["POST /api/v1/payment/notify/giftcard"]
    Shop --> OS
    OS -->|CreatePayment| GW
    NotifyH --> OS
    OS --> Fulfill
  end

  subgraph GCP["GiftCard Platform"]
    API[商户 REST API]
    Cashier[收银台页 /c/:token]
    PayOrd[平台订单服务]
    AliGW[支付宝适配器 全局一套密钥]
    NWorker[Notify 重试 Worker]
    ExpW[Expire Worker]
    API --> PayOrd
    Cashier --> PayOrd
    PayOrd --> AliGW
    AliGW --> PayOrd
    PayOrd --> NWorker
    ExpW --> PayOrd
  end

  subgraph Ali["支付宝"]
    AliPay[page/wap 收银台]
    AliN[异步 notify]
  end

  U --> Shop
  GW -->|HTTPS 签名| API
  API -->|cashier_url| GW
  GW -->|redirect| U
  U --> Cashier
  Cashier --> AliPay
  AliN --> AliGW
  NWorker -->|签名 POST notify_url| NotifyH
  OS -->|Query/Close 签名| API
```

#### 支付成功时序

```mermaid
sequenceDiagram
  participant U as 用户
  participant K2 as K2 OrderService
  participant GW as giftcard Gateway
  participant P as GiftCard Platform
  participant A as 支付宝

  U->>K2: POST checkout method=giftcard
  K2->>K2: opts.NotifyURL = site_url/api/v1/payment/notify/giftcard
  K2->>K2: opts.RedirectURL = order-result 或 same-host return
  K2->>GW: CreatePayment(order, cfg, opts)
  GW->>P: POST /api/v1/orders (Header 签名 body_sha256)
  P->>P: 幂等创建 pending 订单, 生成 cashier_token
  P-->>GW: { cashier_url, platform_trade_no, expire_at }
  GW-->>K2: PaymentIntent{type:redirect, URL:cashier_url}
  K2-->>U: 跳转 cashier_url
  U->>P: GET /c/{token}
  U->>P: 点击「支付宝支付」
  P->>A: trade.page/wap.pay (timeout_express=剩余TTL, notify→平台)
  A-->>U: 付款
  A->>P: 异步 notify (RSA2)
  P->>P: 验签, 等额, CAS pending→paid
  P->>K2: POST notify_url (body 内 SignMD5)
  K2->>GW: HandleNotify (验签 + app_id 匹配)
  GW-->>K2: NotifyResult{TradeNo, PaidAmount分, Success}
  K2->>K2: 金额校验 + MarkPaidAndFulfill
  K2-->>P: HTTP 200 body "ok"
  A-->>U: return_url → 平台 return → 302 到 K2 RedirectURL
```

#### 丢 notify / 对账补单时序

```mermaid
sequenceDiagram
  participant U as 用户/定时任务
  participant K2 as OrderService.ReconcileFromGateway
  participant GW as giftcard.QueryPayment
  participant P as Platform GET /orders/{out_trade_no}

  Note over P: 支付宝已付, 平台 paid, 但回调 K2 失败重试中
  U->>K2: POST /user/orders/:trade_no/sync 或管理补单 / ReconcileStalePending
  K2->>GW: QueryPayment(order)
  GW->>P: GET 查单 (body_sha256=sha256空)
  P-->>GW: status=paid, paid_amount分, alipay_trade_no
  GW-->>K2: QueryResult{Paid:true, PaidAmount}
  K2->>K2: MarkPaidAndFulfill (幂等)
```

#### 用户取消时序

```mermaid
sequenceDiagram
  participant U as 用户
  participant K2 as OrderService.CancelByUser
  participant GW as giftcard.ClosePayment
  participant P as Platform
  participant A as 支付宝

  U->>K2: POST cancel
  K2->>K2: CAS pending→cancelled, remark="closed by user"
  K2->>GW: tryCloseRemote → ClosePayment
  GW->>P: POST /orders/{out_trade_no}/close
  P->>P: status→closed (若仍 pending)
  P->>A: alipay.trade.close (best-effort)
  Note over P,K2: 若 close 前用户已付成功竞态：
  P->>P: status=paid_orphan (= need_manual)
  P->>K2: 仍发送 success notify status=paid
  K2->>K2: ProcessNotify ACK, 因 closed by user 不履约
  Note over P: 运维人工退款或 admin mark-paid
```

---

### B. K2 侧变更 — `giftcard` Gateway

#### B.1 新增文件与注册

| 项 | 值 |
|----|-----|
| 新文件 | `internal/payment/gateways/giftcard.go` |
| 单测 | `internal/payment/gateways/giftcard_test.go`（签名、HandleNotify、金额分、app_id 校验、40901） |
| 注册 | `func init() { payment.Register(GiftCardGateway{}) }` |
| 空白导入 | 已有 `_ "K2board/internal/payment/gateways"`（`OrderService` 包 init / admin payment handler），**无需改 import** |
| 订单核心 | **原则上不改** `OrderService`；Checkout 拼的 `NotifyURL` 自动变成 `.../notify/giftcard` |
| 可选微小改动 | 见 B.4「已支付 40901」：可在 `Checkout` 对 giftcard 错误触发 `ReconcileFromGateway`，或仅在 Gateway 返回可识别错误文案由用户点 sync |
| notify ACK | `Notify` handler 对非 `alipay` 默认回 `ok`，**giftcard 使用 `ok`** |

#### B.2 接口实现清单

```go
// internal/payment/gateways/giftcard.go
type GiftCardGateway struct {
    HTTPClient *http.Client // 可注入，默认 timeout_sec
}

func (GiftCardGateway) Code() string { return "giftcard" }
func (GiftCardGateway) Name() string { return "发卡收银台" }

// payment.Gateway: CreatePayment / HandleNotify / QueryPayment
// payment.Closer:  ClosePayment
```

对齐接口（现有，勿改）：

```go
// package payment — Gateway / Closer / CreateOptions / PaymentIntent / NotifyResult / QueryResult
CreatePayment(ctx, order *models.Order, cfg string, opts CreateOptions) (*PaymentIntent, error)
HandleNotify(ctx, headers map[string]string, body []byte, cfg string) (*NotifyResult, error)
QueryPayment(ctx, order *models.Order, cfg string) (*QueryResult, error)
ClosePayment(ctx, order *models.Order, cfg string) error
```

#### B.3 Config JSON Schema（`payment_methods.config`）

```json
{
  "base_url": "https://pay.example.com",
  "app_id": "k2-main",
  "api_secret": "<与平台 Merchant.api_secret 一致>",
  "timeout_sec": 20,
  "product_name_template": "{plan_name}",
  "sign_version": "v1"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `base_url` | string | 是 | 平台 origin，无尾 `/` |
| `app_id` | string | 是 | 平台侧 Merchant 标识 |
| `api_secret` | string | 是 | 签名密钥；**永不**对用户端回显（`ListEnabledMethods` 已 strip config） |
| `timeout_sec` | int | 否 | HTTP 客户端超时，默认 20 |
| `product_name_template` | string | 否 | subject 模板，默认 `{plan_name}`；可含 `{trade_no}` |
| `sign_version` | string | 否 | 固定 `v1` |

#### B.4 CreatePayment 行为

**输入来源**（`OrderService.Checkout` 设置 `CreateOptions`）：

- `opts.NotifyURL` = `{site_url}/api/v1/payment/notify/giftcard`
- `opts.RedirectURL` = 用户 return（same-host）或默认 `{site_url}/#/user/order-result?trade_no={trade_no}`
- `order.TradeNo` → `out_trade_no`
- `order.TotalAmount` → `amount`（**分**，JSON **整数** number，例 `2888`）
- `order.Currency` → `currency`（默认 CNY）
- `order.PlanName` → `subject`（template 后 **最多 128 rune**，与 K2 `PlanName` `size:128` 对齐）
- `expire_at` = `order.ExpiredAt.Unix()`（K2 默认 TTL 30min）
- 可选 `user_ref`：`u{user_id}`，**禁止**邮箱明文

**请求**：

```http
POST {base_url}/api/v1/orders
Content-Type: application/json
X-App-Id: {app_id}
X-Timestamp: {unix_seconds}
X-Nonce: {≥16 字节随机 hex}
X-Signature: {sign}   # 见 B.8，唯一算法
```

Body 示例（**amount 为分**）：

```json
{
  "out_trade_no": "K2T20260712XXXX",
  "amount": 2888,
  "currency": "CNY",
  "subject": "VIP 月付",
  "notify_url": "https://panel.example.com/api/v1/payment/notify/giftcard",
  "return_url": "https://panel.example.com/#/user/order-result?trade_no=K2T20260712XXXX",
  "expire_at": 1720771200,
  "user_ref": "u42"
}
```

**成功响应 → PaymentIntent**（平台 HTTP 200 且 `code==0`）：

```go
&payment.PaymentIntent{
    Type:     payment.IntentRedirect,
    URL:      data.CashierURL,
    TradeNo:  order.TradeNo,
    Amount:   order.TotalAmount,
    Currency: order.Currency,
    Message:  "正在跳转发卡收银台…",
    ExpireAt: data.ExpireAt,
    Extra: map[string]any{
        "gateway":           "giftcard",
        "platform_trade_no": data.PlatformTradeNo,
        "cashier_url":       data.CashierURL,
        "cashier_token":     data.CashierToken,
    },
}
```

Checkout 会把 `intent.Extra` 写入 `order.meta`（现有逻辑），供客服排查。

##### 错误映射与「已支付 40901」补单路径（冻结 — 唯一叙事）

Gateway 解析响应：**始终尝试读 JSON body 的 `code`/`message`/`data`**（含 HTTP 4xx/409）。成功当且仅当 **`code == 0`**（见 C.5）。

| 平台响应 | Gateway 行为 |
|----------|--------------|
| HTTP 2xx + `code=0` | 返回 `(*PaymentIntent, nil)` |
| `code=40901` 且 `data.status` ∈ {`paid`,`paid_orphan`} | **仅**返回下方固定 error（intent 恒为 `nil`） |
| `code=40902` / 其他 `code!=0` | `fmt.Errorf("giftcard: %s", message)` |
| 网络错误 / 非 JSON | `fmt.Errorf("giftcard: ...")` |

**Gateway 固定错误串（唯一，禁止其他措辞）**：

```go
// CreatePayment — 40901 + paid/paid_orphan
return nil, fmt.Errorf("giftcard: already_paid: out_trade_no=%s", order.TradeNo)
```

- Gateway **不得**返回 `IntentCompleted`（无 DB，无法 MarkPaid，对用户无履约意义）。
- 匹配前缀仅允许：`giftcard: already_paid:`（`strings.Contains(err.Error(), "giftcard: already_paid:")`）。

**可选 Checkout 自动补单**（`OrderService.Checkout` 签名为 `(*PaymentIntent, *models.Order, error)`）：

```go
intent, err := gw.CreatePayment(ctx, order, pm.Config, opts)
if err != nil {
    if methodCode == "giftcard" && strings.Contains(err.Error(), "giftcard: already_paid:") {
        ord, rerr := s.ReconcileFromGateway(ctx, order.TradeNo, userID)
        if rerr != nil {
            return nil, order, rerr
        }
        if ord.Status != models.OrderPaid {
            // canQuery 未付、notify 仍在途等
            return nil, ord, fmt.Errorf("giftcard: already_paid but order not paid yet; try sync later")
        }
        return &payment.PaymentIntent{
            Type:     payment.IntentCompleted,
            TradeNo:  ord.TradeNo,
            Amount:   ord.TotalAmount,
            Currency: ord.Currency,
            Message:  "支付已同步",
        }, ord, nil
    }
    return nil, order, err
}
```

**无 Checkout 改动时的用户恢复**：`POST /user/orders/:trade_no/sync`；管理端 `POST /admin/orders/:id/sync`；定时 `ReconcileStalePending`。

**PR 要求**：PR-7 实现固定 error 前缀；同 PR 或 follow-up 可选落地上述 Checkout 片段。测试覆盖「平台已 paid、K2 pending、再次 checkout」。

#### B.5 HandleNotify 行为

平台回调 body（JSON，金额为 **分**）：

```json
{
  "app_id": "k2-main",
  "out_trade_no": "K2T20260712XXXX",
  "platform_trade_no": "GC123456",
  "amount": 2888,
  "paid_amount": 2888,
  "currency": "CNY",
  "status": "paid",
  "alipay_trade_no": "2026071222001....",
  "paid_at": 1720770000,
  "timestamp": 1720770001,
  "nonce": "a1b2c3d4e5f6...",
  "signature": "..."
}
```

**解析强制规则**：

```go
type giftcardNotify struct {
    AppID           string `json:"app_id"`
    OutTradeNo      string `json:"out_trade_no"`
    PlatformTradeNo string `json:"platform_trade_no"`
    Amount          int64  `json:"amount"`       // 分；禁止 float64 / json.Number 松散解析
    PaidAmount      int64  `json:"paid_amount"`  // 分
    Currency        string `json:"currency"`
    Status          string `json:"status"`
    AlipayTradeNo   string `json:"alipay_trade_no"`
    PaidAt          int64  `json:"paid_at"`
    Timestamp       int64  `json:"timestamp"`
    Nonce           string `json:"nonce"`
    Signature       string `json:"signature"`
}
```

- 使用 **typed struct + `int64`**；若 JSON 出现浮点金额（如 `2888.5`），`encoding/json` 解码 int64 会失败 → 返回 error（拒绝）。
- **禁止** `map[string]any` 再 `fmt.Sprint` 浮点（避免 `2.888e+03`）。

**处理顺序**：

1. JSON 解码到 struct；
2. 用各字段 **FormatInt** 组装 sign map，`SignMD5` 验签（见 B.8 Notify 段）；
3. **`app_id` 必须与 `cfg.AppID` 完全相等（大小写敏感）**；不匹配 → `fmt.Errorf("giftcard: app_id mismatch")` → HTTP 400，不 ACK；
4. 映射 `NotifyResult`。

```go
success := n.Status == "paid"
return &payment.NotifyResult{
    TradeNo:    n.OutTradeNo,
    PaidAmount: n.PaidAmount, // 分
    Success:    success,
    CallbackNo: firstNonEmpty(n.AlipayTradeNo, n.PlatformTradeNo),
    Raw:        string(body),
}, nil
```

金额二次校验在 `ProcessNotify`：`result.PaidAmount != order.TotalAmount` → 不履约、HTTP 400。

#### B.6 QueryPayment 行为

```http
GET {base_url}/api/v1/orders/{out_trade_no}
X-App-Id / X-Timestamp / X-Nonce / X-Signature
# 无 body；body_sha256 = sha256("")，见 B.8 矩阵
```

**HTTP/`code` → Gateway 映射（冻结）**：

| 平台响应 | QueryPayment 返回 |
|----------|-------------------|
| `code==0` + body data | 见下方 status 映射；`error=nil` |
| `code==40401` / HTTP 404 订单不存在 | `&QueryResult{Paid: false}, nil`（Reconcile 视为未付，不当作用户硬错误） |
| `code==40101` / `40102` / `40301` | `error`（鉴权/商户问题） |
| `code==50000` / 5xx / 网络 / 非 JSON | `error` |

```go
// code==0 时：
paid := data.Status == "paid" || data.Status == "paid_orphan"
// ReconcileFromGateway.canQuery 仅 pending | auto-expired；
// closed-by-user 不会因 Query Paid=true 而开通。
return &payment.QueryResult{
    Paid:       paid,
    PaidAmount: data.PaidAmount, // 分；平台必须返回
    CallbackNo: firstNonEmpty(data.AlipayTradeNo, data.PlatformTradeNo),
}, nil
```

#### B.7 ClosePayment 行为

```http
POST {base_url}/api/v1/orders/{out_trade_no}/close
Body: {"reason":"k2_cancel"}
# body_sha256 = sha256(exact raw body bytes)
```

**软成功（返回 `nil`，不打成硬错误）** — 与支付宝 `ACQ.TRADE_NOT_EXIST` 对齐：

| 条件 | ClosePayment 返回 |
|------|-------------------|
| `code==0` | `nil` |
| `code==40401` 订单不存在 | `nil` |
| `code==40902` 已 closed/expired | `nil` |
| `code==40901` 已 paid / paid_orphan | `nil`（无法关闭已收款；本地 cancel 仍成立） |
| 网络错误 / `code==50000` / `401xx` | `error`（`tryCloseRemote` 仅 Warn） |

#### B.8 商户请求签名 — **唯一冻结算法 v1**

> **删除所有备选方案。** 实现者只实现本节。  
> MD5-as-MAC 为兼容 K2 现有 `SignMD5` / bepusdt 心智的 **legacy 实用选择**，非现代密码学理想；密钥熵与 HTTPS 是前提。未来 `sign_version=v2` 可迁 HMAC-SHA256，v1 范围不实现。

##### B.8.1 商户 API（K2 → 平台）Header 签名

**参与签名的且仅有的字段**（四个）：

| 键 | 来源 |
|----|------|
| `app_id` | Header `X-App-Id` / config |
| `timestamp` | Header `X-Timestamp`，十进制秒字符串 |
| `nonce` | Header `X-Nonce` |
| `body_sha256` | `lower_hex(sha256(exact_http_body_bytes))` |

```text
body_sha256 = lower_hex(sha256(exact HTTP body bytes as transmitted))
  — 使用「实际发送/接收的原始字节」，禁止 re-marshal / canonical JSON / 改空白

sign_params = {
  "app_id":      app_id,
  "timestamp":   timestamp_string,
  "nonce":       nonce,
  "body_sha256": body_sha256,
}
X-Signature = SignMD5(sign_params, api_secret)
```

`SignMD5`（与 `internal/payment/sign.go` 一致）：

```text
keys = sort(keys where value != "" and key != "signature")
raw  = join(key+"="+value, "&") + api_secret
return lower_hex(md5(raw_utf8))
```

##### B.8.2 方法 × body_sha256 矩阵（冻结）

| Method | Path 示例 | Body | `body_sha256` 输入 |
|--------|-----------|------|-------------------|
| `POST` | `/api/v1/orders` | raw JSON bytes | `sha256(raw_body)` |
| `GET` | `/api/v1/orders/{out_trade_no}` | **空（零长度）** | `sha256("")` = `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| `POST` | `/api/v1/orders/{out_trade_no}/close` | raw JSON bytes（如 `{"reason":"k2_cancel"}`） | `sha256(raw_body)` |

**明确不在签名内**：

- URL path（含 `out_trade_no`）
- Query string
- 除 `X-App-Id` 外的其他 Header

路径完整性依赖 **TLS** + 服务端按 `X-App-Id` 做 **订单归属校验**（只能操作本 merchant 的 `out_trade_no`）。`reason` 等 body 字段 **仅**通过 body 字节哈希覆盖。

##### B.8.3 防重放

- `|now_unix - timestamp| <= 300`（平台 `security.sign_skew_sec`）
- `(app_id, nonce)` 在 **10 分钟**内唯一；重复 → `40102`
- 默认存储：表 `api_nonces`（见 C.3）；也可用 Redis `SETNX key EX 600`

##### B.8.4 异步 Notify 签名（平台 → K2，body 内）

与商户 API **不同**：无 Header 签名；**body 内全部非空标量**（除 `signature`）参与 `SignMD5`。

```text
sign_params[k] = string value for every non-empty field except signature
  amount, paid_amount, paid_at, timestamp → strconv.FormatInt(v, 10)  // 如 "2888"
signature = SignMD5(sign_params, api_secret)
```

平台发送与 K2 校验必须使用 **同一套 FormatInt 规则**（先解码到 int64，再 FormatInt，再 Sign）。

黄金向量见 **附录 C**。

---

### C. 发卡平台本体（`giftcard-platform` 新仓库）

#### C.1 技术选型（默认推荐）

| 层 | 推荐 | 可替换 | 理由 |
|----|------|--------|------|
| 后端 | **Go 1.22+ + Gin** | Node/Fastify | 与 K2 同栈；迁移 `alipay.go` + `smartwalle/alipay/v3` |
| 前端收银台 | 服务端渲染 HTML + 少量 JS | Vite 单页 | 极简 |
| DB | **SQLite**（小规模）/ **MySQL 8**（生产） | PostgreSQL | GORM |
| 部署 | 单二进制 + Caddy/Nginx TLS | Docker Compose | |
| 支付宝 SDK | `github.com/smartwalle/alipay/v3` | | 从 K2 迁移 |

目录建议：

```text
giftcard-platform/
  cmd/server/main.go
  internal/config/
  internal/models/          # merchants, orders, api_nonces
  internal/sign/            # SignMD5 + merchant auth middleware
  internal/httpserver/
  internal/service/order.go
  internal/service/notify_k2.go
  internal/service/expire.go
  internal/alipay/
  internal/admin/
  web/cashier/
  migrations/
  configs/config.example.yaml
  README.md
  docs/API.md
```

#### C.2 配置项清单

```yaml
# config.yaml
server:
  listen: ":8088"
  public_base_url: "https://pay.example.com"
  admin_token: "<random>"

db:
  driver: mysql   # 或 sqlite
  dsn: "user:pass@tcp(127.0.0.1:3306)/giftcard?parseTime=true&charset=utf8mb4"

# v1：全局唯一一套支付宝商户身份（整部署共用）
alipay:
  app_id: "20xxx"
  private_key: |
    -----BEGIN RSA PRIVATE KEY-----
    ...
  alipay_public_key: |
    -----BEGIN PUBLIC KEY-----
    ...
  is_production: true
  product: "page"          # page | wap | auto(UA)
  timeout_express: "30m"   # 仅支付宝 trade 参数上限；不缩短平台 orders.expire_at
  mock_pay: false          # true 时收银台显示「模拟支付」；is_production=true 时强制 false

notify_worker:
  max_attempts: 12
  base_backoff_sec: 5
  poll_interval_sec: 3

expire_worker:
  interval_sec: 15         # 扫描周期
  batch_size: 100

security:
  sign_skew_sec: 300
  rate_limit_rps: 20
  https_only: true
  # 生产必须：notify_url DNS 解析后拒绝私网/链路本地/云 metadata IP
  ssrf_block_private_ip: true
```

环境变量前缀：`GC_`（如 `GC_ALIPAY_APP_ID`、`GC_ALIPAY_MOCK_PAY`）。

**约束**：`alipay.is_production == true` ⇒ `alipay.mock_pay` 强制 `false`（启动时校验失败则拒绝启动）。

#### C.3 领域模型与表结构

##### merchants

| 列 | 类型 | 说明 |
|----|------|------|
| id | PK | |
| app_id | varchar(64) UNIQUE | K2 config.app_id |
| name | varchar(128) | |
| api_secret | varchar(128) | 当前签名密钥 |
| api_secret_prev | varchar(128) NULL | **轮转窗口**：非空时新旧 secret 均接受签名（建议 24h） |
| enable | bool | |
| notify_url_host_allowlist | text | JSON 数组 exact host，如 `["panel.example.com"]` |
| created_at / updated_at | datetime | |

> **v1 语义**：多个 Merchant = 多个 K2 面板（不同 secret / allowlist / 订单命名空间），**共享**进程内同一套 `alipay.*` 密钥。不是多支付宝子商户。

##### orders（平台支付单）

| 列 | 类型 | 说明 |
|----|------|------|
| id | PK | |
| platform_trade_no | varchar(64) UNIQUE | 平台号；支付宝 out_trade_no |
| out_trade_no | varchar(64) | K2 trade_no |
| app_id | varchar(64) | 商户 |
| **UNIQUE(app_id, out_trade_no)** | | 幂等键 |
| amount | bigint | **分** |
| currency | varchar(8) | CNY |
| subject | varchar(256) | 存储上限 256；入站已截断 128 |
| status | varchar(32) | `pending` / `paid` / `closed` / `expired` / `paid_orphan` |
| cashier_token | varchar(64) UNIQUE | |
| notify_url | varchar(512) | |
| return_url | varchar(512) | |
| user_ref | varchar(64) | |
| alipay_trade_no | varchar(64) | |
| paid_amount | bigint | 分 |
| paid_at | datetime null | |
| expire_at | datetime | |
| close_reason | varchar(64) | |
| notify_status | varchar(16) | `none` / `pending` / `success` / `failed` |
| notify_attempts | int | |
| notify_next_at | datetime null | |
| notify_last_error | varchar(512) | |
| meta | text | JSON |
| created_at / updated_at | | |

**`need_manual` 定义（无独立列）**：

```text
need_manual := (status == "paid_orphan")
```

管理端订单列表提供 filter `status=paid_orphan`（文案「需人工」）。不新增 `need_manual` bool 列，避免双写。

##### api_nonces（防重放，默认实现）

| 列 | 类型 | 说明 |
|----|------|------|
| app_id | varchar(64) | |
| nonce | varchar(64) | |
| created_at | datetime | |
| **PRIMARY KEY (app_id, nonce)** | | |

- 插入冲突 → `40102`
- **清理任务**：每分钟删除 `created_at < now() - 10 minutes`  
- 可选 Redis：`SETNX giftcard:nonce:{app_id}:{nonce} 1 EX 600`

##### admin_audit_logs（可选）

| 列 | 说明 |
|----|------|
| id, actor, action, target, detail, created_at | 重推、关单等 |

##### 状态机

```text
pending ──pay success──► paid ──notify ok──► notify_status=success
   │                      │
   │                      └──（少见）与 close 竞态见下
   ├── close API / admin ──► closed
   └── expire worker ──► expired  (+ best-effort alipay.trade.close)

paid / paid_orphan：禁止再拉起支付宝
closed / expired：拒绝支付；若支付宝仍到账 → paid_orphan (=need_manual) + 仍回调 K2
```

#### C.3.1 Expire Worker（冻结规格）

| 项 | 规格 |
|----|------|
| 进程 | 与 API 同进程后台 goroutine（或独立 cmd，v1 同进程即可） |
| 周期 | `expire_worker.interval_sec` 默认 **15s** |
| 查询 | `status='pending' AND expire_at < now()` LIMIT `batch_size`（默认 100） |
| 并发 | 逐条处理或小并发；单行 `UPDATE` 或 CAS `WHERE status='pending'` |
| 本地状态 | CAS → `expired`，`close_reason='auto_expired'` |
| 支付宝 | **best-effort** `alipay.trade.close(OutTradeNo=platform_trade_no)`；失败只打日志 |
| 收银台 | `status!=pending` 或 `now>expire_at` → 禁用支付按钮；点击 pay 返回业务错误。K2 在订单仍 pending 时可 Create revive（C.3.1） |
| 与 K2 TTL | 平台 `orders.expire_at` **跟随 K2 请求的 `expire_at`**（见下）；**不得**用 `alipay.timeout_express` 缩短平台订单寿命 |

**创建订单时 `expire_at` 计算（冻结 — 平台寿命只跟 K2）**：

```text
# Preferred / 唯一合法公式
req_exp = body.expire_at
if req_exp <= now: reject 40001  // 或视为非法参数
hard_max = now + 24h              // 平台防滥用上限（与支付宝 timeout_express 无关）
orders.expire_at = min(req_exp, hard_max)

# 禁止：
# orders.expire_at = min(req_exp, now + alipay.timeout_express)  // 会短于 K2 TTL → 40902 卡死
```

**幂等 Create 与过期行（配合上式，避免 stranding）**：

| 已有行状态 | Create 行为 |
|------------|-------------|
| `pending` 且 `now < expire_at` | 幂等返回同一单（校验 amount/currency，见下） |
| `pending` 且已过本地 expire | expire worker 会标 `expired`；见下行 |
| `expired` 且请求带 **未来** `expire_at` | **允许 revive**（见 **C.3.1.1**）；`code=0`，**同一 `out_trade_no`（K2）** |
| `expired` 且请求 expire_at 仍过期 | `40902` |
| `closed` | `40902`（用户主动关单不 revive；K2 侧多为 cancelled） |
| `paid` / `paid_orphan` | `40901` |

##### C.3.1.1 Revive 语义（冻结 — Preferred）

> **为何 revive**：K2 `Checkout` 在 `pending` 上复用同一 `trade_no`，无法为「平台先过期」另开新单。平台寿命对齐 K2 后 revive 仅作时钟偏差/worker 延迟的兜底；**主路径不应依赖 revive**。

Expire worker 可能已对 **旧** `platform_trade_no` 调用 `alipay.trade.close`。因此 revive **禁止**复用支付宝侧 out_trade_no。

**Revive 原子步骤（单事务 / 行锁）**：

```text
1. 校验 body.amount == orders.amount 且 currency 一致
   - 不一致 → 40001 或 40901（禁止改价复活）
2. old_ptn = orders.platform_trade_no
3. 生成 **新** platform_trade_no（ULID/snowflake，全局 UNIQUE）
4. 生成 **新** cashier_token（≥128-bit CSPRNG）
5. CAS: status expired → pending
6. 写入：
   - platform_trade_no = 新值          // 后续支付宝 out_trade_no
   - cashier_token = 新值
   - expire_at = 按 C.3.1 公式
   - close_reason = ""
   - alipay_trade_no = NULL/""
   - paid_amount = 0, paid_at = NULL
   - 清空 pay_url / 临时支付宝字段（若存在于 meta）
   - notify_status 保持 none（未付不回调）
7. meta JSON 追加运维轨迹（不丢历史）：
   {
     "platform_trade_no_history": [ ...旧列表, old_ptn ],
     "revived_at": <unix>,
     "latest_platform_trade_no": <新值>
   }
   // orders.platform_trade_no 列始终是「当前」支付宝号；
   // K2 order.meta（CreatePayment Extra）在下次 checkout 成功后会覆盖为新 platform_trade_no
8. 返回 data 含新 platform_trade_no、cashier_url、expire_at
```

**幂等 pending 返回**（未过期再 Create）：同样校验 `amount`/`currency` 与行一致；不一致 → `40001`（或 `40901`），不得静默返回旧收银链接。

**支付宝 notify 路由与迟到 SUCCESS（冻结 — 资金安全优先）**：

查找订单（v1）：

1. 按 **当前** `orders.platform_trade_no = notify.out_trade_no`；
2. 未命中则在 **`meta.platform_trade_no_history` 含该 id** 的订单中反查（v1 可全表/JSON 扫描；规模大后再加侧表 `platform_trade_no_index`）。

对命中订单，按 `trade_status` 分支：

| trade_status | 订单当前状态 | 动作 |
|--------------|--------------|------|
| SUCCESS / FINISHED，金额与 `orders.amount` 等额 | `pending`（含 revive 后当前号已是 **新** PT、notify 却是 **历史** PT） | **受理一次付款**：CAS `pending→paid`；写 `alipay_trade_no`；`meta.latest_paid_platform_trade_no = notify 的 out_trade_no`（可为历史号）；入队 K2 notify（`out_trade_no`=K2）；收银台 **禁止再付**；对 **当前** 未付的 `platform_trade_no` best-effort `alipay.trade.close`（防双扣） |
| SUCCESS / FINISHED，等额 | `expired` / `closed` | CAS → `paid_orphan`；同样写 alipay 字段与 `latest_paid_platform_trade_no`；**仍**回调 K2（closed-by-user 侧 ACK 不履约） |
| SUCCESS / FINISHED | 已 `paid` / `paid_orphan` | **幂等** ACK `success`（可校验金额；不重复履约/不重复入队若 notify 已 success） |
| 非成功（关闭/等待等噪音）且 id 仅在 history | 任意 | ACK `success`，**不改状态**，Warn（关闭类迟到噪音） |
| SUCCESS 但金额不等 | 任意 | 不改状态，**不** ACK success（或记 failed + 告警），防错账 |

**禁止**（R3 错误句已废止）：在订单仍为 `pending` 时，把历史 PT 上的 SUCCESS 当成「close 噪音忽略」。

**双扣缓解**：

- 一旦任意 PT（当前或历史）SUCCESS 被受理 → 订单 `paid`，公开 status / 收银台拒绝支付；Create 幂等返回 `40901`。
- 受理历史 SUCCESS 后：best-effort close **当前** `platform_trade_no` 上可能已创建的支付宝单。
- Query/Reconcile：订单 `paid` 后 `Paid=true`，不依赖 PT2 是否付过。

**拉起支付宝时 `timeout_express`（仅影响支付宝侧窗口，不改平台 expire_at）**：

```text
remaining = orders.expire_at - now
if remaining <= 0: 拒绝支付（本地已过期）
# 支付宝侧收紧：避免平台未过期但支付宝单挂太久
timeout_express = min(config.alipay.timeout_express, format_minutes(remaining))
# 至少 1m；config 默认 "30m" 只是支付宝参数上限，不是平台订单 TTL
```

**不变式**：

```text
platform orders.expire_at  ≈  K2 order.ExpiredAt   （来自 Create body，可差时钟）
alipay.timeout_express     ≤  remaining(orders.expire_at)   （每次 pay）
```

#### C.4 支付宝集成（从 K2 迁移）

参考：`internal/payment/gateways/alipay.go`（`sanitizeAlipaySubject`、`parseAlipayAmountToCents`、Close 软错误码）。

| 能力 | 支付宝 API | 平台用途 |
|------|------------|----------|
| 电脑网站 | `alipay.trade.page.pay` | product=page |
| 手机网站 | `alipay.trade.wap.pay` | product=wap 或 UA |
| 异步通知 | 平台 `POST /alipay/notify` | RSA2 + app_id |
| 查单 | `alipay.trade.query` | 管理同步 / 对账 |
| 关单 | `alipay.trade.close` | close API / expire worker |

支付宝 `out_trade_no` = **当前列 `orders.platform_trade_no`**（revive 后为新号，见 C.3.1.1）。  
金额对支付宝：分 → `FormatFiatFromCents` 元字符串。  
Subject 管道见 **C.4.1**。  
K2 `order.meta` / Create 响应的 `platform_trade_no` 以 **最新一次 Create/revive** 为准，供客服对照；历史号在平台 `meta.platform_trade_no_history`。

**退款 / 对账单：Non-Goal。**

支付宝 notify 成功路径（与 **C.3.1.1 路由表** 一致）：

1. 验签、app_id；
2. **按当前 `platform_trade_no` 或 `meta.platform_trade_no_history` 反查** 订单；
3. 若 `trade_status ∈ {SUCCESS, FINISHED}`：total_amount → 分，与 `orders.amount` 严格相等，否则拒绝；
4. CAS：`pending→paid`（**含** notify 落在历史 PT、当前列已是 revive 新号）；`closed|expired` → `paid_orphan`；已 paid → 幂等；
5. 记录 `alipay_trade_no`、`meta.latest_paid_platform_trade_no`；必要时 best-effort close 其它未付 PT；
6. 入队 K2 notify（K2 `out_trade_no` 不变）；
7. 应答支付宝 **`success`**。

非 SUCCESS 的历史号通知：ACK + 不改状态（噪音）。

##### C.4.1 Subject 长度管道（统一）

| 阶段 | 上限 |
|------|------|
| K2 `PlanName` / template 输出 | **128 rune**（Gateway Create 截断） |
| 平台 DB `subject` | varchar **256** |
| 送支付宝 sanitize | **256 rune**（对齐 `sanitizeAlipaySubject`） |

#### C.5 对 K2 的商户 REST API

##### C.5.1 通用约定（HTTP + code 冻结）

- Base：`https://pay.example.com`，前缀 `/api/v1`
- 鉴权：Header `X-App-Id`, `X-Timestamp`, `X-Nonce`, `X-Signature`（**仅 B.8.1**）
- Content-Type：`application/json; charset=utf-8`
- 响应 envelope：

```json
{ "code": 0, "message": "ok", "data": { } }
```

| code | 含义 | 推荐 HTTP |
|------|------|-----------|
| 0 | 成功 | 200 |
| 40001 | 参数错误 | 400 |
| 40101 | 签名失败 | 401 |
| 40102 | 时间窗/nonce 重放 | 401 |
| 40301 | 商户禁用 | 403 |
| 40401 | 订单不存在 | 404 |
| 40901 | 状态冲突（已支付等） | 409 |
| 40902 | 已关闭/过期/不可支付 | 409 |
| 50000 | 内部错误 | 500 |

**冻结规则（K2 Gateway 与平台必须一致）**：

1. **业务成败只看 body.`code`**：`code==0` ⇔ 成功；Gateway **在 HTTP 409 时也必须读 body**。
2. HTTP 用于中间件/缓存语义；**禁止**只发 HTTP 200 且 `code!=0` 却当作成功（若历史实现混用，Gateway 仍以 `code` 为准）。
3. 鉴权失败：**HTTP 401** + `code` 40101/40102。

##### C.5.2 `POST /api/v1/orders` — 创建

**校验**：amount>0 整数分；currency 默认 CNY；notify_url 生产 https 且 host ∈ allowlist；生产解析 DNS 后拒绝私网（`ssrf_block_private_ip`）；out_trade_no `^[A-Za-z0-9_-]+$` ≤64。

**幂等与 HTTP/code**：

| 已有状态 | HTTP | code | data |
|----------|------|------|------|
| 无记录 | 200 | 0 | 新订单 pending + cashier_url |
| pending 未过期 | 200 | 0 | **同一订单**（幂等返回） |
| pending 已过期 / 行已 `expired`，且 body.expire_at 为未来 | 200 | 0 | **revive**（C.3.1.1）：新 `platform_trade_no` + 新 `cashier_token` + 刷新 `expire_at`；清空 alipay 字段 |
| pending 已过期 / `expired`，body.expire_at 仍过期 | 409 | 40902 | 不可支付 |
| 任意幂等/revive 但 amount/currency 与行不一致 | 400 | 40001 | 禁止改价 |
| paid / paid_orphan | 409 | **40901** | `status`, `paid_amount`, `platform_trade_no`, `alipay_trade_no` |
| closed | 409 | 40902 | 不 revive |

成功 data：

```json
{
  "out_trade_no": "K2T...",
  "platform_trade_no": "GC...",
  "cashier_token": "tok_...",
  "cashier_url": "https://pay.example.com/c/tok_...",
  "amount": 2888,
  "currency": "CNY",
  "status": "pending",
  "expire_at": 1720771200
}
```

##### C.5.3 `GET /api/v1/orders/{out_trade_no}` — 查单

| 结果 | HTTP | code |
|------|------|------|
| 找到（本 app_id） | 200 | 0 |
| 不存在 | 404 | 40401 |

data 含：`status`, `amount`, `paid_amount`（分）, `alipay_trade_no`, `notify_status`, `expire_at`, `cashier_url` 等。

##### C.5.4 `POST /api/v1/orders/{out_trade_no}/close` — 关单

Body：`{"reason":"k2_cancel"}`（字段纳入 body_sha256）。

| 当前状态 | 行为 | HTTP | code | K2 ClosePayment |
|----------|------|------|------|-----------------|
| pending | → closed + alipay close BE | 200 | 0 | nil |
| closed / expired | 幂等 | 200 | 0 | nil |
| 不存在 | — | 404 | 40401 | **nil（软成功）** |
| paid / paid_orphan | 不改状态 | 409 | 40901 | **nil（软成功）** |

##### C.5.5 Refund — Non-Goal

`POST .../refund` → HTTP 501 或 400 + `code=40001`，`message: "refund not supported in v1"`。

#### C.6 对 K2 的异步回调

**URL**：订单 `notify_url`。  
**方法**：`POST` JSON。  
**字段**：同 B.5；v1 仅在收款成功路径发送，`status` 固定 `"paid"`（含 orphan 到账）。

**签名**：B.8.4。

##### C.6.1 成功 / 失败矩阵（冻结）

| K2 响应 | 平台动作 |
|---------|----------|
| HTTP 2xx 且 body trim 大小写 ∈ {`ok`,`success`} | **停止重试**，`notify_status=success` |
| HTTP 200 但 body 非 ok/success | 视为失败，重试 |
| HTTP 400 + `amount mismatch` | **重试** + 告警（金额配置错误极少自愈，但按重试策略耗尽后 failed） |
| HTTP 400 其他（坏签等，正常不应发生） | 重试；耗尽 → failed + manual |
| HTTP 404 `order not found` | 重试有限次后 **停止**，`notify_status=failed`，运维检查 out_trade_no |
| HTTP 5xx / 超时 / 网络 | **重试** |
| v1 不发送 `status!=paid` 的回调 | 若误发，K2 `Success=false` 仍 200 ok — 平台不应依赖此路径 |

> giftcard ACK 为 **`ok`**（`notify.go` default 分支；仅 `alipay` 特殊返回 `success`）。平台接受两者以防改动。

**重试参数**：max 12；间隔 5s…3600s（同前版）；管理端 renotify 重置 attempts。

#### C.7 收银台 UX

| 项 | 规格 |
|----|------|
| URL | `{public_base_url}/c/{cashier_token}` |
| **cashier_token** | **≥ 128-bit CSPRNG**（如 16+ 字节 `crypto/rand` → hex/base64url）；禁止可预测自增 ID |
| 展示 | subject、金额（元两位 **仅展示**）、倒计时、状态 |
| 主按钮 | 支付宝；`mock_pay` 时额外「模拟支付成功」 |
| 过期/关闭 | 禁用支付 |
| return | `/c/{token}/return` → 处理中 → **302 仅至建单时保存的 `orders.return_url`**；**忽略** query/body 中任何 `redirect`/`return_url`/`url` 覆盖（防开放重定向） |
| 轮询 | `GET /public/orders/status?token=` 每 3s |
| 卡密 | 可选展示；履约不依赖 |

#### C.8 管理端最小能力

`/admin/api/v1`，`Authorization: Bearer {admin_token}`。

| 能力 | 方法 |
|------|------|
| 订单列表（含 `status=paid_orphan` 筛选） | GET /orders |
| 详情 | GET /orders/:id |
| 手动重推 notify | POST /orders/:id/renotify |
| 关单 | POST /orders/:id/close |
| 支付宝查单同步 | POST /orders/:id/sync-alipay |
| 商户 CRUD（含 secret 轮转） | /merchants |
| 支付宝配置（脱敏） | /settings/alipay |

##### C.8.1 `api_secret` 轮转（v1）

| 方向 | 使用的密钥 |
|------|------------|
| **入站** 商户 API 验签（K2→平台） | 先 `api_secret`，失败再试 `api_secret_prev`（若非空） |
| **出站** 平台→K2 notify 签名 | **始终且仅用当前 `api_secret`**（**永不**用 `api_secret_prev`） |

**Runbook（冻结顺序）**：

1. 平台写：`api_secret_prev = 旧值`，`api_secret = 新值`（出站 notify 立即改签新 secret）；
2. **立即**更新 K2 `payment_methods.config.api_secret` 为新值（避免 HandleNotify 验签失败；窗口内 notify 会按 C.6.1 重试）；
3. 入站 Create/Query/Close：K2 已用新 secret 时走主密钥；若有滞后客户端仍可用 prev 入站；
4. 24h 后清空 `api_secret_prev`；
5. 无 prev 字段时：接受短暂双端失败，依赖 notify 重试 + 人工 sync。

#### C.9 安全

| 项 | 要求 |
|----|------|
| 传输 | 生产 HTTPS；`https_only` |
| 商户签名 | **仅** B.8.1 body_sha256 四元组 |
| Notify 签名 | B.8.4 + **app_id 匹配 cfg**（K2）；出站仅用当前 `api_secret` |
| 重放 | timestamp + api_nonces |
| 限流 | app_id + IP |
| 密钥 | 不入日志 |
| 金额 | 后端订单为准 |
| cashier_token | ≥128-bit CSPRNG；防枚举 |
| return 跳转 | 仅建单时 `return_url`；禁止客户端覆盖（C.7） |
| notify_url SSRF | exact host allowlist；**生产** `ssrf_block_private_ip=true`：DNS 解析后拒绝 RFC1918、链路本地、localhost、云 metadata（如 169.254.169.254） |
| mock_pay | 生产强制关 |

#### C.10 幂等与并发

- Create：唯一键 `(app_id, out_trade_no)`；
- 支付 CAS：`WHERE status='pending'`；
- close/expire 与 pay 竞态：已付优先 → paid 或 paid_orphan；
- K2 notify 成功后不再发；K2 侧 paid 幂等 ACK。

---

### D. 完美对接语义映射表

| K2 概念 | 发卡平台概念 | 说明 |
|---------|--------------|------|
| `order.TradeNo` | `out_trade_no` | 主键对齐 |
| `order.TotalAmount` **分** | `amount`/`paid_amount` **分** | 严格等额；**非** bepusdt 元 |
| `order.Currency` | `currency` | 默认 CNY |
| `order.PlanName` ≤128 | `subject` | 见 C.4.1 |
| `payment_methods.code=giftcard` | Merchant `app_id` | 面板↔商户 |
| `opts.NotifyURL` | `notify_url` | |
| `opts.RedirectURL` | `return_url` | same-host |
| `PaymentIntent.URL` | `cashier_url` | redirect |
| `HandleNotify` | 平台→K2 notify | 验签+app_id |
| `ProcessNotify` 金额 | `paid_amount` 分 | 不一致 400 |
| `QueryPayment` / `ReconcileFromGateway` | GET 查单 | 丢回调 |
| `ClosePayment` / `tryCloseRemote` | POST close + 支付宝 close | 软成功码见表 |
| `remark=closed by user` | closed；迟到 `paid_orphan` | 不履约 |
| `remark=auto-expired` | expired；迟到可 paid | 可恢复 |
| `CallbackNo` | `alipay_trade_no` 优先 | |
| `need_manual`（运维） | `status==paid_orphan` | 无独立列 |
| ACK `ok` | 停止重试 | |
| 支付宝密钥 | 平台全局配置 | K2 仅 api_secret |

#### 「用户取消后仍付款」策略

| 阶段 | 行为 |
|------|------|
| K2 取消 | `closed by user` → tryCloseRemote → 平台 close + 支付宝 close |
| close 成功未付 | 拒绝再付 |
| 竞态已付 | `paid_orphan`（= need_manual） |
| 回调 K2 | **仍发** `status=paid` |
| K2 | ACK **不履约**（`ProcessNotify`） |
| 对账 | canQuery 不含 closed-by-user |
| 运维 | 退款或 admin mark-paid |

---

## API / Interface Changes

### K2 对外 API

**无破坏性变更**（可选 Checkout 自动 reconcile 为增强）：

```text
POST /user/orders/:trade_no/checkout  { method: "giftcard", return_url? }
POST /user/orders/:trade_no/sync      # 丢 notify / 40901 后用户补单（已有）
POST /api/v1/payment/notify/giftcard
POST /admin/orders/:id/sync
```

### K2 内部

- 新增 `GiftCardGateway`
- 可选：`Checkout` 识别 `giftcard: already_paid:` → `ReconcileFromGateway`

### 平台 API 一览

| 方法 | 路径 | 调用方 |
|------|------|--------|
| POST | `/api/v1/orders` | CreatePayment |
| GET | `/api/v1/orders/{out_trade_no}` | QueryPayment |
| POST | `/api/v1/orders/{out_trade_no}/close` | ClosePayment |
| POST | `/alipay/notify` | 支付宝 |
| GET | `/c/{token}` | 用户 |
| GET | `/public/orders/status` | 收银台 |
| * | `/admin/api/v1/*` | 运维 |

---

## Data Model Changes

### K2

- 无强制 migration；`payment_methods` 增 `giftcard` 行；切换时 `alipay.enable=0`（密钥保留至 drain 完成）。

### 平台

- 新建 `merchants`（含 `api_secret_prev`）、`orders`、`api_nonces`、可选 `admin_audit_logs`。

---

## Alternatives Considered

### 1. 继续 K2 直连支付宝

| 优点 | 缺点 |
|------|------|
| 已实现 | 面板持私钥；不符合迁移目标 |

**结论**：保留代码回滚，生产禁用。

### 2. 独角 / Dujiao-Next 中转

SKU/金额/关单语义摩擦大 → 否决。

### 3. 自建极简发卡平台（**推荐**）

按 K2 契约设计；需维护独立服务。

### 4. bepusdt 风格但支付宝密钥仍在 K2

不满足密钥剥离 → 否决。

### 5. 仅远程 RSA 代签 / HSM / 易支付聚合（无自建收银台）

可把私钥移出面板，但 **没有** 统一 orphan 域、关单语义、独立收银台 UX 与多面板 `out_trade_no` 隔离；产品目标是「发卡/收银台平台」→ **否决为 v1 主方案**（可作未来密钥托管补充，不替代本平台）。

---

## Security & Privacy Considerations

| 威胁 | 严重度 | 缓解 |
|------|--------|------|
| 伪造 K2 notify | 高 | SignMD5 + secret；**app_id 匹配**；金额二次校验 |
| 重放 | 中 | timestamp + nonce 表 |
| SSRF via notify_url | 高 | allowlist + **生产禁私网 IP** |
| 商户 API 路径篡改 | 中 | TLS + app_id 归属；path 不在签名内（已知取舍） |
| MD5-as-MAC 理论弱点 | 中 | 高熵 secret + HTTPS；v1 接受；不用于无密钥材料完整性场景 |
| mock_pay 进生产 | 高 | is_production 强制关闭 |
| 密钥日志 | 高 | redact |

---

## Observability

### 指标

`orders_created_total`、`orders_paid_total`、`orders_orphan_total`（paid_orphan）、`k2_notify_success/failed`、`expire_worker_closed_total`、`alipay_notify_total`。

### 告警

notify 失败 >1h；orphan 新增；验签失败突增；expire 与支付宝 close 失败率。

### 密钥轮转

见 C.8.1；指标可观察 40101 在轮转窗口的尖刺。

---

## Rollout Plan

### 阶段（含 in-flight alipay drain）

1. **开发/沙箱**：平台 `mock_pay` 或支付宝沙箱；K2 测试环境 giftcard。
2. **并行**：生产配置 giftcard **不**对普通用户启用；内部验收。
3. **切换（有序）**：
   1. 启用 giftcard（用户可见）；
   2. **`alipay.enable=false`** — 阻止 **新** checkout 选支付宝；**保留** `payment_methods.config` 内 RSA 密钥；
   3. **Drain**：等待 ≥ `defaultOrderTTL`（30m）+ 余量；跑 `ReconcileStalePending` / 管理端对仍 `pending` 且 `payment_method=alipay` 的订单 sync；确认无 in-flight 或接受人工收尾；
   4. **此时**才从面板 config **删除**支付宝 RSA 密钥；
   5. **保留** `alipay.go` 代码与 method 行（enable=0）便于回滚。
4. 观察 48h（giftcard notify、orphan）。
5. Runbook 定稿。

> **说明**：`ProcessNotify` 按 `code` 加载 method **不要求 enable**，故 disable 后只要密钥仍在，迟到的支付宝直连 notify **仍可验签履约**。过早删密钥会破坏 drain。

### 回滚

重新 `alipay.enable=true` 并恢复密钥配置；`giftcard.enable=false`。

### 网络

用户↔K2/平台/支付宝公网 HTTPS；K2↔平台互通；支付宝仅见平台 notify。

### 本地开发

- `alipay.mock_pay: true`（非 production）；
- 或支付宝沙箱；
- K2 `site_url` 用隧道暴露。

---

## Testing Plan

### 单元

| 用例 | 期望 |
|------|------|
| 附录 C 黄金向量 | 签名逐字节一致 |
| GET 空 body sha256 | `e3b0c4…b855` |
| 金额 **2888 分** notify | Success；履约等额 |
| 误用 **28.88** | 解码失败或金额 mismatch |
| app_id mismatch | HandleNotify error |
| 坏签名 | error |
| Close 软成功码 | 40401/40901/40902 → nil |
| Query 40401 | `Paid:false, err=nil` |

### 集成

| 场景 | 期望 |
|------|------|
| checkout → mock pay → notify | K2 paid + 权益 |
| 丢 notify → sync | 补单成功 |
| 平台已 paid 再 checkout（40901） | reconcile / already_paid 路径 |
| 用户取消后到账 | orphan；K2 不履约 |
| auto-expired 到账 | 可恢复 |
| expire worker | pending→expired + alipay close 尝试 |
| 平台 expired 后 K2 仍 pending 再 Create | revive 成功：新 platform_trade_no + 新 cashier_token，code=0；旧号可入 history |
| revive 后再次拉支付宝 | 使用新 platform_trade_no，不复用已 close 的旧号 |
| 付 PT1 → revive 到 PT2 → 迟到 SUCCESS(PT1) | 订单 **一次** paid；`latest_paid_platform_trade_no=PT1`；回调 K2；收银台禁二付；best-effort close PT2 |
| 已 paid 后再收到任意 PT SUCCESS | 幂等 ACK，不重复入队履约 |
| 平台 expire_at 不短于 K2 ExpiredAt | 配置 alipay.timeout_express=5m 时平台订单仍 30m |
| 金额篡改 notify | 400，不履约 |
| SSRF notify_url 私网 | 创建拒绝（生产） |

---

## Open Questions

| # | 问题 | 默认（已冻结倾向） |
|---|------|-------------------|
| 1 | 支付宝 out_trade_no | **platform_trade_no**（已定） |
| 2 | 多 K2 / 多支付宝 | 多 Merchant（多面板）；**单支付宝账号/部署**（已定） |
| 3 | page/wap | 默认 page；可 auto UA |
| 4 | SQLite vs MySQL | 起步 SQLite；生产建议 MySQL |
| 5 | 是否删除 alipay 代码 | **否** |
| 6 | orphan 告警渠道 | v1 列表+日志；后置钉钉 |
| 7 | Checkout 是否自动 reconcile 40901 | **推荐做**；固定前缀 `giftcard: already_paid:` + 断言 `Status==paid`（见 B.4 伪代码） |

---

## References

- `docs/PAYMENT_DESIGN.md`
- `internal/payment/gateway.go`、`closer.go`、`options.go`、`sign.go`
- `internal/payment/gateways/alipay.go`、`bepusdt.go`、`epusdt.go`
- `internal/services/order_svc.go`（`Checkout`, `ProcessNotify`, `MarkPaidAndFulfill`, `ReconcileFromGateway`, `tryCloseRemote`, `CancelByUser`, `defaultOrderTTL`）
- `internal/handlers/payment/notify.go`
- `internal/handlers/user/order.go`（`SyncOrder`）
- `internal/models/order.go`
- `github.com/smartwalle/alipay/v3`

---

## 给 AI 的实现顺序（Checklist）

### Phase 0 — 准备

- [ ] 建仓库 `giftcard-platform`；K2 分支 `feat/payment-giftcard`
- [ ] 将 B.8 / C.5 / C.6 **契约冻结**进 `docs/API.md`

### Phase 1 — 脚手架 + 签名 + nonce  
**repo: giftcard-platform · 平台 MVP 起点**

- [ ] Gin + config + GORM models（含 `api_nonces`）
- [ ] B.8.1 中间件 + 附录 C 向量单测
- [ ] nonce TTL 清理

### Phase 2 — 商户 API + Expire worker  
**repo: giftcard-platform**

- [ ] Create/Get/Close + C.5 HTTP/code 表
- [ ] Expire worker（C.3.1，可先不调支付宝 close）；`expire_at` 仅跟 K2；Create revive expired
- [ ] SSRF allowlist（mock 阶段可关私网检查）

### Phase 3 — 收银台 + mock_pay  
**repo: giftcard-platform**

- [ ] `/c/{token}`、public status、`alipay.mock_pay`

### Phase 4 — K2 notify worker  
**repo: giftcard-platform**

- [ ] 签名 POST + C.6.1 矩阵；**不含** admin HTTP renotify

### Phase 5 — 支付宝沙箱  
**repo: giftcard-platform**（可拆 5a pay+notify / 5b query+close+orphan）

- [ ] 迁移 alipay 逻辑；timeout_express=剩余 TTL
- [ ] expire/close 调 trade.close；orphan 路径

### Phase 6 — 管理端  
**repo: giftcard-platform**

- [ ] renotify / merchants / secret 轮转 / orphan 筛选

### Phase 7 — K2 giftcard Gateway（**单文件插件级**）  
**repo: K2Board**

- [ ] 依赖：**API+notify body 契约已冻结**（Phase 0/4 文档）
- [ ] Create/HandleNotify/Query/Close + 单测（分、app_id、黄金向量、软 close）
- [ ] `already_paid` 错误前缀；可选 Checkout reconcile

### Phase 8 — 契约/E2E 测试（范围收敛）  
**双 repo**

- [ ] **最小**：平台 mock_pay + httptest 契约测试（签名/create/notify 向量）
- [ ] **次级**：K2 sqlite/test 启动 + 配置 giftcard + checkout + mock + 断言 paid（脚本，非完整浏览器）
- [ ] 浏览器路径可选

### Phase 9 — Runbook + 生产切换（含 drain）

- [ ] 按 Rollout 有序 disable alipay → drain → 删密钥

---

## Key Decisions

| # | 决策 | 理由 |
|---|------|------|
| 1 | 网关 code = **`giftcard`** | 稳定 notify 路径与产品语义 |
| 2 | **自建**极简平台，不魔改独角 | 契约可控 |
| 3 | 商户请求签名 **唯一**：`SignMD5({app_id,timestamp,nonce,body_sha256})`，body 为 **原始字节**；GET 用 `sha256("")` | 消除歧义；防 JSON 格式分叉 |
| 4 | path/query **不**入签 | 简化；TLS + app_id 归属 |
| 5 | 金额 giftcard 路径 **整数分**；支付宝层再转元 | 对齐 K2；避免抄 bepusdt 元 |
| 6 | 支付宝 out_trade_no = **platform_trade_no** | 多面板隔离 |
| 7 | **v1 全局一套支付宝密钥/部署**；Merchant 只隔离 K2 面板 | 消解 multi-merchant vs 多支付宝张力 |
| 8 | Close/Query/notify 重试完整实现 | 对齐 tryCloseRemote / Reconcile |
| 9 | Close 软成功：`0/40401/40902/40901` → Gateway `nil` | 避免 cancel 路径 Warn 风暴 |
| 10 | 取消后到账：`paid_orphan` + 仍回调；K2 不履约 | 复用现网语义 |
| 11 | `need_manual := status==paid_orphan` | 无冗余列 |
| 12 | HandleNotify **强制 app_id==cfg.AppID** | 纵深防御，对齐 alipay 网关 |
| 13 | 40901 already paid → 错误前缀 + **推荐** Checkout reconcile；保底 sync API | 丢 notify 可恢复 |
| 14 | **平台 `orders.expire_at` 仅跟 K2 `expire_at`**（可 hard_max 24h）；**禁止**用 `alipay.timeout_express` 缩短平台寿命；支付宝 `timeout_express` **仅在拉起支付时** = min(config, remaining) | 避免平台先于 K2 过期 → Create 40902 卡死 pending 单 |
| 14b | `expired` + 未来 expire_at 允许 **revive**：`out_trade_no`（K2）不变；**新 `platform_trade_no` + 新 `cashier_token`**；清空 alipay 字段；history 写入 meta | expire 可能已 `trade.close` 旧号，复用支付宝 out_trade_no 会阻断再付 |
| 14d | 历史 PT 上迟到 **SUCCESS/FINISHED**：**必须受理付款**（pending→paid / closed→orphan），禁止当噪音丢弃；已付幂等；受理后禁二付 + best-effort close 新 PT | 防漏单与双扣 |
| 14c | 出站 notify **只签当前 `api_secret`**；入站可 prev | 轮转时立刻改 K2 config |
| 15 | 切换时 **先 disable 再 drain 再删密钥** | 保护 in-flight alipay |
| 16 | K2 **不改**状态机核心；Checkout 自动 reconcile 为可选小改 | 风险可控 |
| 17 | 退款 Non-Goal；无库存依赖 | 收窄范围 |
| 18 | Go+Gin；MD5-as-MAC 为 v1 legacy 兼容 | 工程实用 |
| 19 | `api_secret` + 可选 `api_secret_prev` 轮转 | 降 downtime |
| 20 | 生产 SSRF：allowlist + 禁私网 IP | 平台主动出站风险 |
| 21 | mock_pay 配置项显式；production 强制关 | 安全 |

---

## PR Plan

> 平台 PR-1…6 ≈ **平台 MVP**；PR-7 ≈ **K2 单文件插件**；PR-8 契约测试优先；PR-9 切换。

### PR-1 — `giftcard-platform`: 脚手架 + 签名 + nonce 表

| 项 | 内容 |
|----|------|
| **标题** | feat: bootstrap with SignMD5 body_sha256 auth and api_nonces |
| **影响** | cmd/server, config, models, sign, migrations, 附录 C 向量测试 |
| **依赖** | 无 |
| **简述** | 健康检查；**唯一** B.8 算法；GET 空 body；nonce TTL 清理 |

### PR-2 — `giftcard-platform`: 商户 API + expire worker（本地）

| 项 | 内容 |
|----|------|
| **标题** | feat: orders create/get/close API, HTTP/code table, expire worker |
| **影响** | service/order, expire.go, httpserver |
| **依赖** | PR-1 |
| **简述** | C.5 全表；幂等+expired revive（**新 platform_trade_no**）；expire_at 跟 K2；expire CAS→expired（支付宝 close 可 stub）；SSRF allowlist |

### PR-3 — `giftcard-platform`: 收银台 + mock_pay

| 项 | 内容 |
|----|------|
| **标题** | feat: cashier UX and alipay.mock_pay |
| **影响** | web/cashier, config alipay.mock_pay |
| **依赖** | PR-2 |
| **简述** | 展示/倒计时/mock 置 paid 并入队（worker 可 no-op 直至 PR-4） |

### PR-4 — `giftcard-platform`: K2 notify worker（无 admin HTTP）

| 项 | 内容 |
|----|------|
| **标题** | feat: signed K2 notify worker with retry matrix |
| **影响** | service/notify_k2.go |
| **依赖** | PR-2, PR-3 |
| **简述** | C.6.1；**手动 renotify HTTP 留给 PR-6**；worker 可提供内部 `EnqueueRenotify(id)` |

### PR-5a — `giftcard-platform`: 支付宝 pay + notify

| 项 | 内容 |
|----|------|
| **标题** | feat: Alipay page/wap pay and async notify |
| **影响** | internal/alipay, /alipay/notify |
| **依赖** | PR-2, PR-4 |
| **简述** | platform_trade_no；timeout_express=剩余；mock 可并存 |

### PR-5b — `giftcard-platform`: Alipay query/close + orphan + expire close

| 项 | 内容 |
|----|------|
| **标题** | feat: Alipay query/close, expire trade.close, paid_orphan, historical SUCCESS |
| **影响** | alipay close/query, expire worker 实调, orphan CAS, history lookup notify |
| **依赖** | PR-5a |
| **简述** | 完整关单与竞态；**历史 PT SUCCESS 受理**（C.3.1.1）；禁二付 + close 新 PT |

### PR-6 — `giftcard-platform`: 管理端

| 项 | 内容 |
|----|------|
| **标题** | feat: admin orders, renotify HTTP, merchants, secret rotation |
| **影响** | internal/admin |
| **依赖** | PR-4, PR-5b |
| **简述** | Bearer；orphan 筛选；调用 PR-4 内部 renotify |

### PR-7 — `K2Board`: giftcard Gateway

| 项 | 内容 |
|----|------|
| **标题** | feat(payment): giftcard gateway (cents, body_sha256, closer) |
| **影响** | `internal/payment/gateways/giftcard.go`, `*_test.go`；可选 Checkout 5 行 reconcile；可选 `docs/PAYMENT_DESIGN.md` 一节 |
| **依赖** | **契约冻结**（B.8/C.5/C.6）；联调依赖平台 PR-4+；可与 PR-2 起并行写单测 mock server |
| **简述** | 完整 Gateway+Closer；app_id 校验；40901 前缀；Close 软成功 |

### PR-8 — 契约与最小 E2E（范围冻结）

| 项 | 内容 |
|----|------|
| **标题** | test: giftcard contract tests and minimal mock-pay fulfill path |
| **影响** | platform `tests/contract` 或 K2 `scripts/`；**不**要求完整双 repo docker 浏览器矩阵 |
| **依赖** | PR-4, PR-7 |
| **简述** | (1) 签名/create/notify 黄金向量 CI；(2) 可选：K2 test DB + mock_pay 平台 → paid 断言；fixture 写明如何插入 payment_method 与 user token |

### PR-9 — 生产切换 Runbook

| 项 | 内容 |
|----|------|
| **标题** | docs: cutover with alipay drain then key removal |
| **影响** | 双仓 README/runbook |
| **依赖** | PR-5b, PR-7, 预发验收 |
| **简述** | enable giftcard → disable alipay → drain 30m+reconcile → 删面板 RSA；回滚步骤 |

### PR 依赖图

```mermaid
flowchart LR
  P1[PR-1 auth] --> P2[PR-2 API+expire]
  P2 --> P3[PR-3 cashier]
  P2 --> P4[PR-4 notify worker]
  P3 --> P4
  P4 --> P5a[PR-5a alipay pay]
  P5a --> P5b[PR-5b close/orphan]
  P4 --> P6[PR-6 admin]
  P5b --> P6
  P2 -.->|契约| P7[PR-7 K2 gateway]
  P4 -.->|notify 契约| P7
  P4 --> P8[PR-8 tests]
  P7 --> P8
  P5b --> P9[PR-9 cutover]
  P7 --> P9
  P8 --> P9
```

---

## 附录 A — K2 Checkout 拼装 NotifyURL

符号：`OrderService.Checkout`（`internal/services/order_svc.go`）。

对非 mock 渠道：

- `opts.NotifyURL = PublicBaseURL() + "/api/v1/payment/notify/" + methodCode`
- `opts.RedirectURL` = same-host `return_url` 或默认 `/#/user/order-result?trade_no=` + TradeNo

`methodCode=giftcard` ⇒ notify 路径自动正确。

## 附录 B — ProcessNotify 与 closed-by-user

符号：`OrderService.ProcessNotify` / `MarkPaidAndFulfill`。

- `closed by user`：ACK、不履约。
- 金额：`PaidAmount` 必须等于 `TotalAmount`（分）。
- `auto-expired`：可恢复。
- Admin 人工：`method`/`callbackNo` 走 manual 路径。

用户补单：`POST /user/orders/:trade_no/sync` → `ReconcileFromGateway`。

## 附录 C — 签名黄金向量（实现单测必须对齐）

**公共参数**：

```text
api_secret = "test_secret"
app_id     = "k2-main"
timestamp  = "1720770000"
nonce      = "n1n2n3n4n5n6n7n8"
```

### C.1 GET 查单（空 body）

```text
raw_body = (empty, zero bytes)
body_sha256 = e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855

sign_raw =
  app_id=k2-main
  &body_sha256=e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
  &nonce=n1n2n3n4n5n6n7n8
  &timestamp=1720770000
  + test_secret

X-Signature = 15906004c50c79f16dca9d067124e4c3
```

### C.2 POST 创建订单

```text
raw_body (exact UTF-8, no extra spaces) =
{"out_trade_no":"T1","amount":100,"currency":"CNY","subject":"VIP","notify_url":"https://panel.example.com/api/v1/payment/notify/giftcard","return_url":"https://panel.example.com/#/user/order-result?trade_no=T1"}

body_sha256 = c4f97996d3633582e36ae6f438e9c563b77cc3c265bad47f868a8b9ddad12a85
X-Signature = 0d39b855a6a1e46bba9f78e5839fd15d
```

> `amount:100` 表示 **100 分 = ¥1.00**，不是 100 元。

### C.3 POST 关单

```text
raw_body = {"reason":"k2_cancel"}
body_sha256 = 02222ae436a62053014c0453dd2a871c481c5c3668e44e8adcd14b43d1b86dc3
X-Signature = a97ed3842d6bc2906bd0c4228315c28d
```

### C.4 Notify body 签名（平台→K2）

字段（签名用十进制整数字符串）：

```text
app_id=k2-main
out_trade_no=T1
platform_trade_no=GC1
amount=100
paid_amount=100
currency=CNY
status=paid
alipay_trade_no=ALI1
paid_at=1720770000
timestamp=1720770001
nonce=notifynonce0001

signature = f0acd306f758fc3062a40effee6b99b8
```

（`SignMD5` 按 key 排序后拼接再加 secret。）

## 附录 D — 生产配置对照

### K2 giftcard method

```json
{
  "code": "giftcard",
  "name": "支付宝（发卡收银台）",
  "enable": true,
  "sort": 10,
  "config": {
    "base_url": "https://pay.example.com",
    "app_id": "k2-main",
    "api_secret": "<platform merchant secret>",
    "timeout_sec": 20,
    "product_name_template": "{plan_name}",
    "sign_version": "v1"
  }
}
```

### 切换清单（摘要）

1. giftcard enable=true  
2. alipay enable=false（**保留密钥**）  
3. drain ≥30m + reconcile alipay pending  
4. 删除面板支付宝 RSA  
5. 回滚预案演练  

---

*文档结束（R4）。实现以 B.8 / C.3.1 TTL+revive+历史 SUCCESS 受理 / C.5 / C.6 冻结契约为准；与代码冲突时先改本文。*

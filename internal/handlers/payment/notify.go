package payment

import (
	"bytes"
	"errors"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"K2board/internal/services"
)

// firstReturnTradeNo picks merchant order id from browser return query.
// Epay: out_trade_no = merchant; trade_no = platform serial (do not prefer).
func firstReturnTradeNo(c *gin.Context) string {
	return pickReturnTradeNo(
		c.Query("out_trade_no"),
		c.Query("tn"),
		c.Query("order_id"),
		c.Query("trade_no"),
	)
}

// pickReturnTradeNo is pure for tests: first non-empty among candidates;
// comma-joined values prefer a K2* segment (merchant trade_no).
func pickReturnTradeNo(candidates ...string) string {
	for _, raw := range candidates {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if strings.Contains(v, ",") {
			for _, p := range strings.Split(v, ",") {
				p = strings.TrimSpace(p)
				if strings.HasPrefix(strings.ToUpper(p), "K2") {
					return p
				}
			}
		}
		return v
	}
	return ""
}

// NotifyHandler receives async payment provider callbacks (no JWT).
type NotifyHandler struct {
	orders *services.OrderService
}

func NewNotifyHandler() *NotifyHandler {
	return &NotifyHandler{orders: services.NewOrderService()}
}

// Notify POST|GET /api/v1/payment/notify/:code
// Responds with plain "ok" / "success" on acceptance (provider-specific).
func (h *NotifyHandler) Notify(c *gin.Context) {
	code := strings.TrimSpace(c.Param("code"))
	if code == "" {
		c.String(http.StatusBadRequest, "missing code")
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.String(http.StatusBadRequest, "read body failed")
		return
	}
	// 易支付等渠道常用 GET query 异步通知；body 为空时回退 query / form
	if len(bytes.TrimSpace(body)) == 0 {
		if raw := strings.TrimSpace(c.Request.URL.RawQuery); raw != "" {
			body = []byte(raw)
		} else if err := c.Request.ParseForm(); err == nil && len(c.Request.Form) > 0 {
			body = []byte(c.Request.Form.Encode())
		}
	}

	headers := map[string]string{}
	for k, v := range c.Request.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	ack, err := h.orders.ProcessNotify(c.Request.Context(), code, headers, body)
	if err != nil {
		slog.Warn("payment notify failed", "code", code, "error", err)
		// Amount mismatch / bad sig → 400 so provider may retry carefully
		if errors.Is(err, services.ErrAmountMismatch) {
			c.String(http.StatusBadRequest, "amount mismatch")
			return
		}
		if errors.Is(err, services.ErrOrderNotFound) {
			c.String(http.StatusNotFound, "order not found")
			return
		}
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	if !ack {
		c.String(http.StatusBadRequest, "not acked")
		return
	}

	// Provider-specific ACK bodies:
	// - Alipay / 易支付 require exact "success"
	// - Epusdt/BEpusdt accept "ok" or "success"
	// 易支付 / 青蛙（含 frog_alipay、frog_wx 等多实例 code）：应答体必须是 success
	base := code
	if i := strings.IndexByte(code, '_'); i > 0 {
		base = code[:i]
	}
	switch base {
	case "alipay", "epay", "frog":
		c.String(http.StatusOK, "success")
	default:
		c.String(http.StatusOK, "ok")
	}
}

// Return is a browser landing when redirect_url points at the API host.
// Tries to redirect to user portal order-result; otherwise shows a clear HTML hint
// so users know to re-login if the third-party never jumps back.
//
// Param priority (易支付等): out_trade_no = merchant order id; trade_no is often
// the provider serial and must NOT be preferred for K2 lookup.
func (h *NotifyHandler) Return(c *gin.Context) {
	tradeNo := firstReturnTradeNo(c)

	siteURL := strings.TrimRight(services.PublicBaseURL(), "/")
	if siteURL != "" && tradeNo != "" {
		// Hash SPA: prefer tn= so provider-appended trade_no does not clobber merchant id
		dest := siteURL + "/#/user/order-result?tn=" + url.QueryEscape(tradeNo)
		c.Redirect(http.StatusFound, dest)
		return
	}

	// Friendly fallback page (not bare JSON) when site_url missing
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, `<!DOCTYPE html><html lang="zh-CN"><head>
<meta charset="utf-8"/><meta name="viewport" content="width=device-width,initial-scale=1"/>
<title>支付返回 · 东京热云</title>
<style>
body{font-family:system-ui,sans-serif;background:#0c0a14;color:#f1f5f9;margin:0;min-height:100vh;display:grid;place-items:center;padding:24px}
.card{max-width:420px;background:rgba(255,255,255,.08);border:1px solid rgba(255,255,255,.15);border-radius:16px;padding:28px 24px;line-height:1.6}
h1{font-size:18px;margin:0 0 12px}p{margin:0 0 10px;color:#cbd5e1;font-size:14px}
.tip{background:rgba(244,63,94,.12);border:1px solid rgba(244,63,94,.3);border-radius:12px;padding:12px;font-size:13px;margin-top:14px}
code{color:#fda4af}
</style></head><body><div class="card">
<h1>支付处理中</h1>
<p>订单号：<code>%s</code></p>
<p>若您已完成付款，套餐通常会在后台自动开通。</p>
<div class="tip"><b>未自动跳回本站？</b><br/>请重新打开用户中心并登录，仪表盘即可看到已生效的套餐。也可进入「我的订单」查看支付状态。</div>
</div></body></html>`, html.EscapeString(tradeNo))
}

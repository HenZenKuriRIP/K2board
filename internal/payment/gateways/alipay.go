package gateways

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	alipay "github.com/smartwalle/alipay/v3"

	"K2board/internal/models"
	"K2board/internal/payment"
)

// AlipayGateway integrates Alipay merchant OpenAPI (page / wap pay).
// Config is stored on payment_methods.config as JSON.
type AlipayGateway struct{}

func (AlipayGateway) Code() string { return "alipay" }
func (AlipayGateway) Name() string { return "支付宝" }

// alipayConfig holds merchant credentials.
// private_key: app RSA private key (PKCS1 or PKCS8 PEM text; \n escaped OK)
// alipay_public_key: Alipay platform public key (not app public key)
type alipayConfig struct {
	AppID           string `json:"app_id"`
	PrivateKey      string `json:"private_key"`
	AlipayPublicKey string `json:"alipay_public_key"`
	IsProduction    bool   `json:"is_production"`
	// Product: "page" (电脑网站, default) | "wap" (手机网站)
	Product string `json:"product"`
	// TimeoutExpress e.g. "30m" — optional Alipay close window
	TimeoutExpress string `json:"timeout_express"`
}

func parseAlipayConfig(cfg string) (alipayConfig, error) {
	var c alipayConfig
	if strings.TrimSpace(cfg) == "" {
		cfg = "{}"
	}
	if err := json.Unmarshal([]byte(cfg), &c); err != nil {
		return c, fmt.Errorf("invalid alipay config JSON: %w", err)
	}
	c.AppID = strings.TrimSpace(c.AppID)
	c.PrivateKey = normalizePEM(c.PrivateKey)
	c.AlipayPublicKey = normalizePEM(c.AlipayPublicKey)
	c.Product = strings.ToLower(strings.TrimSpace(c.Product))
	if c.Product == "" {
		c.Product = "page"
	}
	if c.TimeoutExpress == "" {
		c.TimeoutExpress = "30m"
	}
	return c, nil
}

func normalizePEM(s string) string {
	s = strings.TrimSpace(s)
	// JSON often stores PEM with literal \n
	s = strings.ReplaceAll(s, "\\n", "\n")
	return strings.TrimSpace(s)
}

func newAlipayClient(cfg alipayConfig) (*alipay.Client, error) {
	if cfg.AppID == "" {
		return nil, fmt.Errorf("alipay: app_id 未配置")
	}
	if cfg.PrivateKey == "" {
		return nil, fmt.Errorf("alipay: private_key 未配置")
	}
	if cfg.AlipayPublicKey == "" {
		return nil, fmt.Errorf("alipay: alipay_public_key 未配置（支付宝公钥，非应用公钥）")
	}
	client, err := alipay.New(cfg.AppID, cfg.PrivateKey, cfg.IsProduction)
	if err != nil {
		return nil, fmt.Errorf("alipay client: %w（请检查私钥 PKCS1/PKCS8 格式）", err)
	}
	if err := client.LoadAliPayPublicKey(cfg.AlipayPublicKey); err != nil {
		return nil, fmt.Errorf("alipay public key: %w", err)
	}
	return client, nil
}

func (AlipayGateway) CreatePayment(ctx context.Context, order *models.Order, cfgJSON string, opts payment.CreateOptions) (*payment.PaymentIntent, error) {
	_ = ctx
	cfg, err := parseAlipayConfig(cfgJSON)
	if err != nil {
		return nil, err
	}
	if opts.NotifyURL == "" || opts.RedirectURL == "" {
		return nil, fmt.Errorf("alipay: 缺少 notify_url/redirect_url（请配置 site_url）")
	}
	client, err := newAlipayClient(cfg)
	if err != nil {
		return nil, err
	}

	amount := payment.FormatFiatFromCents(order.TotalAmount)
	subject := sanitizeAlipaySubject(order.PlanName)

	var payURL *url.URL
	switch cfg.Product {
	case "wap":
		p := alipay.TradeWapPay{}
		p.NotifyURL = opts.NotifyURL
		p.ReturnURL = opts.RedirectURL
		p.Subject = subject
		p.OutTradeNo = order.TradeNo
		p.TotalAmount = amount
		p.ProductCode = "QUICK_WAP_WAY"
		p.Body = subject
		p.GoodsType = "0" // virtual
		p.TimeoutExpress = cfg.TimeoutExpress
		payURL, err = client.TradeWapPay(p)
	default: // page
		p := alipay.TradePagePay{}
		p.NotifyURL = opts.NotifyURL
		p.ReturnURL = opts.RedirectURL
		p.Subject = subject
		p.OutTradeNo = order.TradeNo
		p.TotalAmount = amount
		p.ProductCode = "FAST_INSTANT_TRADE_PAY"
		p.Body = subject
		p.GoodsType = "0"
		p.TimeoutExpress = cfg.TimeoutExpress
		payURL, err = client.TradePagePay(p)
	}
	if err != nil {
		return nil, fmt.Errorf("alipay create pay: %w", err)
	}
	if payURL == nil || payURL.String() == "" {
		return nil, fmt.Errorf("alipay: empty pay url")
	}

	return &payment.PaymentIntent{
		Type:     payment.IntentRedirect,
		URL:      payURL.String(),
		TradeNo:  order.TradeNo,
		Amount:   order.TotalAmount,
		Currency: order.Currency,
		Message:  "正在跳转支付宝收银台…",
		ExpireAt: order.ExpiredAt.Unix(),
		Extra: map[string]any{
			"gateway":  "alipay",
			"product":  cfg.Product,
			"pay_url":  payURL.String(),
			"amount":   amount,
			"out_trade_no": order.TradeNo,
		},
	}, nil
}

func (AlipayGateway) HandleNotify(ctx context.Context, _ map[string]string, body []byte, cfgJSON string) (*payment.NotifyResult, error) {
	cfg, err := parseAlipayConfig(cfgJSON)
	if err != nil {
		return nil, err
	}
	client, err := newAlipayClient(cfg)
	if err != nil {
		return nil, err
	}

	// Alipay posts application/x-www-form-urlencoded
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, fmt.Errorf("alipay: parse form: %w", err)
	}
	// Also accept if body was already key=value without encoding issues
	if len(values) == 0 && len(body) > 0 {
		return nil, fmt.Errorf("alipay: empty form body")
	}

	noti, err := client.DecodeNotification(ctx, values)
	if err != nil {
		return nil, fmt.Errorf("alipay: verify notify: %w", err)
	}

	// Official secondary check: app_id must match merchant app
	if noti.AppId != "" && !strings.EqualFold(noti.AppId, cfg.AppID) {
		return nil, fmt.Errorf("alipay: app_id mismatch (got %s)", noti.AppId)
	}

	// Only success / finished fulfill
	switch noti.TradeStatus {
	case alipay.TradeStatusSuccess, alipay.TradeStatusFinished:
		// ok
	default:
		return &payment.NotifyResult{
			TradeNo: noti.OutTradeNo,
			Success: false,
			Raw:     string(body),
		}, nil
	}

	paidCents := parseAlipayAmountToCents(noti.TotalAmount)
	return &payment.NotifyResult{
		TradeNo:    noti.OutTradeNo,
		PaidAmount: paidCents,
		Success:    true,
		CallbackNo: firstNonEmpty(noti.TradeNo, noti.NotifyId),
		Raw:        string(body),
	}, nil
}

func (AlipayGateway) QueryPayment(ctx context.Context, order *models.Order, cfgJSON string) (*payment.QueryResult, error) {
	cfg, err := parseAlipayConfig(cfgJSON)
	if err != nil {
		return nil, err
	}
	client, err := newAlipayClient(cfg)
	if err != nil {
		return nil, err
	}
	rsp, err := client.TradeQuery(ctx, alipay.TradeQuery{OutTradeNo: order.TradeNo})
	if err != nil {
		return nil, err
	}
	if rsp == nil {
		return &payment.QueryResult{Paid: false}, nil
	}
	// TradeQueryRsp embeds Error — check IsSuccess if available
	if !rsp.IsSuccess() {
		return &payment.QueryResult{Paid: false}, nil
	}
	paid := rsp.TradeStatus == alipay.TradeStatusSuccess || rsp.TradeStatus == alipay.TradeStatusFinished
	return &payment.QueryResult{
		Paid:       paid,
		PaidAmount: parseAlipayAmountToCents(rsp.TotalAmount),
		CallbackNo: rsp.TradeNo,
	}, nil
}

// ClosePayment implements payment.Closer (alipay.trade.close).
func (AlipayGateway) ClosePayment(ctx context.Context, order *models.Order, cfgJSON string) error {
	cfg, err := parseAlipayConfig(cfgJSON)
	if err != nil {
		return err
	}
	client, err := newAlipayClient(cfg)
	if err != nil {
		return err
	}
	rsp, err := client.TradeClose(ctx, alipay.TradeClose{OutTradeNo: order.TradeNo})
	if err != nil {
		return err
	}
	if rsp != nil && !rsp.IsSuccess() {
		// Already closed / not exists — treat as soft OK for local cancel paths
		sub := rsp.SubCode
		if sub == "ACQ.TRADE_NOT_EXIST" || sub == "ACQ.TRADE_STATUS_ERROR" ||
			strings.Contains(rsp.SubMsg, "交易不存在") || strings.Contains(rsp.SubMsg, "状态不合法") {
			return nil
		}
		return fmt.Errorf("alipay trade.close: %s %s", rsp.SubCode, rsp.SubMsg)
	}
	return nil
}

// sanitizeAlipaySubject removes chars Alipay forbids in subject (/,=, & etc.) and trims length.
func sanitizeAlipaySubject(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		s = "K2Board 套餐"
	}
	replacer := strings.NewReplacer(
		"/", " ",
		"\\", " ",
		"=", " ",
		"&", " ",
		"?", " ",
		"#", " ",
		"%", " ",
		"<", " ",
		">", " ",
		"\"", " ",
		"'", " ",
	)
	s = replacer.Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		s = "K2Board 套餐"
	}
	runes := []rune(s)
	if len(runes) > 256 {
		s = string(runes[:256])
	}
	return s
}

func parseAlipayAmountToCents(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return payment.FiatToCents(f)
}

func init() {
	payment.Register(AlipayGateway{})
}

package gateways

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"K2board/internal/models"
	"K2board/internal/payment"
)

// GiftCardGateway talks to a self-hosted giftcard/cashier platform that holds
// Alipay merchant credentials. Amounts are always integer cents (not yuan).
// See docs/GIFTCARD_PAYMENT_PLATFORM.md.
type GiftCardGateway struct {
	HTTPClient *http.Client
}

func (GiftCardGateway) Code() string { return "giftcard" }
func (GiftCardGateway) Name() string { return "发卡收银台" }

type giftcardConfig struct {
	BaseURL             string `json:"base_url"`
	AppID               string `json:"app_id"`
	APISecret           string `json:"api_secret"`
	TimeoutSec          int    `json:"timeout_sec"`
	ProductNameTemplate string `json:"product_name_template"`
	SignVersion         string `json:"sign_version"`
}

func parseGiftcardConfig(cfg string) (giftcardConfig, error) {
	var c giftcardConfig
	if strings.TrimSpace(cfg) == "" {
		cfg = "{}"
	}
	if err := json.Unmarshal([]byte(cfg), &c); err != nil {
		return c, fmt.Errorf("giftcard: invalid config JSON: %w", err)
	}
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	c.AppID = strings.TrimSpace(c.AppID)
	c.APISecret = strings.TrimSpace(c.APISecret)
	c.ProductNameTemplate = strings.TrimSpace(c.ProductNameTemplate)
	c.SignVersion = strings.TrimSpace(c.SignVersion)
	if c.TimeoutSec <= 0 {
		c.TimeoutSec = 20
	}
	// Default: neutral bill title — do NOT send plan names to giftcard/Alipay.
	// Ops can override with product_name_template (still sanitized). Placeholders:
	// {trade_no} full trade no; {trade_tail} last 6 chars; {plan_name} discouraged.
	if c.ProductNameTemplate == "" {
		c.ProductNameTemplate = "数字商品"
	}
	if c.SignVersion == "" {
		c.SignVersion = "v1"
	}
	return c, nil
}

func (c giftcardConfig) validate() error {
	if c.BaseURL == "" {
		return fmt.Errorf("giftcard: base_url 未配置")
	}
	if err := assertSafeGiftcardBaseURL(c.BaseURL); err != nil {
		return err
	}
	if c.AppID == "" {
		return fmt.Errorf("giftcard: app_id 未配置")
	}
	if c.APISecret == "" {
		return fmt.Errorf("giftcard: api_secret 未配置")
	}
	if c.SignVersion != "v1" {
		return fmt.Errorf("giftcard: unsupported sign_version %q (only v1)", c.SignVersion)
	}
	return nil
}

// assertSafeGiftcardBaseURL reduces SSRF risk from misconfigured admin base_url.
// Allows RFC1918/self-hosted cashiers (common for giftcard platforms) but blocks:
// cloud metadata (169.254.169.254), non-http(s) schemes, userinfo, empty host.
func assertSafeGiftcardBaseURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("giftcard: base_url 无效")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("giftcard: base_url 仅支持 http/https")
	}
	if u.User != nil {
		return fmt.Errorf("giftcard: base_url 不允许包含用户名密码")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("giftcard: base_url 主机名为空")
	}
	// Explicit metadata hostnames
	if host == "metadata.google.internal" || host == "metadata" ||
		strings.HasSuffix(host, ".metadata.google.internal") {
		return fmt.Errorf("giftcard: base_url 禁止指向云元数据地址")
	}
	// Resolve and reject link-local / metadata IP (does not ban 10.x — self-hosted OK)
	ips, err := net.LookupIP(host)
	if err != nil {
		// DNS failure: still allow URL shape check at create time; runtime will fail later.
		// Reject numeric link-local host without DNS.
		if ip := net.ParseIP(host); ip != nil && isBlockedOutboundIP(ip) {
			return fmt.Errorf("giftcard: base_url 禁止指向链路本地/元数据地址")
		}
		return nil
	}
	for _, ip := range ips {
		if isBlockedOutboundIP(ip) {
			return fmt.Errorf("giftcard: base_url 解析到禁止的地址 %s", ip.String())
		}
	}
	return nil
}

func isBlockedOutboundIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	// Cloud metadata / link-local
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	// Explicit AWS/GCP/Azure style metadata
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
	}
	return false
}

func (g GiftCardGateway) client(timeoutSec int) *http.Client {
	if g.HTTPClient != nil {
		return g.HTTPClient
	}
	if timeoutSec <= 0 {
		timeoutSec = 20
	}
	return &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
}

// giftcardAPIEnvelope is the platform response wrapper.
type giftcardAPIEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type giftcardOrderData struct {
	OutTradeNo      string `json:"out_trade_no"`
	PlatformTradeNo string `json:"platform_trade_no"`
	CashierURL      string `json:"cashier_url"`
	CashierToken    string `json:"cashier_token"`
	Status          string `json:"status"`
	Amount          int64  `json:"amount"`
	PaidAmount      int64  `json:"paid_amount"`
	Currency        string `json:"currency"`
	ExpireAt        int64  `json:"expire_at"`
	AlipayTradeNo   string `json:"alipay_trade_no"`
}

type giftcardNotify struct {
	AppID           string `json:"app_id"`
	OutTradeNo      string `json:"out_trade_no"`
	PlatformTradeNo string `json:"platform_trade_no"`
	Amount          int64  `json:"amount"`
	PaidAmount      int64  `json:"paid_amount"`
	Currency        string `json:"currency"`
	Status          string `json:"status"`
	AlipayTradeNo   string `json:"alipay_trade_no"`
	PaidAt          int64  `json:"paid_at"`
	Timestamp       int64  `json:"timestamp"`
	Nonce           string `json:"nonce"`
	Signature       string `json:"signature"`
}

func (g GiftCardGateway) CreatePayment(ctx context.Context, order *models.Order, cfgJSON string, opts payment.CreateOptions) (*payment.PaymentIntent, error) {
	cfg, err := parseGiftcardConfig(cfgJSON)
	if err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if opts.NotifyURL == "" || opts.RedirectURL == "" {
		return nil, fmt.Errorf("giftcard: 缺少 notify_url/redirect_url（请配置 site_url 或传入 return_url）")
	}

	currency := strings.TrimSpace(order.Currency)
	if currency == "" {
		currency = "CNY"
	}
	subject := giftcardSubject(cfg.ProductNameTemplate, order)
	expireAt := order.ExpiredAt.Unix()
	if expireAt <= 0 {
		expireAt = time.Now().Add(30 * time.Minute).Unix()
	}

	// Amount is integer cents — never yuan float.
	reqBody := map[string]any{
		"out_trade_no": order.TradeNo,
		"amount":       order.TotalAmount,
		"currency":     currency,
		"subject":      subject,
		"notify_url":   opts.NotifyURL,
		"return_url":   opts.RedirectURL,
		"expire_at":    expireAt,
		"user_ref":     fmt.Sprintf("u%d", order.UserID),
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("giftcard: marshal create body: %w", err)
	}

	respBody, status, err := g.doSigned(ctx, cfg, http.MethodPost, cfg.BaseURL+"/api/v1/orders", raw)
	if err != nil {
		return nil, err
	}

	env, data, err := parseGiftcardEnvelope(respBody)
	if err != nil {
		return nil, fmt.Errorf("giftcard: create response (HTTP %d): %w", status, err)
	}
	if env.Code == 40901 {
		st := strings.TrimSpace(data.Status)
		if st == "paid" || st == "paid_orphan" {
			return nil, fmt.Errorf("giftcard: already_paid: out_trade_no=%s", order.TradeNo)
		}
	}
	if env.Code != 0 {
		msg := env.Message
		if msg == "" {
			msg = fmt.Sprintf("code=%d", env.Code)
		}
		return nil, fmt.Errorf("giftcard: %s", msg)
	}
	if data.CashierURL == "" {
		return nil, fmt.Errorf("giftcard: create ok but cashier_url empty")
	}

	return &payment.PaymentIntent{
		Type:     payment.IntentRedirect,
		URL:      data.CashierURL,
		TradeNo:  order.TradeNo,
		Amount:   order.TotalAmount,
		Currency: currency,
		Message:  "正在跳转发卡收银台…",
		ExpireAt: data.ExpireAt,
		Extra: map[string]any{
			"gateway":           "giftcard",
			"platform_trade_no": data.PlatformTradeNo,
			"cashier_url":       data.CashierURL,
			"cashier_token":     data.CashierToken,
		},
	}, nil
}

func (g GiftCardGateway) HandleNotify(_ context.Context, _ map[string]string, body []byte, cfgJSON string) (*payment.NotifyResult, error) {
	cfg, err := parseGiftcardConfig(cfgJSON)
	if err != nil {
		return nil, err
	}
	if cfg.APISecret == "" {
		return nil, fmt.Errorf("giftcard: api_secret 未配置")
	}
	if cfg.AppID == "" {
		return nil, fmt.Errorf("giftcard: app_id 未配置")
	}

	var n giftcardNotify
	if err := json.Unmarshal(body, &n); err != nil {
		return nil, fmt.Errorf("giftcard: invalid notify JSON: %w", err)
	}
	if n.OutTradeNo == "" {
		return nil, fmt.Errorf("giftcard: empty out_trade_no")
	}
	if n.Signature == "" {
		return nil, fmt.Errorf("giftcard: missing signature")
	}

	signParams := map[string]string{
		"app_id":            n.AppID,
		"out_trade_no":      n.OutTradeNo,
		"platform_trade_no": n.PlatformTradeNo,
		"amount":            strconv.FormatInt(n.Amount, 10),
		"paid_amount":       strconv.FormatInt(n.PaidAmount, 10),
		"currency":          n.Currency,
		"status":            n.Status,
		"alipay_trade_no":   n.AlipayTradeNo,
		"paid_at":           strconv.FormatInt(n.PaidAt, 10),
		"timestamp":         strconv.FormatInt(n.Timestamp, 10),
		"nonce":             n.Nonce,
	}
	// Zero numeric fields FormatInt to "0" — SignMD5 skips empty strings only.
	// paid_at/timestamp may legitimately be 0 in tests; include non-empty only
	// for optional string fields already handled; for ints, always include via FormatInt
	// except we must match platform: empty string fields omitted by SignMD5.
	// paid_at=0 → "0" is non-empty and participates (platform should send real paid_at).

	expect := payment.SignMD5(signParams, cfg.APISecret)
	if !strings.EqualFold(expect, strings.TrimSpace(n.Signature)) {
		return nil, fmt.Errorf("giftcard: bad signature")
	}
	if n.AppID != cfg.AppID {
		return nil, fmt.Errorf("giftcard: app_id mismatch")
	}

	success := n.Status == "paid"
	return &payment.NotifyResult{
		TradeNo:    n.OutTradeNo,
		PaidAmount: n.PaidAmount,
		Success:    success,
		CallbackNo: firstNonEmpty(n.AlipayTradeNo, n.PlatformTradeNo),
		Raw:        string(body),
	}, nil
}

func (g GiftCardGateway) QueryPayment(ctx context.Context, order *models.Order, cfgJSON string) (*payment.QueryResult, error) {
	cfg, err := parseGiftcardConfig(cfgJSON)
	if err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	path := cfg.BaseURL + "/api/v1/orders/" + url.PathEscape(order.TradeNo)
	respBody, status, err := g.doSigned(ctx, cfg, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	env, data, err := parseGiftcardEnvelope(respBody)
	if err != nil {
		return nil, fmt.Errorf("giftcard: query response (HTTP %d): %w", status, err)
	}
	if env.Code == 40401 || status == http.StatusNotFound {
		return &payment.QueryResult{Paid: false}, nil
	}
	if env.Code == 40101 || env.Code == 40102 || env.Code == 40301 {
		msg := env.Message
		if msg == "" {
			msg = fmt.Sprintf("code=%d", env.Code)
		}
		return nil, fmt.Errorf("giftcard: query auth: %s", msg)
	}
	if env.Code != 0 {
		msg := env.Message
		if msg == "" {
			msg = fmt.Sprintf("code=%d", env.Code)
		}
		return nil, fmt.Errorf("giftcard: query: %s", msg)
	}

	paid := data.Status == "paid" || data.Status == "paid_orphan"
	return &payment.QueryResult{
		Paid:       paid,
		PaidAmount: data.PaidAmount,
		CallbackNo: firstNonEmpty(data.AlipayTradeNo, data.PlatformTradeNo),
	}, nil
}

// ClosePayment implements payment.Closer.
func (g GiftCardGateway) ClosePayment(ctx context.Context, order *models.Order, cfgJSON string) error {
	cfg, err := parseGiftcardConfig(cfgJSON)
	if err != nil {
		return err
	}
	if err := cfg.validate(); err != nil {
		return err
	}

	raw, err := json.Marshal(map[string]string{"reason": "k2_cancel"})
	if err != nil {
		return err
	}
	path := cfg.BaseURL + "/api/v1/orders/" + url.PathEscape(order.TradeNo) + "/close"
	respBody, status, err := g.doSigned(ctx, cfg, http.MethodPost, path, raw)
	if err != nil {
		return err
	}

	env, _, err := parseGiftcardEnvelope(respBody)
	if err != nil {
		return fmt.Errorf("giftcard: close response (HTTP %d): %w", status, err)
	}
	// Soft success: already gone / closed / paid
	switch env.Code {
	case 0, 40401, 40901, 40902:
		return nil
	default:
		msg := env.Message
		if msg == "" {
			msg = fmt.Sprintf("code=%d", env.Code)
		}
		return fmt.Errorf("giftcard: close: %s", msg)
	}
}

func parseGiftcardEnvelope(body []byte) (giftcardAPIEnvelope, giftcardOrderData, error) {
	var env giftcardAPIEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return env, giftcardOrderData{}, fmt.Errorf("invalid JSON: %w", err)
	}
	var data giftcardOrderData
	if len(env.Data) > 0 && string(env.Data) != "null" {
		if err := json.Unmarshal(env.Data, &data); err != nil {
			return env, data, fmt.Errorf("invalid data: %w", err)
		}
	}
	return env, data, nil
}

func giftcardSubject(tmpl string, order *models.Order) string {
	s := tmpl
	trade := strings.TrimSpace(order.TradeNo)
	tail := trade
	if len(tail) > 6 {
		tail = tail[len(tail)-6:]
	}
	s = strings.ReplaceAll(s, "{plan_name}", order.PlanName)
	s = strings.ReplaceAll(s, "{trade_no}", trade)
	s = strings.ReplaceAll(s, "{trade_tail}", tail)
	s = strings.TrimSpace(s)
	if s == "" {
		s = "数字商品"
	}
	s = sanitizeGiftcardSubject(s)
	// Align with models.Order PlanName size:128 (runes).
	if utf8.RuneCountInString(s) > 128 {
		r := []rune(s)
		s = string(r[:128])
	}
	return s
}

// sanitizeGiftcardSubject strips provider-forbidden chars and sensitive product hints
// so Alipay / giftcard bill titles stay neutral.
func sanitizeGiftcardSubject(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "数字商品"
	}
	// Alipay-forbidden / noisy punctuation
	replacer := strings.NewReplacer(
		"/", " ", "\\", " ", "=", " ", "&", " ", "?", " ",
		"#", " ", "%", " ", "<", " ", ">", " ", "\"", " ", "'", " ",
		"@", " ", ":", " ",
	)
	s = replacer.Replace(s)

	// Case-insensitive Latin keyword scrub
	lower := strings.ToLower(s)
	for _, w := range giftcardSubjectSensitiveEN {
		for {
			i := strings.Index(lower, w)
			if i < 0 {
				break
			}
			// replace in original by rune-safe byte index (ASCII keywords only)
			s = s[:i] + " " + s[i+len(w):]
			lower = strings.ToLower(s)
		}
	}
	// CJK / product sensitive phrases
	for _, w := range giftcardSubjectSensitiveCN {
		s = strings.ReplaceAll(s, w, " ")
	}

	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimSpace(s)
	if s == "" || isMostlySensitiveLeftover(s) {
		return "数字商品"
	}
	return s
}

var giftcardSubjectSensitiveEN = []string{
	"k2board", "k2 board", "anytls", "v2ray", "xray", "hysteria", "hysteria2",
	"shadowsocks", "trojan", "wireguard", "openvpn", "clash", "sing-box", "singbox",
	"vpn", "proxy", "ssr", "vmess", "vless", "trojan-go",
}

var giftcardSubjectSensitiveCN = []string{
	"机场", "科学上网", "翻墙", "翻牆", "代理节点", "节点订阅", "订阅链接",
	"加速器", "跨境加速", "网络加速", "网路加速", "梯子", "魔法上网",
	"过墙", "抗墙", "中转节点", "专线机场", "机场套餐", "流量套餐",
	"代理服务", "代理套餐", "翻墙套餐", "VPN", "vpn",
}

func isMostlySensitiveLeftover(s string) bool {
	// If after scrub only digits/punctuation remain, treat as empty.
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= 0x4e00 && r <= 0x9fff) {
			return false
		}
	}
	return true
}

// doSigned signs with B.8.1: SignMD5({app_id,timestamp,nonce,body_sha256}).
// rawBody nil → empty body (GET).
func (g GiftCardGateway) doSigned(ctx context.Context, cfg giftcardConfig, method, fullURL string, rawBody []byte) ([]byte, int, error) {
	if rawBody == nil {
		rawBody = []byte{}
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce, err := randomHex(16)
	if err != nil {
		return nil, 0, fmt.Errorf("giftcard: nonce: %w", err)
	}
	sum := sha256.Sum256(rawBody)
	bodySHA := hex.EncodeToString(sum[:])
	sig := payment.SignMD5(map[string]string{
		"app_id":      cfg.AppID,
		"timestamp":   ts,
		"nonce":       nonce,
		"body_sha256": bodySHA,
	}, cfg.APISecret)

	var bodyReader io.Reader
	if len(rawBody) > 0 {
		bodyReader = bytes.NewReader(rawBody)
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	if len(rawBody) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-App-Id", cfg.AppID)
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Signature", sig)

	resp, err := g.client(cfg.TimeoutSec).Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("giftcard: request failed: %w（请确认发卡平台 base_url 可达）", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("giftcard: read body: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

func randomHex(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func init() {
	payment.Register(GiftCardGateway{})
}

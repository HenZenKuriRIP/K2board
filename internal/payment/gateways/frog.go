package gateways

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"K2board/internal/models"
	"K2board/internal/payment"
)

// FrogGateway integrates 青蛙系统四方 API (JSON + MD5).
// Official endpoints (ops):
//
//	POST {base}/rest/pay/create
//	POST {base}/rest/pay/query
//	POST {base}/rest/pay/balance
//
// base_url 示例: https://pay.pp.qwgua.com
// 默认自动补 path 前缀 /rest；若网关无 /rest，配置 "path_prefix":"none"。
//
// Config JSON:
//
//	{
//	  "base_url": "https://pay.pp.qwgua.com",
//	  "path_prefix": "rest",
//	  "mch_id": "8888888888",
//	  "key": "merchant_secret",
//	  "code": "1000",
//	  "product_name": "数字商品",
//	  "device": "web"
//	}
//
// Sign: filter empty + exclude sign → sort keys → k=v&... + key → MD5 lowercase.
type FrogGateway struct {
	HTTPClient *http.Client
}

func (FrogGateway) Code() string { return "frog" }
func (FrogGateway) Name() string { return "青蛙四方" }

type frogConfig struct {
	BaseURL     string `json:"base_url"`
	PathPrefix  string `json:"path_prefix"` // default "rest"; "none" = no prefix
	MchID       string `json:"mch_id"`
	Key         string `json:"key"`
	Code        string `json:"code"`         // pay channel code (1–4 chars)
	ProductName string `json:"product_name"` // title template
	Device      string `json:"device"`       // optional, max 10
	// aliases
	MchId    string `json:"mchId"`
	Secret   string `json:"secret"`
	APIToken string `json:"api_token"`
	Channel  string `json:"channel"`
}

func parseFrogConfig(cfg string) (frogConfig, error) {
	var c frogConfig
	if strings.TrimSpace(cfg) == "" {
		cfg = "{}"
	}
	if err := json.Unmarshal([]byte(cfg), &c); err != nil {
		return c, fmt.Errorf("frog: invalid config JSON: %w", err)
	}
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	// 运营若粘贴了完整接口 URL，剥到网关根
	for _, suf := range []string{
		"/rest/pay/create", "/rest/pay/query", "/rest/pay/balance",
		"/pay/create", "/pay/query", "/pay/balance",
	} {
		if strings.HasSuffix(c.BaseURL, suf) {
			c.BaseURL = strings.TrimSuffix(c.BaseURL, suf)
			c.BaseURL = strings.TrimRight(c.BaseURL, "/")
			break
		}
	}
	c.PathPrefix = strings.Trim(strings.TrimSpace(c.PathPrefix), "/")
	c.MchID = strings.TrimSpace(c.MchID)
	if c.MchID == "" {
		c.MchID = strings.TrimSpace(c.MchId)
	}
	c.Key = strings.TrimSpace(c.Key)
	if c.Key == "" {
		c.Key = strings.TrimSpace(c.Secret)
	}
	if c.Key == "" {
		c.Key = strings.TrimSpace(c.APIToken)
	}
	c.Code = strings.TrimSpace(c.Code)
	if c.Code == "" {
		c.Code = strings.TrimSpace(c.Channel)
	}
	c.ProductName = strings.TrimSpace(c.ProductName)
	if c.ProductName == "" {
		c.ProductName = "数字商品"
	}
	c.Device = strings.TrimSpace(c.Device)
	if c.Device == "" {
		c.Device = "web"
	}
	if utf8.RuneCountInString(c.Device) > 10 {
		c.Device = string([]rune(c.Device)[:10])
	}
	return c, nil
}

// apiURL builds e.g. https://pay.pp.qwgua.com/rest/pay/create
// path_prefix 默认 rest；base 已以 /rest 结尾时不再重复；path_prefix=none 则 {base}/pay/...
func (c frogConfig) apiURL(payPath string) string {
	payPath = strings.Trim(payPath, "/")
	base := strings.TrimRight(c.BaseURL, "/")
	prefix := c.PathPrefix
	if prefix == "" {
		prefix = "rest" // 青蛙正式网关默认
	}
	switch strings.ToLower(prefix) {
	case "none", "-", "off", "false":
		return base + "/" + payPath
	}
	// base 已是 .../rest 则不再拼前缀
	if strings.HasSuffix(strings.ToLower(base), "/"+strings.ToLower(prefix)) {
		return base + "/" + payPath
	}
	return base + "/" + prefix + "/" + payPath
}

func (c frogConfig) validate() error {
	if c.BaseURL == "" {
		return fmt.Errorf("frog: base_url 未配置（运营提供的网关地址）")
	}
	if c.MchID == "" {
		return fmt.Errorf("frog: mch_id 未配置")
	}
	if c.Key == "" {
		return fmt.Errorf("frog: key 未配置（商户密钥）")
	}
	if c.Code == "" {
		return fmt.Errorf("frog: code 未配置（支付通道编码，向运营获取）")
	}
	if len(c.Code) > 4 {
		return fmt.Errorf("frog: code 最长 4 字符")
	}
	return nil
}

func (g FrogGateway) client() *http.Client {
	if g.HTTPClient != nil {
		return g.HTTPClient
	}
	return &http.Client{Timeout: 25 * time.Second}
}

// frogSign: filter empty & sign → sort by key → k=v&... + secretKey → MD5 lower.
func frogSign(params map[string]string, secretKey string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "sign" || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	raw := strings.Join(parts, "&") + secretKey
	sum := md5.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func frogRequestTime() string {
	// 文档：yyyyMMddHHmmss — 使用本地时区（部署 Asia/Shanghai 时与运营一致）
	return time.Now().Format("20060102150405")
}

func frogTitle(tmpl string, order *models.Order) string {
	name := tmpl
	trade, plan := "", ""
	if order != nil {
		trade = order.TradeNo
		plan = order.PlanName
	}
	tail := trade
	if len(tail) > 6 {
		tail = tail[len(tail)-6:]
	}
	name = strings.ReplaceAll(name, "{trade_no}", trade)
	name = strings.ReplaceAll(name, "{trade_tail}", tail)
	name = strings.ReplaceAll(name, "{plan_name}", plan)
	name = strings.Map(func(r rune) rune {
		if r < 32 {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" {
		name = "数字商品"
	}
	if utf8.RuneCountInString(name) > 128 {
		name = string([]rune(name)[:128])
	}
	return name
}

func frogClientIP(opts payment.CreateOptions) string {
	ip := strings.TrimSpace(opts.ClientIP)
	if ip == "" {
		return "127.0.0.1"
	}
	// strip port / brackets if present
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	ip = strings.Trim(ip, "[]")
	if ip == "" {
		return "127.0.0.1"
	}
	if utf8.RuneCountInString(ip) > 68 {
		ip = string([]rune(ip)[:68])
	}
	return ip
}

func (g FrogGateway) CreatePayment(ctx context.Context, order *models.Order, cfgJSON string, opts payment.CreateOptions) (*payment.PaymentIntent, error) {
	cfg, err := parseFrogConfig(cfgJSON)
	if err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if opts.NotifyURL == "" {
		return nil, fmt.Errorf("frog: 缺少 notify_url（请在系统设置填写 site_url）")
	}

	amount := payment.FormatFiatFromCents(order.TotalAmount)
	reqTime := frogRequestTime()
	title := frogTitle(cfg.ProductName, order)
	ip := frogClientIP(opts)

	// All values as strings for signing (doc examples use string forms)
	signParams := map[string]string{
		"mchId":       cfg.MchID,
		"version":     "1.0",
		"requestTime": reqTime,
		"code":        cfg.Code,
		"amount":      amount,
		"mchOrderNo":  order.TradeNo,
		"notifyUrl":   opts.NotifyURL,
		"title":       title,
		"ip":          ip,
		"device":      cfg.Device,
	}
	if body := strings.TrimSpace(order.PlanName); body != "" {
		if utf8.RuneCountInString(body) > 255 {
			body = string([]rune(body)[:255])
		}
		signParams["body"] = body
	}
	if opts.RedirectURL != "" {
		signParams["returnUrl"] = opts.RedirectURL
	}
	signParams["sign"] = frogSign(signParams, cfg.Key)

	// JSON body: mchId as number when possible (doc type Long)
	bodyMap := make(map[string]any, len(signParams))
	for k, v := range signParams {
		if k == "mchId" {
			if n, e := strconv.ParseInt(v, 10, 64); e == nil {
				bodyMap[k] = n
				continue
			}
		}
		bodyMap[k] = v
	}
	payload, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, err
	}

	urlCreate := cfg.apiURL("pay/create")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlCreate, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := g.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("frog create failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var envelope struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, fmt.Errorf("frog create invalid JSON: %s", truncate(string(respBody), 240))
	}
	if envelope.Code != 200 {
		msg := strings.TrimSpace(envelope.Msg)
		if msg == "" {
			msg = truncate(string(respBody), 200)
		}
		return nil, fmt.Errorf("frog create: %s (code=%d)", msg, envelope.Code)
	}

	dataParams, err := frogJSONToStringMap(envelope.Data)
	if err != nil {
		return nil, fmt.Errorf("frog create: bad data: %w", err)
	}
	// Verify response sign when present
	if got := strings.TrimSpace(dataParams["sign"]); got != "" {
		expect := frogSign(dataParams, cfg.Key)
		if !strings.EqualFold(expect, got) {
			return nil, fmt.Errorf("frog create: response sign mismatch")
		}
	}
	payURL := strings.TrimSpace(dataParams["payUrl"])
	if payURL == "" {
		return nil, fmt.Errorf("frog create: empty payUrl")
	}
	sysNo := strings.TrimSpace(dataParams["sysOrderNo"])

	return &payment.PaymentIntent{
		Type:     payment.IntentRedirect,
		URL:      payURL,
		TradeNo:  order.TradeNo,
		Amount:   order.TotalAmount,
		Currency: order.Currency,
		Message:  "将跳转青蛙收银台完成付款",
		ExpireAt: order.ExpiredAt.Unix(),
		Extra: map[string]any{
			"payment_url":  payURL,
			"gateway":      "frog",
			"sys_order_no": sysNo,
			"channel_code": cfg.Code,
			"amount":       amount,
		},
	}, nil
}

func frogJSONToStringMap(raw json.RawMessage) (map[string]string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			out[k] = t
		case float64:
			// avoid 1.0e+10 style for integers used in sign
			if t == float64(int64(t)) {
				out[k] = strconv.FormatInt(int64(t), 10)
			} else {
				out[k] = strconv.FormatFloat(t, 'f', -1, 64)
			}
		case json.Number:
			out[k] = t.String()
		case bool:
			out[k] = strconv.FormatBool(t)
		default:
			out[k] = strings.TrimSpace(fmt.Sprint(v))
		}
	}
	return out, nil
}

// HandleNotify: POST application/json, only success notifies; ACK body "success".
func (g FrogGateway) HandleNotify(_ context.Context, _ map[string]string, body []byte, cfgJSON string) (*payment.NotifyResult, error) {
	cfg, err := parseFrogConfig(cfgJSON)
	if err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	params, err := frogParseNotifyBody(body)
	if err != nil {
		return nil, err
	}
	gotSign := strings.TrimSpace(params["sign"])
	if gotSign == "" {
		return nil, fmt.Errorf("frog: missing sign")
	}
	expect := frogSign(params, cfg.Key)
	if !strings.EqualFold(expect, gotSign) {
		return nil, fmt.Errorf("frog: invalid signature")
	}
	if mid := strings.TrimSpace(params["mchId"]); mid != "" && mid != cfg.MchID {
		return nil, fmt.Errorf("frog: mchId mismatch")
	}
	tradeNo := strings.TrimSpace(params["mchOrderNo"])
	if tradeNo == "" {
		return nil, fmt.Errorf("frog: missing mchOrderNo")
	}
	// status 3 = 支付完成
	st := strings.TrimSpace(params["status"])
	success := st == "3"
	paidCents := int64(0)
	if a := strings.TrimSpace(params["amount"]); a != "" {
		f, e := strconv.ParseFloat(a, 64)
		if e != nil {
			return nil, fmt.Errorf("frog: invalid amount %q", a)
		}
		paidCents = payment.FiatToCents(f)
	}
	cb := strings.TrimSpace(params["sysOrderNo"])
	if cb == "" {
		cb = tradeNo
	}
	return &payment.NotifyResult{
		TradeNo:    tradeNo,
		PaidAmount: paidCents,
		Success:    success,
		CallbackNo: cb,
		Raw:        string(body),
	}, nil
}

func frogParseNotifyBody(body []byte) (map[string]string, error) {
	raw := bytes.TrimSpace(body)
	if len(raw) == 0 {
		return nil, fmt.Errorf("frog: empty notify body")
	}
	// Prefer JSON (documented)
	if raw[0] == '{' {
		return frogJSONToStringMap(raw)
	}
	// Fallback form / query (defensive)
	vals, err := url.ParseQuery(string(raw))
	if err != nil || len(vals) == 0 {
		return nil, fmt.Errorf("frog: invalid notify body")
	}
	out := make(map[string]string, len(vals))
	for k, vs := range vals {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	return out, nil
}

// QueryPayment POST /pay/query — status 3 = paid.
func (g FrogGateway) QueryPayment(ctx context.Context, order *models.Order, cfgJSON string) (*payment.QueryResult, error) {
	cfg, err := parseFrogConfig(cfgJSON)
	if err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	reqTime := frogRequestTime()
	signParams := map[string]string{
		"mchId":       cfg.MchID,
		"version":     "1.0",
		"requestTime": reqTime,
		"mchOrderNo":  order.TradeNo,
	}
	signParams["sign"] = frogSign(signParams, cfg.Key)

	bodyMap := map[string]any{
		"version":     "1.0",
		"requestTime": reqTime,
		"mchOrderNo":  order.TradeNo,
		"sign":        signParams["sign"],
	}
	if n, e := strconv.ParseInt(cfg.MchID, 10, 64); e == nil {
		bodyMap["mchId"] = n
	} else {
		bodyMap["mchId"] = cfg.MchID
	}
	payload, _ := json.Marshal(bodyMap)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.apiURL("pay/query"), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("frog query failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var envelope struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, fmt.Errorf("frog query invalid JSON: %s", truncate(string(respBody), 200))
	}
	if envelope.Code != 200 {
		return &payment.QueryResult{Paid: false}, nil
	}
	dataParams, err := frogJSONToStringMap(envelope.Data)
	if err != nil {
		return &payment.QueryResult{Paid: false}, nil
	}
	if got := strings.TrimSpace(dataParams["sign"]); got != "" {
		if !strings.EqualFold(frogSign(dataParams, cfg.Key), got) {
			return nil, fmt.Errorf("frog query: response sign mismatch")
		}
	}
	if strings.TrimSpace(dataParams["status"]) != "3" {
		return &payment.QueryResult{Paid: false}, nil
	}
	paid := order.TotalAmount
	if a := strings.TrimSpace(dataParams["amount"]); a != "" {
		if f, e := strconv.ParseFloat(a, 64); e == nil {
			paid = payment.FiatToCents(f)
		}
	}
	cb := strings.TrimSpace(dataParams["sysOrderNo"])
	if cb == "" {
		cb = order.CallbackNo
	}
	return &payment.QueryResult{
		Paid:       true,
		PaidAmount: paid,
		CallbackNo: cb,
	}, nil
}

func init() {
	payment.Register(FrogGateway{})
}

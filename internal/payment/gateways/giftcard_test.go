package gateways

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"K2board/internal/models"
	"K2board/internal/payment"
)

const (
	testGCSecret = "test_secret"
	testGCAppID  = "k2-main"
)

func TestAssertSafeGiftcardBaseURL(t *testing.T) {
	if err := assertSafeGiftcardBaseURL("https://pay.example.com"); err != nil {
		// DNS may fail offline — only require shape; if DNS works and resolves public, ok
		t.Log(err)
	}
	if err := assertSafeGiftcardBaseURL("ftp://evil.com"); err == nil {
		t.Fatal("ftp must fail")
	}
	if err := assertSafeGiftcardBaseURL("http://169.254.169.254/latest"); err == nil {
		t.Fatal("metadata IP must fail")
	}
	if err := assertSafeGiftcardBaseURL("http://user:pass@evil.com"); err == nil {
		t.Fatal("userinfo must fail")
	}
	// httptest / local loopback allowed (self-hosted giftcard next to panel)
	if err := assertSafeGiftcardBaseURL("http://127.0.0.1:8089"); err != nil {
		t.Fatalf("loopback self-host should be allowed: %v", err)
	}
}

func TestGiftcard_AppendixC_GoldenVectors(t *testing.T) {
	// C.1 GET empty body
	emptySHA := sha256Hex(nil)
	if emptySHA != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatalf("empty sha256=%s", emptySHA)
	}
	sig1 := payment.SignMD5(map[string]string{
		"app_id": testGCAppID, "timestamp": "1720770000",
		"nonce": "n1n2n3n4n5n6n7n8", "body_sha256": emptySHA,
	}, testGCSecret)
	if sig1 != "15906004c50c79f16dca9d067124e4c3" {
		t.Fatalf("C.1 signature=%s", sig1)
	}

	// C.2 POST create
	createBody := []byte(`{"out_trade_no":"T1","amount":100,"currency":"CNY","subject":"VIP","notify_url":"https://panel.example.com/api/v1/payment/notify/giftcard","return_url":"https://panel.example.com/#/user/order-result?trade_no=T1"}`)
	createSHA := sha256Hex(createBody)
	if createSHA != "c4f97996d3633582e36ae6f438e9c563b77cc3c265bad47f868a8b9ddad12a85" {
		t.Fatalf("C.2 body_sha256=%s", createSHA)
	}
	sig2 := payment.SignMD5(map[string]string{
		"app_id": testGCAppID, "timestamp": "1720770000",
		"nonce": "n1n2n3n4n5n6n7n8", "body_sha256": createSHA,
	}, testGCSecret)
	if sig2 != "0d39b855a6a1e46bba9f78e5839fd15d" {
		t.Fatalf("C.2 signature=%s", sig2)
	}

	// C.3 POST close
	closeBody := []byte(`{"reason":"k2_cancel"}`)
	closeSHA := sha256Hex(closeBody)
	if closeSHA != "02222ae436a62053014c0453dd2a871c481c5c3668e44e8adcd14b43d1b86dc3" {
		t.Fatalf("C.3 body_sha256=%s", closeSHA)
	}
	sig3 := payment.SignMD5(map[string]string{
		"app_id": testGCAppID, "timestamp": "1720770000",
		"nonce": "n1n2n3n4n5n6n7n8", "body_sha256": closeSHA,
	}, testGCSecret)
	if sig3 != "a97ed3842d6bc2906bd0c4228315c28d" {
		t.Fatalf("C.3 signature=%s", sig3)
	}

	// C.4 notify body signature
	sig4 := payment.SignMD5(map[string]string{
		"app_id":            testGCAppID,
		"out_trade_no":      "T1",
		"platform_trade_no": "GC1",
		"amount":            "100",
		"paid_amount":       "100",
		"currency":          "CNY",
		"status":            "paid",
		"alipay_trade_no":   "ALI1",
		"paid_at":           "1720770000",
		"timestamp":         "1720770001",
		"nonce":             "notifynonce0001",
	}, testGCSecret)
	if sig4 != "f0acd306f758fc3062a40effee6b99b8" {
		t.Fatalf("C.4 signature=%s", sig4)
	}
}

func TestGiftcard_CreatePayment_MockServer(t *testing.T) {
	var sawAmount any
	var sawCentsOK bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/orders" {
			http.NotFound(w, r)
			return
		}
		if err := verifyMerchantHeaders(r, testGCSecret, testGCAppID); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		sawAmount = body["amount"]
		// Must be JSON number integer cents, not 28.88
		switch v := sawAmount.(type) {
		case float64:
			if v == 2888 {
				sawCentsOK = true
			}
		case json.Number:
			if v.String() == "2888" {
				sawCentsOK = true
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    0,
			"message": "ok",
			"data": map[string]any{
				"out_trade_no":      "K2T1",
				"platform_trade_no": "GC999",
				"cashier_url":       "https://pay.example.com/c/tok123",
				"cashier_token":     "tok123",
				"status":            "pending",
				"amount":            2888,
				"expire_at":         time.Now().Add(30 * time.Minute).Unix(),
			},
		})
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(map[string]any{
		"base_url": srv.URL, "app_id": testGCAppID, "api_secret": testGCSecret,
	})
	g := GiftCardGateway{}
	order := &models.Order{
		TradeNo: "K2T1", UserID: 42, PlanName: "VIP 月付",
		TotalAmount: 2888, Currency: "CNY", ExpiredAt: time.Now().Add(30 * time.Minute),
	}
	intent, err := g.CreatePayment(context.Background(), order, string(cfg), payment.CreateOptions{
		NotifyURL: "https://panel.example.com/api/v1/payment/notify/giftcard",
		RedirectURL: "https://panel.example.com/#/user/order-result?trade_no=K2T1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if intent.Type != payment.IntentRedirect || intent.URL != "https://pay.example.com/c/tok123" {
		t.Fatalf("intent=%+v", intent)
	}
	if intent.Amount != 2888 {
		t.Fatalf("intent amount must stay cents: %d", intent.Amount)
	}
	if !sawCentsOK {
		t.Fatalf("platform saw amount=%v (want integer cents 2888)", sawAmount)
	}
	extra, _ := intent.Extra.(map[string]any)
	if extra["platform_trade_no"] != "GC999" {
		t.Fatalf("extra=%v", extra)
	}
}

func TestGiftcard_CreatePayment_AlreadyPaid_40901(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    40901,
			"message": "already paid",
			"data":    map[string]any{"status": "paid", "paid_amount": 100, "out_trade_no": "T1"},
		})
	}))
	defer srv.Close()
	cfg := cfgJSON(srv.URL)
	g := GiftCardGateway{}
	_, err := g.CreatePayment(context.Background(), sampleOrder("T1", 100), cfg, sampleOpts())
	if err == nil || !strings.Contains(err.Error(), "giftcard: already_paid:") {
		t.Fatalf("want already_paid prefix, got %v", err)
	}
}

func TestGiftcard_CreatePayment_MissingConfig(t *testing.T) {
	var g GiftCardGateway
	_, err := g.CreatePayment(context.Background(), sampleOrder("T1", 100), `{}`, sampleOpts())
	if err == nil {
		t.Fatal("expected config error")
	}
}

func TestGiftcard_HandleNotify_Success(t *testing.T) {
	body := signedNotifyBody(t, map[string]any{
		"app_id": testGCAppID, "out_trade_no": "T1", "platform_trade_no": "GC1",
		"amount": int64(100), "paid_amount": int64(100), "currency": "CNY",
		"status": "paid", "alipay_trade_no": "ALI1",
		"paid_at": int64(1720770000), "timestamp": int64(1720770001),
		"nonce": "notifynonce0001",
	})
	cfg := `{"app_id":"k2-main","api_secret":"test_secret","base_url":"https://x"}`
	var g GiftCardGateway
	res, err := g.HandleNotify(context.Background(), nil, body, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.TradeNo != "T1" || res.PaidAmount != 100 {
		t.Fatalf("%+v", res)
	}
	if res.CallbackNo != "ALI1" {
		t.Fatalf("callback=%s", res.CallbackNo)
	}
}

func TestGiftcard_HandleNotify_AppendixC4(t *testing.T) {
	// Exact appendix C.4 field set
	params := map[string]string{
		"app_id": testGCAppID, "out_trade_no": "T1", "platform_trade_no": "GC1",
		"amount": "100", "paid_amount": "100", "currency": "CNY", "status": "paid",
		"alipay_trade_no": "ALI1", "paid_at": "1720770000", "timestamp": "1720770001",
		"nonce": "notifynonce0001",
	}
	sig := payment.SignMD5(params, testGCSecret)
	body, _ := json.Marshal(map[string]any{
		"app_id": testGCAppID, "out_trade_no": "T1", "platform_trade_no": "GC1",
		"amount": 100, "paid_amount": 100, "currency": "CNY", "status": "paid",
		"alipay_trade_no": "ALI1", "paid_at": 1720770000, "timestamp": 1720770001,
		"nonce": "notifynonce0001", "signature": sig,
	})
	var g GiftCardGateway
	res, err := g.HandleNotify(context.Background(), nil, body, `{"app_id":"k2-main","api_secret":"test_secret"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.PaidAmount != 100 {
		t.Fatalf("%+v", res)
	}
}

func TestGiftcard_HandleNotify_BadSignature(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"app_id": testGCAppID, "out_trade_no": "T1", "status": "paid",
		"paid_amount": 100, "amount": 100, "signature": "deadbeef",
		"timestamp": 1, "nonce": "n",
	})
	var g GiftCardGateway
	_, err := g.HandleNotify(context.Background(), nil, body, `{"app_id":"k2-main","api_secret":"test_secret"}`)
	if err == nil {
		t.Fatal("expected bad signature")
	}
}

func TestGiftcard_HandleNotify_AppIDMismatch(t *testing.T) {
	body := signedNotifyBody(t, map[string]any{
		"app_id": "other-app", "out_trade_no": "T1", "platform_trade_no": "GC1",
		"amount": int64(100), "paid_amount": int64(100), "currency": "CNY",
		"status": "paid", "paid_at": int64(1), "timestamp": int64(2), "nonce": "nn",
	})
	// Sign with secret but app_id in body is other-app; config expects k2-main
	var g GiftCardGateway
	_, err := g.HandleNotify(context.Background(), nil, body, `{"app_id":"k2-main","api_secret":"test_secret"}`)
	if err == nil || !strings.Contains(err.Error(), "app_id mismatch") {
		t.Fatalf("want app_id mismatch, got %v", err)
	}
}

func TestGiftcard_HandleNotify_RejectFloatAmount(t *testing.T) {
	// encoding/json cannot unmarshal 28.88 into int64
	raw := []byte(`{"app_id":"k2-main","out_trade_no":"T1","amount":28.88,"paid_amount":28.88,"status":"paid","signature":"x","timestamp":1,"nonce":"n"}`)
	var g GiftCardGateway
	_, err := g.HandleNotify(context.Background(), nil, raw, `{"app_id":"k2-main","api_secret":"test_secret"}`)
	if err == nil {
		t.Fatal("float amount must fail decode")
	}
}

func TestGiftcard_HandleNotify_NonPaidStatus(t *testing.T) {
	body := signedNotifyBody(t, map[string]any{
		"app_id": testGCAppID, "out_trade_no": "T1", "status": "pending",
		"amount": int64(100), "paid_amount": int64(0), "currency": "CNY",
		"paid_at": int64(0), "timestamp": int64(1), "nonce": "n1",
	})
	var g GiftCardGateway
	res, err := g.HandleNotify(context.Background(), nil, body, `{"app_id":"k2-main","api_secret":"test_secret"}`)
	if err != nil {
		t.Fatal(err)
	}
	if res.Success {
		t.Fatal("pending must not be success")
	}
}

func TestGiftcard_QueryPayment_Paid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s", r.Method)
		}
		if err := verifyMerchantHeaders(r, testGCSecret, testGCAppID); err != nil {
			http.Error(w, err.Error(), 401)
			return
		}
		// Empty body signature path
		raw, _ := io.ReadAll(r.Body)
		if len(raw) != 0 {
			t.Errorf("GET must have empty body, got %d bytes", len(raw))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"status": "paid", "paid_amount": 2888,
				"alipay_trade_no": "ALI9", "platform_trade_no": "GC9",
			},
		})
	}))
	defer srv.Close()
	g := GiftCardGateway{}
	qr, err := g.QueryPayment(context.Background(), sampleOrder("T1", 2888), cfgJSON(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if !qr.Paid || qr.PaidAmount != 2888 || qr.CallbackNo != "ALI9" {
		t.Fatalf("%+v", qr)
	}
}

func TestGiftcard_QueryPayment_40401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 40401, "message": "not found"})
	}))
	defer srv.Close()
	g := GiftCardGateway{}
	qr, err := g.QueryPayment(context.Background(), sampleOrder("missing", 100), cfgJSON(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if qr.Paid {
		t.Fatal("40401 must be Paid:false")
	}
}

func TestGiftcard_QueryPayment_PaidOrphan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{"status": "paid_orphan", "paid_amount": 100, "platform_trade_no": "GC1"},
		})
	}))
	defer srv.Close()
	g := GiftCardGateway{}
	qr, err := g.QueryPayment(context.Background(), sampleOrder("T1", 100), cfgJSON(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if !qr.Paid {
		t.Fatal("paid_orphan must map to Paid=true for reconcile")
	}
}

func TestGiftcard_ClosePayment_SoftSuccess(t *testing.T) {
	for _, code := range []int{0, 40401, 40901, 40902} {
		code := code
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/close") {
					http.NotFound(w, r)
					return
				}
				if err := verifyMerchantHeaders(r, testGCSecret, testGCAppID); err != nil {
					http.Error(w, err.Error(), 401)
					return
				}
				raw, _ := io.ReadAll(r.Body)
				if string(raw) != `{"reason":"k2_cancel"}` {
					t.Errorf("close body=%s", raw)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": "x"})
			}))
			defer srv.Close()
			g := GiftCardGateway{}
			if err := g.ClosePayment(context.Background(), sampleOrder("T1", 100), cfgJSON(srv.URL)); err != nil {
				t.Fatalf("code %d: %v", code, err)
			}
		})
	}
}

func TestGiftcard_ClosePayment_HardError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 50000, "message": "boom"})
	}))
	defer srv.Close()
	g := GiftCardGateway{}
	if err := g.ClosePayment(context.Background(), sampleOrder("T1", 100), cfgJSON(srv.URL)); err == nil {
		t.Fatal("expected error")
	}
}

func TestGiftcard_Registered(t *testing.T) {
	g, ok := payment.Get("giftcard")
	if !ok || g.Code() != "giftcard" {
		t.Fatal("giftcard gateway not registered")
	}
	if _, ok := payment.AsCloser(g); !ok {
		t.Fatal("giftcard must implement Closer")
	}
}

func TestGiftcard_SubjectTruncate(t *testing.T) {
	long := strings.Repeat("套", 200)
	order := &models.Order{PlanName: long, TradeNo: "T"}
	s := giftcardSubject("{plan_name}", order)
	if len([]rune(s)) != 128 {
		t.Fatalf("runes=%d", len([]rune(s)))
	}
}

func TestGiftcard_SubjectDefaultNeutral(t *testing.T) {
	order := &models.Order{PlanName: "机场年付VPN套餐", TradeNo: "K220260712ABCDEF"}
	// empty tmpl uses parse default path via giftcardSubject("数字商品", ...)
	s := giftcardSubject("数字商品", order)
	if s != "数字商品" {
		t.Fatalf("got %q", s)
	}
	// plan_name template still scrubbed
	s2 := giftcardSubject("{plan_name}", order)
	if strings.Contains(s2, "机场") || strings.Contains(strings.ToLower(s2), "vpn") {
		t.Fatalf("sensitive left: %q", s2)
	}
	if s2 == "" {
		t.Fatal("empty subject")
	}
	// trade_tail = last 6 of trade no
	s3 := giftcardSubject("数字商品-{trade_tail}", order)
	if s3 != "数字商品-ABCDEF" {
		t.Fatalf("trade_tail got %q", s3)
	}
}

func TestGiftcard_SubjectScrubSensitive(t *testing.T) {
	order := &models.Order{PlanName: "x", TradeNo: "T1"}
	s := giftcardSubject("K2Board 机场 VPN 月付", order)
	if strings.Contains(strings.ToLower(s), "vpn") || strings.Contains(s, "机场") || strings.Contains(strings.ToLower(s), "k2board") {
		t.Fatalf("not scrubbed: %q", s)
	}
}

// --- helpers ---

func sha256Hex(b []byte) string {
	if b == nil {
		b = []byte{}
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func cfgJSON(base string) string {
	b, _ := json.Marshal(map[string]any{
		"base_url": base, "app_id": testGCAppID, "api_secret": testGCSecret,
	})
	return string(b)
}

func sampleOrder(trade string, cents int64) *models.Order {
	return &models.Order{
		TradeNo: trade, UserID: 1, PlanName: "VIP",
		TotalAmount: cents, Currency: "CNY",
		ExpiredAt: time.Now().Add(30 * time.Minute),
	}
}

func sampleOpts() payment.CreateOptions {
	return payment.CreateOptions{
		NotifyURL:   "https://panel.example.com/api/v1/payment/notify/giftcard",
		RedirectURL: "https://panel.example.com/#/user/order-result?trade_no=T1",
	}
}

func signedNotifyBody(t *testing.T, fields map[string]any) []byte {
	t.Helper()
	params := map[string]string{}
	for k, v := range fields {
		switch x := v.(type) {
		case string:
			params[k] = x
		case int64:
			params[k] = strconv.FormatInt(x, 10)
		case int:
			params[k] = strconv.Itoa(x)
		default:
			t.Fatalf("unsupported field type %T for %s", v, k)
		}
	}
	sig := payment.SignMD5(params, testGCSecret)
	out := map[string]any{}
	for k, v := range fields {
		out[k] = v
	}
	out["signature"] = sig
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func verifyMerchantHeaders(r *http.Request, secret, appID string) error {
	gotApp := r.Header.Get("X-App-Id")
	ts := r.Header.Get("X-Timestamp")
	nonce := r.Header.Get("X-Nonce")
	sig := r.Header.Get("X-Signature")
	if gotApp != appID {
		return errStr("app_id")
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	// restore body for caller
	r.Body = io.NopCloser(strings.NewReader(string(raw)))
	sum := sha256.Sum256(raw)
	expect := payment.SignMD5(map[string]string{
		"app_id": appID, "timestamp": ts, "nonce": nonce,
		"body_sha256": hex.EncodeToString(sum[:]),
	}, secret)
	if !strings.EqualFold(expect, sig) {
		return errStr("bad signature")
	}
	return nil
}

type errStr string

func (e errStr) Error() string { return string(e) }

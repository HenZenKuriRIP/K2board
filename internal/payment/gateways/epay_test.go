package gateways

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"K2board/internal/models"
	"K2board/internal/payment"
)

func TestEpaySign_KnownVector(t *testing.T) {
	params := map[string]string{
		"money":        "1.00",
		"name":         "VIP会员",
		"notify_url":   "http://www.pay.com/notify_url.php",
		"out_trade_no": "20160806151343349",
		"pid":          "1001",
		"return_url":   "http://www.pay.com/return_url.php",
		"type":         "alipay",
		"sign_type":    "MD5",
		"sign":         "should-ignore",
	}
	key := "testkey"
	raw := "money=1.00&name=VIP会员&notify_url=http://www.pay.com/notify_url.php&out_trade_no=20160806151343349&pid=1001&return_url=http://www.pay.com/return_url.php&type=alipay" + key
	sum := md5.Sum([]byte(raw))
	want := hex.EncodeToString(sum[:])
	got := epaySign(params, key)
	if got != want {
		t.Fatalf("epaySign:\n got %s\nwant %s", got, want)
	}
	params2 := map[string]string{
		"money": "1.00", "name": "VIP会员",
		"notify_url": "http://www.pay.com/notify_url.php",
		"out_trade_no": "20160806151343349", "pid": "1001",
		"return_url": "http://www.pay.com/return_url.php", "type": "alipay",
	}
	if epaySign(params2, key) != got {
		t.Fatal("sign/sign_type/empty must not affect signature")
	}
}

func TestEpay_CreatePayment_SubmitURL(t *testing.T) {
	gw := EpayGateway{}
	order := &models.Order{
		TradeNo:     "TN202601010001",
		PlanName:    "VIP月付",
		TotalAmount: 2888,
		Currency:    "CNY",
		ExpiredAt:   time.Now().Add(30 * time.Minute),
	}
	cfg := `{"base_url":"https://pay.example.com","pid":"1001","key":"ksecret","type":"alipay","product_name":"数字商品"}`
	intent, err := gw.CreatePayment(context.Background(), order, cfg, payment.CreateOptions{
		NotifyURL:   "https://panel.example/api/v1/payment/notify/epay",
		RedirectURL: "https://panel.example/#/user/order-result?trade_no=TN202601010001",
	})
	if err != nil {
		t.Fatal(err)
	}
	if intent.Type != payment.IntentRedirect {
		t.Fatalf("type=%s", intent.Type)
	}
	if !strings.HasPrefix(intent.URL, "https://pay.example.com/submit.php?") {
		t.Fatalf("url=%s", intent.URL)
	}
	u, err := url.Parse(intent.URL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("pid") != "1001" || q.Get("out_trade_no") != "TN202601010001" {
		t.Fatalf("query pid/out_trade_no: %v", q)
	}
	if q.Get("money") != "28.88" {
		t.Fatalf("money=%s", q.Get("money"))
	}
	if q.Get("type") != "alipay" {
		t.Fatalf("type=%s", q.Get("type"))
	}
	if q.Get("sign_type") != "MD5" || q.Get("sign") == "" {
		t.Fatal("missing sign")
	}
	signParams := map[string]string{}
	for k, vs := range q {
		if len(vs) > 0 {
			signParams[k] = vs[0]
		}
	}
	expect := epaySign(signParams, "ksecret")
	if !strings.EqualFold(expect, q.Get("sign")) {
		t.Fatalf("sign mismatch got=%s want=%s", q.Get("sign"), expect)
	}
	extra, _ := intent.Extra.(map[string]any)
	if extra["payment_url"] == nil || extra["payment_url"] == "" {
		t.Fatal("extra.payment_url required for reopen cashier")
	}
}

func TestEpay_CreatePayment_CashierNoType(t *testing.T) {
	gw := EpayGateway{}
	order := &models.Order{TradeNo: "T1", TotalAmount: 100, Currency: "CNY", ExpiredAt: time.Now().Add(time.Hour)}
	cfg := `{"base_url":"https://pay.example.com","pid":"1","key":"k"}`
	intent, err := gw.CreatePayment(context.Background(), order, cfg, payment.CreateOptions{
		NotifyURL: "https://x/n", RedirectURL: "https://x/r",
	})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(intent.URL)
	if u.Query().Get("type") != "" {
		t.Fatalf("type should be omitted for cashier, got %q", u.Query().Get("type"))
	}
}

func TestEpay_HandleNotify_Success(t *testing.T) {
	gw := EpayGateway{}
	cfg := `{"base_url":"https://pay.example.com","pid":"1001","key":"ksecret"}`
	params := map[string]string{
		"pid":          "1001",
		"trade_no":     "EPAY999",
		"out_trade_no": "TN001",
		"type":         "alipay",
		"name":         "数字商品",
		"money":        "28.88",
		"trade_status": "TRADE_SUCCESS",
	}
	params["sign"] = epaySign(params, "ksecret")
	params["sign_type"] = "MD5"
	body := url.Values{}
	for k, v := range params {
		body.Set(k, v)
	}
	res, err := gw.HandleNotify(context.Background(), nil, []byte(body.Encode()), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.TradeNo != "TN001" {
		t.Fatalf("%+v", res)
	}
	if res.PaidAmount != 2888 {
		t.Fatalf("paid=%d", res.PaidAmount)
	}
	if res.CallbackNo != "EPAY999" {
		t.Fatalf("cb=%s", res.CallbackNo)
	}
}

func TestEpay_HandleNotify_BadSign(t *testing.T) {
	gw := EpayGateway{}
	cfg := `{"base_url":"https://pay.example.com","pid":"1001","key":"ksecret"}`
	body := []byte("pid=1001&out_trade_no=TN001&money=1.00&trade_status=TRADE_SUCCESS&sign=deadbeef&sign_type=MD5")
	_, err := gw.HandleNotify(context.Background(), nil, body, cfg)
	if err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("err=%v", err)
	}
}

func TestEpay_HandleNotify_PidMismatch(t *testing.T) {
	gw := EpayGateway{}
	cfg := `{"base_url":"https://pay.example.com","pid":"1001","key":"ksecret"}`
	params := map[string]string{
		"pid": "9999", "out_trade_no": "TN001", "money": "1.00", "trade_status": "TRADE_SUCCESS",
	}
	params["sign"] = epaySign(params, "ksecret")
	body := url.Values{}
	for k, v := range params {
		body.Set(k, v)
	}
	_, err := gw.HandleNotify(context.Background(), nil, []byte(body.Encode()), cfg)
	if err == nil || !strings.Contains(err.Error(), "pid") {
		t.Fatalf("err=%v", err)
	}
}

func TestEpay_QueryPayment_Paid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("act") != "order" {
			t.Errorf("act=%s", r.URL.Query().Get("act"))
		}
		if r.URL.Query().Get("out_trade_no") != "TN_Q1" {
			t.Errorf("out=%s", r.URL.Query().Get("out_trade_no"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 1, "msg": "ok", "trade_no": "E1", "out_trade_no": "TN_Q1",
			"money": "10.00", "status": 1,
		})
	}))
	defer srv.Close()

	gw := EpayGateway{HTTPClient: srv.Client()}
	cfg, _ := json.Marshal(map[string]string{
		"base_url": srv.URL, "pid": "1001", "key": "k",
	})
	order := &models.Order{TradeNo: "TN_Q1", TotalAmount: 1000}
	qr, err := gw.QueryPayment(context.Background(), order, string(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if !qr.Paid || qr.PaidAmount != 1000 || qr.CallbackNo != "E1" {
		t.Fatalf("%+v", qr)
	}
}

func TestEpay_QueryPayment_Unpaid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"code":1,"status":0,"money":"10.00"}`)
	}))
	defer srv.Close()
	gw := EpayGateway{HTTPClient: srv.Client()}
	cfg := `{"base_url":"` + srv.URL + `","pid":"1","key":"k"}`
	qr, err := gw.QueryPayment(context.Background(), &models.Order{TradeNo: "X"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if qr.Paid {
		t.Fatal("expected unpaid")
	}
}

func TestEpay_Registered(t *testing.T) {
	g, ok := payment.Get("epay")
	if !ok || g.Code() != "epay" {
		t.Fatal("epay not registered")
	}
}

func TestEpayProductName(t *testing.T) {
	o := &models.Order{TradeNo: "ABCDEF123456", PlanName: "套餐A"}
	if got := epayProductName("数字商品", o); got != "数字商品" {
		t.Fatalf("%q", got)
	}
	if got := epayProductName("{plan_name}-{trade_tail}", o); got != "套餐A-123456" {
		t.Fatalf("%q", got)
	}
}

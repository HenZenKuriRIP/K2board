package gateways

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"K2board/internal/models"
	"K2board/internal/payment"
)

func TestFrogSign_DocStyle(t *testing.T) {
	// Matches doc algorithm: sort, join, append key, md5 lower
	params := map[string]string{
		"amount":      "10.00",
		"body":        "jItp",
		"code":        "1000",
		"device":      "app",
		"ip":          "192.168.4.162",
		"mchId":       "8888888888",
		"mchOrderNo":  "123456",
		"notifyUrl":   "http://google.com",
		"requestTime": "20220227053152",
		"title":       "jItp",
		"version":     "1.0",
		"sign":        "ignore-me",
	}
	key := "secretKEY"
	raw := "amount=10.00&body=jItp&code=1000&device=app&ip=192.168.4.162&mchId=8888888888&mchOrderNo=123456&notifyUrl=http://google.com&requestTime=20220227053152&title=jItp&version=1.0" + key
	sum := md5.Sum([]byte(raw))
	want := hex.EncodeToString(sum[:])
	got := frogSign(params, key)
	if got != want {
		t.Fatalf("frogSign:\n got %s\nwant %s", got, want)
	}
}

func TestFrog_CreatePayment_JSON(t *testing.T) {
	var sawBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Official ops path includes /rest prefix
		if r.URL.Path != "/rest/pay/create" {
			http.NotFound(w, r)
			return
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &sawBody)
		// Build response data with valid sign using same key
		data := map[string]string{
			"sysOrderNo":  "P1",
			"mchOrderNo":  "TN1",
			"amount":      "28.88",
			"payUrl":      "https://pay.example.com/cashier/xxx",
			"mchId":       "10086",
			"version":     "1.0",
			"requestTime": "20240101120000",
			"status":      "1",
		}
		data["sign"] = frogSign(data, "ksecret")
		// return data as object of strings (sign verify uses string map)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 200,
			"msg":  "success",
			"data": data,
		})
	}))
	defer srv.Close()

	gw := FrogGateway{HTTPClient: srv.Client()}
	order := &models.Order{
		TradeNo:     "TN1",
		PlanName:    "月付",
		TotalAmount: 2888,
		Currency:    "CNY",
		ExpiredAt:   time.Now().Add(time.Hour),
	}
	cfg := `{"base_url":"` + srv.URL + `","mch_id":"10086","key":"ksecret","code":"1000","product_name":"{plan_name}"}`
	intent, err := gw.CreatePayment(context.Background(), order, cfg, payment.CreateOptions{
		NotifyURL:   "https://panel.example/api/v1/payment/notify/frog",
		RedirectURL: "https://panel.example/#/user/order-result?trade_no=TN1",
		ClientIP:    "203.0.113.9",
	})
	if err != nil {
		t.Fatal(err)
	}
	if intent.Type != payment.IntentRedirect || intent.URL != "https://pay.example.com/cashier/xxx" {
		t.Fatalf("intent=%+v", intent)
	}
	if sawBody["mchOrderNo"] != "TN1" {
		t.Fatalf("body mchOrderNo=%v", sawBody["mchOrderNo"])
	}
	if sawBody["code"] != "1000" {
		t.Fatalf("channel code=%v", sawBody["code"])
	}
	if sawBody["ip"] != "203.0.113.9" {
		t.Fatalf("ip=%v", sawBody["ip"])
	}
	if sawBody["sign"] == nil || sawBody["sign"] == "" {
		t.Fatal("missing sign in request")
	}
}

func TestFrog_HandleNotify_Paid(t *testing.T) {
	params := map[string]string{
		"mchId":       "10086",
		"sysOrderNo":  "P9",
		"mchOrderNo":  "TN9",
		"amount":      "10.00",
		"status":      "3",
		"payTime":     "2024-01-01 12:00:00",
		"createTime":  "2024-01-01 12:00:00",
		"version":     "1.0",
		"requestTime": "20240101120000",
	}
	params["sign"] = frogSign(params, "ksecret")
	body, _ := json.Marshal(params)

	gw := FrogGateway{}
	res, err := gw.HandleNotify(context.Background(), nil, body, `{"base_url":"https://x","mch_id":"10086","key":"ksecret","code":"1"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.TradeNo != "TN9" || res.PaidAmount != 1000 || res.CallbackNo != "P9" {
		t.Fatalf("%+v", res)
	}
}

func TestFrog_HandleNotify_BadSign(t *testing.T) {
	body := []byte(`{"mchId":"10086","mchOrderNo":"TN9","amount":"10.00","status":3,"sign":"deadbeef","version":"1.0","requestTime":"1"}`)
	_, err := FrogGateway{}.HandleNotify(context.Background(), nil, body, `{"base_url":"https://x","mch_id":"10086","key":"ksecret","code":"1"}`)
	if err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("err=%v", err)
	}
}

func TestFrog_APIURL(t *testing.T) {
	c := frogConfig{BaseURL: "https://pay.pp.qwgua.com"}
	if u := c.apiURL("pay/create"); u != "https://pay.pp.qwgua.com/rest/pay/create" {
		t.Fatalf("default rest: %s", u)
	}
	c2 := frogConfig{BaseURL: "https://pay.pp.qwgua.com/rest"}
	if u := c2.apiURL("pay/query"); u != "https://pay.pp.qwgua.com/rest/pay/query" {
		t.Fatalf("base already rest: %s", u)
	}
	c3 := frogConfig{BaseURL: "https://pay.example.com", PathPrefix: "none"}
	if u := c3.apiURL("pay/create"); u != "https://pay.example.com/pay/create" {
		t.Fatalf("none prefix: %s", u)
	}
	c4, err := parseFrogConfig(`{"base_url":"https://pay.pp.qwgua.com/rest/pay/create"}`)
	if err != nil {
		t.Fatal(err)
	}
	if c4.apiURL("pay/create") != "https://pay.pp.qwgua.com/rest/pay/create" {
		t.Fatalf("strip full url: base=%s url=%s", c4.BaseURL, c4.apiURL("pay/create"))
	}
}

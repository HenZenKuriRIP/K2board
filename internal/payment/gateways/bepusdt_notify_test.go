package gateways

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"K2board/internal/models"
	"K2board/internal/payment"
)

func TestBEpusdtHandleNotify_Success(t *testing.T) {
	token := "test_api_token_xyz"
	cfg, _ := json.Marshal(map[string]string{
		"base_url":  "https://pay.example.com",
		"api_token": token,
	})

	reParams := map[string]string{
		"trade_id":             "tid-1",
		"order_id":             "K2ORDER1",
		"amount":               anyToAmountString(28.88),
		"actual_amount":        anyToAmountString(4.25),
		"token":                "TXaddr",
		"block_transaction_id": "0xabc",
		"status":               strconv.Itoa(2),
	}
	sig := payment.SignMD5(reParams, token)
	body, _ := json.Marshal(map[string]any{
		"trade_id":             "tid-1",
		"order_id":             "K2ORDER1",
		"amount":               28.88,
		"actual_amount":        4.25,
		"token":                "TXaddr",
		"block_transaction_id": "0xabc",
		"status":               2,
		"signature":            sig,
	})

	var g BEpusdtGateway
	res, err := g.HandleNotify(context.Background(), nil, body, string(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.TradeNo != "K2ORDER1" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.PaidAmount != 2888 {
		t.Fatalf("paid amount cents=%d", res.PaidAmount)
	}
}

func TestBEpusdtHandleNotify_WaitingNotSuccess(t *testing.T) {
	token := "tok"
	cfg := `{"api_token":"tok"}`
	// Use integer amount so JSON float64 10 matches anyToAmountString → "10"
	rp := map[string]string{
		"order_id": "O1",
		"status":   "1",
		"amount":   "10",
	}
	sig := payment.SignMD5(rp, token)
	body, _ := json.Marshal(map[string]any{
		"order_id": "O1", "status": 1, "amount": 10, "signature": sig,
	})

	var g BEpusdtGateway
	res, err := g.HandleNotify(context.Background(), nil, body, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if res.Success {
		t.Fatal("status=1 must not be success")
	}
}

func TestBEpusdtHandleNotify_BadSignature(t *testing.T) {
	cfg := `{"api_token":"tok"}`
	body, _ := json.Marshal(map[string]any{
		"order_id": "O1", "status": 2, "amount": 10, "signature": "deadbeef",
	})
	var g BEpusdtGateway
	_, err := g.HandleNotify(context.Background(), nil, body, cfg)
	if err == nil {
		t.Fatal("expected signature error")
	}
}

func TestBEpusdtCreatePayment_MissingConfig(t *testing.T) {
	var g BEpusdtGateway
	order := &models.Order{TradeNo: "T1", TotalAmount: 100, PlanName: "P", Currency: "CNY"}
	_, err := g.CreatePayment(context.Background(), order, `{}`, payment.CreateOptions{
		NotifyURL: "https://a/n", RedirectURL: "https://a/r",
	})
	if err == nil {
		t.Fatal("expected error for empty base_url")
	}
}

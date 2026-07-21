package user

import (
	"testing"
	"time"

	"K2board/internal/models"
)

func TestToUserOrderView_PendingCashier(t *testing.T) {
	now := time.Now()
	o := &models.Order{
		TradeNo:       "K2TEST1",
		PlanName:      "月付",
		Duration:      30 * 86400,
		TrafficLimit:  100 * 1024 * 1024 * 1024,
		SpeedLimit:    100,
		DeviceLimit:   3,
		TotalAmount:   1000,
		Currency:      "CNY",
		Status:        models.OrderPending,
		ExpiredAt:     now.Add(20 * time.Minute),
		Meta:          `{"payment_url":"https://pay.example/cashier/1","receive_address":"Txxx","actual_amount":1.5,"token":"usdt","network":"tron"}`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	v := toUserOrderView(o)
	if !v.CanReopenCashier {
		t.Fatal("expected can_reopen_cashier")
	}
	if v.PaymentURL != "https://pay.example/cashier/1" {
		t.Fatalf("payment_url: %s", v.PaymentURL)
	}
	if v.Benefits.DurationText == "" || v.Benefits.TrafficText == "" {
		t.Fatalf("benefits missing: %+v", v.Benefits)
	}
	if v.RemainingSeconds <= 0 {
		t.Fatalf("remaining: %d", v.RemainingSeconds)
	}
	if v.CancelHint == "" || v.StatusHint == "" {
		t.Fatal("hints missing")
	}
}

func TestToUserOrderView_UserCancelHint(t *testing.T) {
	o := &models.Order{
		Status:    models.OrderCancelled,
		Remark:    "closed by user",
		PlanName:  "X",
		ExpiredAt: time.Now(),
	}
	v := toUserOrderView(o)
	if v.CanReopenCashier {
		t.Fatal("cancelled should not reopen")
	}
	if v.PaymentURL != "" {
		t.Fatal("no payment_url on cancelled")
	}
	if v.StatusHint == "" {
		t.Fatal("expected status hint for user cancel")
	}
}

func TestDurationAndTrafficText(t *testing.T) {
	if durationText(30*86400) != "30 天" {
		t.Fatalf("got %s", durationText(30*86400))
	}
	if trafficText(0, 30*86400) != "不限流量" {
		t.Fatal(trafficText(0, 30*86400))
	}
	const gb = 80 * 1024 * 1024 * 1024
	if got := trafficText(gb, 30*86400); got != "80 GB" {
		t.Fatalf("traffic amount: %s", got)
	}
	if got := trafficText(gb, 90*86400); got != "80 GB" {
		t.Fatalf("traffic amount independent of duration: %s", got)
	}
	if trafficLabel(365*86400) != "每月流量" {
		t.Fatal("year plan label")
	}
	if trafficLabel(30*86400) != "流量" {
		t.Fatal("30d plan label")
	}
}

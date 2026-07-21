package gateways

import "testing"

func TestParseEpusdtConfig_CashierSelectDefault(t *testing.T) {
	c, err := parseEpusdtConfig(`{"base_url":"https://upay.example","secret_key":"s"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !c.useCashierSelect() {
		t.Fatalf("empty token/network should cashier-select, got token=%q network=%q", c.Token, c.Network)
	}
}

func TestParseEpusdtConfig_LockedChain(t *testing.T) {
	c, err := parseEpusdtConfig(`{"base_url":"https://upay.example","secret_key":"s","token":"usdt","network":"tron"}`)
	if err != nil {
		t.Fatal(err)
	}
	if c.useCashierSelect() {
		t.Fatal("locked chain should not cashier-select")
	}
	if c.Token != "usdt" || c.Network != "tron" {
		t.Fatalf("got %s/%s", c.Token, c.Network)
	}
}

func TestParseEpusdtConfig_AutoAlias(t *testing.T) {
	c, err := parseEpusdtConfig(`{"token":"auto","network":"auto","secret_key":"s","base_url":"https://x"}`)
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != "" || c.Network != "" || !c.useCashierSelect() {
		t.Fatalf("auto should normalize empty: %#v", c)
	}
}

func TestParseEpusdtConfig_CashierSelectFlag(t *testing.T) {
	on := true
	c, err := parseEpusdtConfig(`{"token":"usdt","network":"tron","secret_key":"s","base_url":"https://x","cashier_select":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if c.CashierSelect == nil || *c.CashierSelect != on {
		t.Fatal("flag not parsed")
	}
	if !c.useCashierSelect() {
		t.Fatal("cashier_select:true must override locked token/network")
	}
}

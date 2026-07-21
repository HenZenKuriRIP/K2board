package user

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"K2board/internal/models"
)

// userOrderView is a client-safe order DTO (no full meta / secrets).
type userOrderView struct {
	ID            uint       `json:"id"`
	TradeNo       string     `json:"trade_no"`
	PlanID        uint       `json:"plan_id"`
	PlanName      string     `json:"plan_name"`
	GroupID       uint       `json:"group_id"`
	Duration      int64      `json:"duration"`
	TrafficLimit  int64      `json:"traffic_limit"`
	SpeedLimit    int64      `json:"speed_limit"`
	DeviceLimit   int        `json:"device_limit"`
	TotalAmount   int64      `json:"total_amount"`
	Currency      string     `json:"currency"`
	Status        string     `json:"status"`
	PaymentMethod string     `json:"payment_method"`
	PaidAt        *time.Time `json:"paid_at,omitempty"`
	ExpiredAt     time.Time  `json:"expired_at"`
	FulfilledAt   *time.Time `json:"fulfilled_at,omitempty"`
	Remark        string     `json:"remark,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	// Checkout extras (safe subset of gateway meta)
	PaymentURL    string `json:"payment_url,omitempty"`
	PayAddress    string `json:"pay_address,omitempty"`
	CryptoAmount  string `json:"crypto_amount,omitempty"`
	CryptoToken   string `json:"crypto_token,omitempty"`
	CryptoNetwork string `json:"crypto_network,omitempty"`

	// UX helpers
	RemainingSeconds int64             `json:"remaining_seconds"`
	Benefits         userOrderBenefits `json:"benefits"`
	CanReopenCashier bool              `json:"can_reopen_cashier"`
	CancelHint       string            `json:"cancel_hint,omitempty"`
	StatusHint       string            `json:"status_hint,omitempty"`
}

type userOrderBenefits struct {
	PlanName     string `json:"plan_name"`
	Duration     int64  `json:"duration"`
	DurationText string `json:"duration_text"`
	TrafficLimit int64  `json:"traffic_limit"`
	TrafficText  string `json:"traffic_text"`
	// TrafficLabel: "每月流量" when duration > 1 month, else "流量"
	TrafficLabel string `json:"traffic_label"`
	SpeedLimit   int64  `json:"speed_limit"`
	SpeedText    string `json:"speed_text"`
	DeviceLimit  int    `json:"device_limit"`
	DeviceText   string `json:"device_text"`
}

func durationText(sec int64) string {
	if sec <= 0 {
		return "—"
	}
	d := float64(sec) / 86400
	if d >= 365 {
		y := d / 365
		if y == float64(int64(y)) {
			return fmt.Sprintf("%d 年", int64(y))
		}
		return fmt.Sprintf("%.1f 年", y)
	}
	if d >= 1 {
		if d == float64(int64(d)) {
			return fmt.Sprintf("%d 天", int64(d))
		}
		return fmt.Sprintf("%.0f 天", d)
	}
	h := sec / 3600
	if h >= 1 {
		return fmt.Sprintf("%d 小时", h)
	}
	m := sec / 60
	if m < 1 {
		m = 1
	}
	return fmt.Sprintf("%d 分钟", m)
}

// monthSec: plans longer than ~1 month treat traffic_limit as monthly quota in UX copy.
const monthSec int64 = 31 * 86400

func trafficAmountText(bytes int64) string {
	if bytes <= 0 {
		return "不限流量"
	}
	const gb = 1024 * 1024 * 1024
	const mb = 1024 * 1024
	if bytes >= gb {
		v := float64(bytes) / float64(gb)
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d GB", int64(v))
		}
		return fmt.Sprintf("%.2f GB", v)
	}
	return fmt.Sprintf("%d MB", bytes/mb)
}

// trafficText formats quota amount only (e.g. "80 GB"). Use trafficLabel for "每月流量".
func trafficText(bytes, duration int64) string {
	_ = duration // reserved; amount formatting independent of duration
	return trafficAmountText(bytes)
}

// isMonthlyTrafficQuota reports whether traffic should be labeled as monthly.
func isMonthlyTrafficQuota(duration int64) bool {
	return duration > monthSec
}

func trafficLabel(duration int64) string {
	if isMonthlyTrafficQuota(duration) {
		return "每月流量"
	}
	return "流量"
}

func speedText(mbps int64) string {
	if mbps <= 0 {
		return "不限速"
	}
	return fmt.Sprintf("%d Mbps", mbps)
}

func deviceText(n int) string {
	if n <= 0 {
		return "不限设备"
	}
	return fmt.Sprintf("%d 台", n)
}

func parseMetaSafe(meta string) (paymentURL, payAddr, amount, token, network string) {
	meta = strings.TrimSpace(meta)
	if meta == "" {
		return
	}
	var m map[string]any
	if json.Unmarshal([]byte(meta), &m) != nil {
		return
	}
	str := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := m[k]; ok && v != nil {
				s := strings.TrimSpace(anyString(v))
				if s != "" && s != "null" {
					return s
				}
			}
		}
		return ""
	}
	paymentURL = str("payment_url")
	payAddr = str("receive_address", "pay_address")
	amount = str("actual_amount", "crypto_amount")
	token = str("token")
	network = str("network")
	return
}

func anyString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case json.Number:
		return t.String()
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return strings.Trim(string(b), `"`)
	}
}

func toUserOrderView(o *models.Order) userOrderView {
	v := userOrderView{
		ID:            o.ID,
		TradeNo:       o.TradeNo,
		PlanID:        o.PlanID,
		PlanName:      o.PlanName,
		GroupID:       o.GroupID,
		Duration:      o.Duration,
		TrafficLimit:  o.TrafficLimit,
		SpeedLimit:    o.SpeedLimit,
		DeviceLimit:   o.DeviceLimit,
		TotalAmount:   o.TotalAmount,
		Currency:      o.Currency,
		Status:        o.Status,
		PaymentMethod: o.PaymentMethod,
		PaidAt:        o.PaidAt,
		ExpiredAt:     o.ExpiredAt,
		FulfilledAt:   o.FulfilledAt,
		Remark:        o.Remark,
		CreatedAt:     o.CreatedAt,
		UpdatedAt:     o.UpdatedAt,
		Benefits: userOrderBenefits{
			PlanName:     o.PlanName,
			Duration:     o.Duration,
			DurationText: durationText(o.Duration),
			TrafficLimit: o.TrafficLimit,
			TrafficText:  trafficText(o.TrafficLimit, o.Duration),
			TrafficLabel: trafficLabel(o.Duration),
			SpeedLimit:   o.SpeedLimit,
			SpeedText:    speedText(o.SpeedLimit),
			DeviceLimit:  o.DeviceLimit,
			DeviceText:   deviceText(o.DeviceLimit),
		},
	}

	payURL, payAddr, amount, token, network := parseMetaSafe(o.Meta)

	switch o.Status {
	case models.OrderPending:
		rem := time.Until(o.ExpiredAt).Seconds()
		if rem > 0 {
			v.RemainingSeconds = int64(rem)
		}
		v.PaymentURL = payURL
		v.PayAddress = payAddr
		v.CryptoAmount = amount
		v.CryptoToken = token
		v.CryptoNetwork = network
		v.CanReopenCashier = payURL != "" && rem > 0
		v.CancelHint = "取消后不可再支付此订单；若已转账请勿取消（系统不会因迟到到账自动开通，需管理员补单）。第三方收银台可能仍显示待付款直至过期。"
		v.StatusHint = "请在倒计时内完成支付。超时将自动关闭；关闭后请勿再向原地址转账。"
	case models.OrderPaid:
		v.StatusHint = "支付成功，套餐权益已写入账户。可在仪表盘复制订阅链接使用。若支付后未自动跳回本站，重新登录即可看到已生效套餐。"
		v.PayAddress = payAddr
		v.CryptoAmount = amount
		v.CryptoToken = token
		v.CryptoNetwork = network
	case models.OrderCancelled:
		switch o.Remark {
		case "closed by user":
			v.StatusHint = "您已手动取消。此后即使链上到账也不会自动开通，如有误付请联系管理员人工补单。"
		case "auto-expired":
			v.StatusHint = "订单超时已关闭。若支付在超时后到账，系统可能自动补开通；也可联系管理员处理。"
		case "closed by admin":
			v.StatusHint = "订单已被管理员关闭。如需开通请重新下单或联系支持。"
		default:
			v.StatusHint = "订单已取消。如需开通请重新选购套餐。"
		}
	case models.OrderFailed:
		v.StatusHint = "订单失败。请重新下单或联系管理员。"
	}

	return v
}

func toUserOrderViews(list []models.Order) []userOrderView {
	out := make([]userOrderView, 0, len(list))
	for i := range list {
		out = append(out, toUserOrderView(&list[i]))
	}
	return out
}

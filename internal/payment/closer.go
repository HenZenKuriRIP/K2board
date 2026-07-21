package payment

import (
	"context"

	"K2board/internal/models"
)

// Closer is an optional gateway capability to close unpaid remote trades
// (e.g. alipay.trade.close). Gateways that do not support close simply omit this.
type Closer interface {
	ClosePayment(ctx context.Context, order *models.Order, cfg string) error
}

// AsCloser returns Closer if the gateway implements remote close.
func AsCloser(g Gateway) (Closer, bool) {
	c, ok := g.(Closer)
	return c, ok
}

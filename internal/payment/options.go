package payment

// CreateOptions carries request-scoped URLs for gateways that need callbacks.
type CreateOptions struct {
	NotifyURL   string
	RedirectURL string
	// ClientIP is the payer IP when available (required by some gateways e.g. frog).
	ClientIP string
}

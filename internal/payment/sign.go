package payment

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// SignMD5 builds the epusdt/BEpusdt-compatible signature:
// sort non-empty params (except signature) by key, join as k=v&..., append apiToken, MD5 lowercase.
func SignMD5(params map[string]string, apiToken string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "signature" || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	raw := strings.Join(parts, "&") + apiToken
	sum := md5.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// FormatFiatFromCents formats minor units as a decimal string with 2 places (e.g. 2888 → "28.88").
func FormatFiatFromCents(cents int64) string {
	if cents < 0 {
		cents = 0
	}
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

// FiatToCents converts a fiat major-unit amount to cents with half-up rounding.
func FiatToCents(amount float64) int64 {
	if amount < 0 {
		return 0
	}
	return int64(amount*100 + 0.5)
}

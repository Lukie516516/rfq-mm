package binance

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

func IsProd(base string) bool {
	u := strings.ToLower(base)
	return strings.Contains(u, "api.binance.com") && !strings.Contains(u, "testnet")
}

// GuardLiveSend blocks mainnet orders unless ALLOW_PROD=1.
func GuardLiveSend(base string, send bool) error {
	if !send {
		return nil
	}
	if IsProd(base) && os.Getenv("ALLOW_PROD") != "1" {
		return fmt.Errorf("refusing live send against %s; set ALLOW_PROD=1 only for a controlled mainnet drip", base)
	}
	return nil
}

func NewClientOrderID(prefix string) string {
	if prefix == "" {
		prefix = "rfq"
	}
	var b [8]byte
	_, _ = rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:])
}

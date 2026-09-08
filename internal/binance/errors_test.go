package binance

import (
	"errors"
	"testing"
)

func TestRetryableClass(t *testing.T) {
	if Retryable(&APIError{Code: -1013, Msg: "LOT_SIZE"}) {
		t.Fatal("filter must not retry")
	}
	if !Retryable(&APIError{Code: -1021, Status: 400, Msg: "timestamp"}) {
		t.Fatal("clock drift should retry")
	}
	if !Retryable(&APIError{Status: 503, Msg: "down"}) {
		t.Fatal("5xx should retry")
	}
	if Retryable(errors.New("BINANCE_API_KEY / BINANCE_API_SECRET not set")) {
		t.Fatal("missing key is not retryable")
	}
}

func TestProdGuard(t *testing.T) {
	if err := GuardLiveSend("https://testnet.binance.vision", true); err != nil {
		t.Fatal(err)
	}
	if err := GuardLiveSend("https://api.binance.com", true); err == nil {
		t.Fatal("prod send must be blocked")
	}
}

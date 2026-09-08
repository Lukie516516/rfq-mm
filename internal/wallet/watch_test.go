package wallet

import (
	"testing"

	"rfq-mm/internal/binance"
	"rfq-mm/internal/ledger"
	"rfq-mm/internal/types"
)

func TestCheckMatchesHedgeFills(t *testing.T) {
	w := Watch{StartBNB: 1, StartUSDT: 100}
	fills := []ledger.Fill{
		{Source: "rfq", Side: types.SideSell, Price: 600.25, Qty: 1},
		{Source: "hedge-binance", Side: types.SideBuy, Price: 600.10, Qty: 1, Fee: 0.01},
	}
	only := ExchangeFills(fills)
	if len(only) != 1 {
		t.Fatalf("rfq must not hit wallet, got %d", len(only))
	}
	acct := binance.Account{Free: map[string]float64{"BNB": 2, "USDT": 100 - 600.10 - 0.01}}
	if err := w.Check(acct, only, 1e-8, 1e-8); err != nil {
		t.Fatal(err)
	}
}

func TestCheckFlagsDrift(t *testing.T) {
	w := Watch{StartBNB: 0, StartUSDT: 1000}
	fills := []ledger.Fill{{Source: "hedge-binance", Side: types.SideBuy, Price: 600, Qty: 1}}
	acct := binance.Account{Free: map[string]float64{"BNB": 0.5, "USDT": 400}}
	if err := w.Check(acct, fills, 0.001, 0.01); err == nil {
		t.Fatal("expected BNB drift")
	}
}

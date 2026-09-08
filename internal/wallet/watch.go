package wallet

import (
	"fmt"
	"math"

	"rfq-mm/internal/binance"
	"rfq-mm/internal/ledger"
	"rfq-mm/internal/types"
)

// Watch remembers the exchange wallet at session start and checks it against our fills.
type Watch struct {
	StartBNB  float64
	StartUSDT float64
}

func New(acct binance.Account) Watch {
	return Watch{StartBNB: acct.FreeOf("BNB"), StartUSDT: acct.FreeOf("USDT")}
}

// ExchangeFills keeps only Binance hedge fills. RFQ prints never hit the wallet.
func ExchangeFills(fills []ledger.Fill) []ledger.Fill {
	var out []ledger.Fill
	for _, f := range fills {
		if f.Source == "hedge-binance" {
			out = append(out, f)
		}
	}
	return out
}

func (w Watch) Expected(fills []ledger.Fill) (bnb, usdt float64) {
	bnb, usdt = w.StartBNB, w.StartUSDT
	for _, f := range fills {
		notional := f.Price * f.Qty
		switch f.Side {
		case types.SideBuy:
			bnb += f.Qty
			usdt -= notional + f.Fee
		case types.SideSell:
			bnb -= f.Qty
			usdt += notional - f.Fee
		}
	}
	return bnb, usdt
}

func (w Watch) Check(acct binance.Account, fills []ledger.Fill, bnbTol, usdtTol float64) error {
	wantBNB, wantUSDT := w.Expected(fills)
	gotBNB, gotUSDT := acct.FreeOf("BNB"), acct.FreeOf("USDT")
	if math.Abs(gotBNB-wantBNB) > bnbTol+1e-12 {
		return fmt.Errorf("wallet BNB drift: exch=%.8f expected=%.8f tol=%.8f", gotBNB, wantBNB, bnbTol)
	}
	if math.Abs(gotUSDT-wantUSDT) > usdtTol+1e-12 {
		return fmt.Errorf("wallet USDT drift: exch=%.8f expected=%.8f tol=%.8f", gotUSDT, wantUSDT, usdtTol)
	}
	return nil
}

package ledger

import (
	"math"
	"testing"

	"rfq-mm/internal/types"
)

func TestApplyBuyAndSell(t *testing.T) {
	b := Book{}
	b.Apply(Fill{Source: "rfq", Side: types.SideSell, Price: 600.25, Qty: 1})
	if math.Abs(b.BNB+1) > 1e-9 || math.Abs(b.USDT-600.25) > 1e-9 {
		t.Fatalf("after sell: %+v", b)
	}
	if math.Abs(b.AvgPx-600.25) > 1e-9 {
		t.Fatalf("short avg = %f", b.AvgPx)
	}
	b.Apply(Fill{Source: "hedge", Side: types.SideBuy, Price: 600.10, Qty: 1})
	if math.Abs(b.BNB) > 1e-9 || math.Abs(b.USDT-0.15) > 1e-9 {
		t.Fatalf("after hedge buy: %+v", b)
	}
	if math.Abs(b.Realized-0.15) > 1e-9 {
		t.Fatalf("realized = %f, want 0.15", b.Realized)
	}
}

func TestAvgCostLongThenSell(t *testing.T) {
	b := Book{}
	b.Apply(Fill{Side: types.SideBuy, Price: 599.85, Qty: 1})
	b.Apply(Fill{Side: types.SideSell, Price: 600.00, Qty: 1})
	if math.Abs(b.BNB) > 1e-9 || math.Abs(b.Realized-0.15) > 1e-9 {
		t.Fatalf("book=%+v", b)
	}
}

func TestUnrealizedMatchesMark(t *testing.T) {
	b := Book{}
	b.Apply(Fill{Side: types.SideSell, Price: 600.25, Qty: 1})
	mark := 600.05
	if math.Abs(b.Unrealized(mark)-0.20) > 1e-9 {
		t.Fatalf("unrealized=%f", b.Unrealized(mark))
	}
	if math.Abs(b.Equity(mark)-(b.Realized+b.Unrealized(mark))) > 1e-9 {
		t.Fatalf("equity identity: eq=%f r=%f u=%f", b.Equity(mark), b.Realized, b.Unrealized(mark))
	}
}

func TestAddToLongUsesIntegerAvg(t *testing.T) {
	b := Book{}
	b.Apply(Fill{Side: types.SideBuy, Price: 600, Qty: 2})
	b.Apply(Fill{Side: types.SideBuy, Price: 603, Qty: 1})
	if math.Abs(b.BNB-3) > 1e-9 {
		t.Fatalf("bnb=%f", b.BNB)
	}
	if math.Abs(b.AvgPx-601) > 1e-9 {
		t.Fatalf("avg=%f want 601", b.AvgPx)
	}
}

func TestFeeReducesRealized(t *testing.T) {
	b := Book{}
	b.Apply(Fill{Side: types.SideSell, Price: 600.25, Qty: 1, Fee: 0.01})
	b.Apply(Fill{Side: types.SideBuy, Price: 600.10, Qty: 1, Fee: 0.01})
	if math.Abs(b.Realized-0.13) > 1e-9 {
		t.Fatalf("realized=%f want 0.13", b.Realized)
	}
}

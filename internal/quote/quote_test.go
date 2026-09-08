package quote

import (
	"math"
	"testing"
	"time"

	"rfq-mm/internal/market"
)

func TestCostBasedHalfSpread(t *testing.T) {
	m := market.Fixture()
	h := HalfSpread(m, Params{ProfitPad: 0.15})
	if math.Abs(h-0.20) > 1e-9 {
		t.Fatalf("half=%f want 0.20", h)
	}
}

func TestLongInventorySkewsDown(t *testing.T) {
	q, err := Make(market.Fixture(), Params{Qty: 1, ProfitPad: 0.15, Inventory: 1, SkewPerBNB: 0.05})
	if err != nil {
		t.Fatal(err)
	}
	flat, _ := Make(market.Fixture(), Params{Qty: 1, ProfitPad: 0.15})
	if q.Bid >= flat.Bid || q.Ask >= flat.Ask {
		t.Fatalf("long inventory should lower both sides: skew=%+v flat=%+v", q, flat)
	}
	if math.Abs(q.Skew-0.05) > 1e-9 {
		t.Fatalf("skew=%f", q.Skew)
	}
}

func TestVolPadWidensHalf(t *testing.T) {
	h := HalfSpread(market.Fixture(), Params{ProfitPad: 0.15, VolPad: 0.05})
	if math.Abs(h-0.25) > 1e-9 {
		t.Fatalf("half=%f want 0.25", h)
	}
}

func TestTTLExpiry(t *testing.T) {
	now := time.Unix(100, 0)
	q, err := Make(market.Fixture(), Params{Qty: 1, ProfitPad: 0.15, TTL: time.Second, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if !q.Alive(now) || !q.Alive(now.Add(time.Second)) {
		t.Fatal("should be alive through expiry instant")
	}
	if q.Alive(now.Add(time.Second + time.Nanosecond)) {
		t.Fatal("should be dead after TTL")
	}
}

func TestShortInventorySkewsUp(t *testing.T) {
	q, err := Make(market.Fixture(), Params{Qty: 1, ProfitPad: 0.15, Inventory: -1, SkewPerBNB: 0.05})
	if err != nil {
		t.Fatal(err)
	}
	flat, _ := Make(market.Fixture(), Params{Qty: 1, ProfitPad: 0.15})
	if q.Bid <= flat.Bid || q.Ask <= flat.Ask {
		t.Fatalf("short inventory should raise both sides")
	}
}

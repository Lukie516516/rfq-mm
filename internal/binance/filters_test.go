package binance

import (
	"math"
	"testing"
)

func fixture() Filters {
	return Filters{
		Symbol:      "BNBUSDT",
		TickSize:    0.01,
		StepSize:    0.001,
		MinQty:      0.001,
		MaxQty:      900000,
		MinPrice:    0.01,
		MaxPrice:    100000,
		MinNotional: 5,
	}
}

func TestFloorAndFormat(t *testing.T) {
	if got := FloorToStep(1.0007, 0.001); math.Abs(got-1.000) > 1e-9 {
		t.Fatalf("floor=%g", got)
	}
	if got := FormatStep(1.000, 0.001); got != "1" && got != "1.000" {
		if got != "1" {
			t.Fatalf("format=%q", got)
		}
	}
	if got := CeilToStep(0.00654, 0.001); math.Abs(got-0.007) > 1e-9 {
		t.Fatalf("ceil=%g", got)
	}
}

func TestAlignQtyAndNotional(t *testing.T) {
	f := fixture()
	q, err := f.AlignQty(1.0007)
	if err != nil || math.Abs(q-1.000) > 1e-9 {
		t.Fatalf("align=%g err=%v", q, err)
	}
	if err := f.CheckNotional(0.001, 764.08); err == nil {
		t.Fatal("0.001 BNB at 764 should fail minNotional 5")
	}
	minQ, err := f.MinHedgeQty(764.08)
	if err != nil {
		t.Fatal(err)
	}
	if minQ*764.08 < 5 {
		t.Fatalf("min hedge qty %g still below notional", minQ)
	}
	if err := f.CheckLot(1.0000001); err == nil {
		t.Fatal("expected lot-size reject")
	}
}

func TestParseExchangeInfo(t *testing.T) {
	body := []byte(`{"symbols":[{"symbol":"BNBUSDT","status":"TRADING","filters":[
		{"filterType":"PRICE_FILTER","minPrice":"0.01000000","maxPrice":"100000.00000000","tickSize":"0.01000000"},
		{"filterType":"LOT_SIZE","minQty":"0.00100000","maxQty":"900000.00000000","stepSize":"0.00100000"},
		{"filterType":"NOTIONAL","minNotional":"5.00000000"}
	]}]}`)
	f, err := ParseExchangeInfo(body, "BNBUSDT")
	if err != nil {
		t.Fatal(err)
	}
	if f.TickSize != 0.01 || f.StepSize != 0.001 || f.MinNotional != 5 {
		t.Fatalf("%+v", f)
	}
}

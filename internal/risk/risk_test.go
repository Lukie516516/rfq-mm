package risk

import (
	"testing"

	"rfq-mm/internal/types"
)

func TestRejectOversize(t *testing.T) {
	if err := CheckQty(2, 1); err == nil {
		t.Fatal("expected oversize reject")
	}
}

func TestProjectedInventory(t *testing.T) {
	if got := ProjectedInventory(0, types.TakerBuy, 1); got != -1 {
		t.Fatalf("taker buy => %f", got)
	}
	if got := ProjectedInventory(0, types.TakerSell, 1); got != 1 {
		t.Fatalf("taker sell => %f", got)
	}
}

func TestDisableSidesAtLimit(t *testing.T) {
	bid, ask := Sides(3, 3)
	if bid || !ask {
		t.Fatalf("max long: bid=%v ask=%v", bid, ask)
	}
	bid, ask = Sides(-3, 3)
	if !bid || ask {
		t.Fatalf("max short: bid=%v ask=%v", bid, ask)
	}
}

func TestVenueSpreadAndLoss(t *testing.T) {
	if err := CheckVenueSpread(1, 0.2); err == nil {
		t.Fatal("wide spread should fail")
	}
	if err := CheckLoss(-2, 1); err == nil {
		t.Fatal("loss limit should fail")
	}
	if err := CheckUnhedged(3, 3); err == nil {
		t.Fatal("unhedged stop should fail")
	}
}

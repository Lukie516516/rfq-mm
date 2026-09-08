package hedge

import (
	"testing"

	"rfq-mm/internal/market"
	"rfq-mm/internal/types"
)

func TestWalkWorsensHedge(t *testing.T) {
	m := market.Fixture()
	w := Walk(m, 2, 0.01)
	if w.Ask != m.Ask+0.02 || w.Bid != m.Bid-0.02 {
		t.Fatalf("walk=%+v base=%+v", w, m)
	}
	fill, err := Immediate{SlipTicks: 1, Tick: 0.01}.Hedge(-1, m, 0)
	if err != nil || fill.Side != types.SideBuy || fill.Price != m.Ask+0.01 {
		t.Fatalf("%+v err=%v", fill, err)
	}
	if fill.TheoryPx != m.Ask {
		t.Fatalf("theory=%f", fill.TheoryPx)
	}
}

func TestFromInventoryDirections(t *testing.T) {
	m := market.Fixture()
	buy, err := FromInventory(-1, m, 0)
	if err != nil || buy.Side != types.SideBuy || buy.Price != m.Ask {
		t.Fatalf("short => buy ask, got %+v err=%v", buy, err)
	}
	sell, err := FromInventory(1, m, 0)
	if err != nil || sell.Side != types.SideSell || sell.Price != m.Bid {
		t.Fatalf("long => sell bid, got %+v err=%v", sell, err)
	}
	flat, err := FromInventory(0, m, 0)
	if err != nil || flat.Qty != 0 {
		t.Fatalf("flat => no hedge, got %+v err=%v", flat, err)
	}
}

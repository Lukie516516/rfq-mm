package hedge

import (
	"fmt"
	"math"

	"rfq-mm/internal/ledger"
	"rfq-mm/internal/market"
	"rfq-mm/internal/types"
)

const flatEps = 1e-12

// Executor tries to flatten residual BNB inventory on the hedge venue.
// Stage 2 can swap the simulated executor for a Binance order client.
type Executor interface {
	Hedge(inventory float64, m market.Book, fee float64) (ledger.Fill, error)
}

// Immediate takes the opposite side of the exchange book until inventory is flat.
// SlipTicks walks the hedge price against us to model delay (0 keeps Stage 1 numbers).
type Immediate struct {
	SlipTicks int
	Tick      float64
}

func (h Immediate) Hedge(inventory float64, m market.Book, fee float64) (ledger.Fill, error) {
	planned, err := FromInventory(inventory, m, fee)
	if err != nil {
		return ledger.Fill{}, err
	}
	fill, err := FromInventory(inventory, Walk(m, h.SlipTicks, h.Tick), fee)
	if err != nil {
		return ledger.Fill{}, err
	}
	fill.TheoryPx = planned.Price
	return fill, nil
}

// Walk makes the hedge worse by n ticks: buy pays more, sell receives less.
func Walk(m market.Book, ticks int, tick float64) market.Book {
	if ticks <= 0 || tick <= 0 {
		return m
	}
	adj := float64(ticks) * tick
	m.Ask += adj
	m.Bid -= adj
	if m.Bid <= 0 {
		m.Bid = tick
	}
	if m.Ask <= m.Bid {
		m.Ask = m.Bid + tick
	}
	return m
}

// FailOnce fails the next n hedges, then delegates to Next (or Immediate).
type FailOnce struct {
	Left   int
	Reason string
	Next   Executor
}

func (f *FailOnce) Hedge(inventory float64, m market.Book, fee float64) (ledger.Fill, error) {
	if f.Left > 0 {
		f.Left--
		reason := f.Reason
		if reason == "" {
			reason = "simulated hedge failure"
		}
		return ledger.Fill{}, fmt.Errorf("%s", reason)
	}
	next := f.Next
	if next == nil {
		next = Immediate{}
	}
	return next.Hedge(inventory, m, fee)
}

// FromInventory builds a taker hedge that drives inventory toward zero.
// Long BNB => sell at bid. Short BNB => buy at ask. Already flat => Qty 0.
func FromInventory(inventory float64, m market.Book, fee float64) (ledger.Fill, error) {
	if err := m.Validate(); err != nil {
		return ledger.Fill{}, err
	}
	qty := math.Abs(inventory)
	if qty < flatEps {
		return ledger.Fill{Source: "hedge"}, nil
	}
	if inventory > 0 {
		return ledger.Fill{
			Source: "hedge",
			Side:   types.SideSell,
			Price:  m.Bid,
			Qty:    qty,
			Fee:    fee,
		}, nil
	}
	return ledger.Fill{
		Source: "hedge",
		Side:   types.SideBuy,
		Price:  m.Ask,
		Qty:    qty,
		Fee:    fee,
	}, nil
}

// FromRFQ is the last-fill mapping. Prefer FromInventory: after a failed hedge,
// blindly reversing the next RFQ would re-open risk that the RFQ just flattened.
func FromRFQ(rfqFill ledger.Fill, m market.Book, fee float64) (ledger.Fill, error) {
	switch rfqFill.Side {
	case types.SideSell:
		return FromInventory(-rfqFill.Qty, m, fee)
	case types.SideBuy:
		return FromInventory(rfqFill.Qty, m, fee)
	default:
		return ledger.Fill{}, fmt.Errorf("unknown mm side %q", rfqFill.Side)
	}
}

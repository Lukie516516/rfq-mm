package quote

import (
	"fmt"
	"time"

	"rfq-mm/internal/market"
)

type Params struct {
	Qty                float64
	ProfitPad          float64
	FeePerUnit         float64
	Inventory          float64
	SkewPerBNB         float64
	MinHalfSpread      float64
	HalfSpreadOverride float64
	VolPad             float64
	TTL                time.Duration
	Now                time.Time
}

type Quote struct {
	Bid          float64
	Ask          float64
	Mid          float64
	Reservation  float64
	HalfSpread   float64
	VenueHalf    float64
	Skew         float64
	Qty          float64
	BidEnabled   bool
	AskEnabled   bool
	Expires      time.Time
}

func HalfSpread(m market.Book, p Params) float64 {
	if p.HalfSpreadOverride > 0 {
		return p.HalfSpreadOverride
	}
	// Cover both RFQ and hedge fees: we pay fee on each fill.
	h := m.Spread()/2 + 2*p.FeePerUnit + p.ProfitPad + p.VolPad
	if p.MinHalfSpread > 0 && h < p.MinHalfSpread {
		return p.MinHalfSpread
	}
	return h
}

func Make(m market.Book, p Params) (Quote, error) {
	if err := m.Validate(); err != nil {
		return Quote{}, err
	}
	if p.Qty <= 0 {
		return Quote{}, fmt.Errorf("qty must be > 0")
	}
	half := HalfSpread(m, p)
	if half <= 0 {
		return Quote{}, fmt.Errorf("half spread must be > 0")
	}
	mid := m.Mid()
	skew := p.SkewPerBNB * p.Inventory
	// Long inventory lowers reservation: more eager to sell, less eager to buy.
	reservation := mid - skew
	q := Quote{
		Bid:         reservation - half,
		Ask:         reservation + half,
		Mid:         mid,
		Reservation: reservation,
		HalfSpread:  half,
		VenueHalf:   m.Spread() / 2,
		Skew:        skew,
		Qty:         p.Qty,
		BidEnabled:  true,
		AskEnabled:  true,
	}
	if p.TTL > 0 {
		now := p.Now
		if now.IsZero() {
			now = time.Now()
		}
		q.Expires = now.Add(p.TTL)
	}
	if q.Bid <= 0 || q.Ask <= q.Bid {
		return Quote{}, fmt.Errorf("invalid rfq quote bid=%f ask=%f", q.Bid, q.Ask)
	}
	return q, nil
}

func (q Quote) Alive(now time.Time) bool {
	return q.Expires.IsZero() || !now.After(q.Expires)
}

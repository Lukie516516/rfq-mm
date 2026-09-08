package ledger

import (
	"rfq-mm/internal/money"
	"rfq-mm/internal/types"
)

const eps = 1e-12

// Fill is one executed trade against the MM book.
// Side is always the MM's direction in BNB.
type Fill struct {
	Source        string
	Side          types.Side
	Price         float64
	Qty           float64
	Fee           float64
	TheoryPx      float64
	OrderID       int64
	ClientOrderID string
}

// Book is the MM cash + inventory ledger.
// Public floats are views. Arithmetic is int64 at 1e8 scale.
type Book struct {
	BNB      float64
	USDT     float64
	AvgPx    float64
	Realized float64
	FeePaid  float64
	Fills    []Fill

	bnb, usdt, avg, realized, fee int64
	ints                          bool
}

type Snapshot struct {
	BNB        float64
	USDT       float64
	AvgPx      float64
	Mark       float64
	Equity     float64
	Realized   float64
	Unrealized float64
	FeePaid    float64
}

func (b *Book) ensureInts() {
	if b.ints {
		return
	}
	b.bnb = money.FromFloat(b.BNB)
	b.usdt = money.FromFloat(b.USDT)
	b.avg = money.FromFloat(b.AvgPx)
	b.realized = money.FromFloat(b.Realized)
	b.fee = money.FromFloat(b.FeePaid)
	b.ints = true
}

func (b *Book) sync() {
	b.BNB = money.ToFloat(b.bnb)
	b.USDT = money.ToFloat(b.usdt)
	b.AvgPx = money.ToFloat(b.avg)
	b.Realized = money.ToFloat(b.realized)
	b.FeePaid = money.ToFloat(b.fee)
}

func (b *Book) Apply(f Fill) {
	b.ensureInts()
	px := money.FromFloat(f.Price)
	qty := money.FromFloat(f.Qty)
	fee := money.FromFloat(f.Fee)
	notional := money.Mul(qty, px)
	switch f.Side {
	case types.SideBuy:
		b.usdt -= notional + fee
		b.realizeAndUpdate(qty, px)
	case types.SideSell:
		b.usdt += notional - fee
		b.realizeAndUpdate(-qty, px)
	}
	b.fee += fee
	b.realized -= fee
	b.Fills = append(b.Fills, f)
	b.sync()
}

func (b *Book) realizeAndUpdate(signed, px int64) {
	if money.Abs(b.bnb) == 0 {
		b.bnb = signed
		b.avg = px
		return
	}
	sameDir := (b.bnb > 0 && signed > 0) || (b.bnb < 0 && signed < 0)
	if sameDir {
		newQ := b.bnb + signed
		cost := money.Mul(money.Abs(b.bnb), b.avg) + money.Mul(money.Abs(signed), px)
		b.avg = money.Div(cost, money.Abs(newQ))
		b.bnb = newQ
		return
	}
	cover := money.Abs(signed)
	if money.Abs(b.bnb) < cover {
		cover = money.Abs(b.bnb)
	}
	if b.bnb > 0 {
		b.realized += money.Mul(px-b.avg, cover)
	} else {
		b.realized += money.Mul(b.avg-px, cover)
	}
	remain := money.Abs(signed) - cover
	if remain == 0 {
		b.bnb += signed
		if b.bnb == 0 {
			b.avg = 0
		}
		return
	}
	if signed > 0 {
		b.bnb = remain
	} else {
		b.bnb = -remain
	}
	b.avg = px
}

func (b Book) Equity(mark float64) float64 {
	return b.USDT + b.BNB*mark
}

func (b Book) Unrealized(mark float64) float64 {
	if abs(b.BNB) < eps {
		return 0
	}
	return b.BNB * (mark - b.AvgPx)
}

func (b Book) Snapshot(mark float64) Snapshot {
	return Snapshot{
		BNB:        b.BNB,
		USDT:       b.USDT,
		AvgPx:      b.AvgPx,
		Mark:       mark,
		Equity:     b.Equity(mark),
		Realized:   b.Realized,
		Unrealized: b.Unrealized(mark),
		FeePaid:    b.FeePaid,
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

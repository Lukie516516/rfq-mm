package market

import (
	"fmt"
	"time"
)

// Book is the best bid/ask we use as the hedge venue quote.
type Book struct {
	Bid    float64
	Ask    float64
	Source string
	Ts     time.Time
}

func (b Book) Mid() float64 {
	return (b.Bid + b.Ask) / 2
}

func (b Book) Spread() float64 {
	return b.Ask - b.Bid
}

func (b Book) Validate() error {
	if b.Bid <= 0 || b.Ask <= 0 {
		return fmt.Errorf("invalid book: bid=%f ask=%f", b.Bid, b.Ask)
	}
	if b.Ask <= b.Bid {
		return fmt.Errorf("crossed or locked book: bid=%f ask=%f", b.Bid, b.Ask)
	}
	return nil
}

func (b Book) Stale(maxAge time.Duration, now time.Time) error {
	if maxAge <= 0 || b.Ts.IsZero() {
		return nil
	}
	age := now.Sub(b.Ts)
	if age > maxAge {
		return fmt.Errorf("stale book age=%s max=%s source=%s", age.Round(time.Millisecond), maxAge, b.Source)
	}
	return nil
}

// Fixture is the Stage-1 acceptance market used in DESIGN.md.
func Fixture() Book {
	return Book{Bid: 600.00, Ask: 600.10, Source: "mock"}
}

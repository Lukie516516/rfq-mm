package hedge

import (
	"fmt"
	"time"

	"rfq-mm/internal/binance"
	"rfq-mm/internal/ledger"
	"rfq-mm/internal/market"
)

// Reliable retries transient hedge errors. Filter/auth failures fail immediately.
type Reliable struct {
	Inner   Executor
	Tries   int
	Backoff time.Duration
}

func (r Reliable) Hedge(inventory float64, m market.Book, fee float64) (ledger.Fill, error) {
	tries := r.Tries
	if tries <= 0 {
		tries = 3
	}
	backoff := r.Backoff
	if backoff <= 0 {
		backoff = 200 * time.Millisecond
	}
	var last error
	for i := 0; i < tries; i++ {
		fill, err := r.Inner.Hedge(inventory, m, fee)
		if err == nil {
			return fill, nil
		}
		last = err
		if !binance.Retryable(err) {
			return ledger.Fill{}, err
		}
		if i+1 < tries {
			time.Sleep(backoff * time.Duration(i+1))
		}
	}
	return ledger.Fill{}, fmt.Errorf("hedge retries exhausted: %w", last)
}

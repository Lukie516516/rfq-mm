package rfq

import (
	"fmt"

	"rfq-mm/internal/ledger"
	"rfq-mm/internal/quote"
	"rfq-mm/internal/types"
)

// FillFromTaker maps a user action onto the MM's RFQ fill.
// Taker buy  => MM sells BNB at RFQ ask.
// Taker sell => MM buys  BNB at RFQ bid.
func FillFromTaker(action types.TakerAction, q quote.Quote, fee float64) (ledger.Fill, error) {
	switch action {
	case types.TakerBuy:
		return ledger.Fill{
			Source: "rfq",
			Side:   types.SideSell,
			Price:  q.Ask,
			Qty:    q.Qty,
			Fee:    fee,
		}, nil
	case types.TakerSell:
		return ledger.Fill{
			Source: "rfq",
			Side:   types.SideBuy,
			Price:  q.Bid,
			Qty:    q.Qty,
			Fee:    fee,
		}, nil
	default:
		return ledger.Fill{}, fmt.Errorf("unknown taker action %q", action)
	}
}

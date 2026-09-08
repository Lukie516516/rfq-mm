package hedge

import (
	"strconv"
	"strings"

	"rfq-mm/internal/binance"
	"rfq-mm/internal/ledger"
	"rfq-mm/internal/store"
	"rfq-mm/internal/types"
)

func StatusFromAck(ack binance.OrderAck) store.Status {
	switch strings.ToUpper(ack.Status) {
	case "FILLED":
		return store.Filled
	case "PARTIALLY_FILLED":
		return store.Partial
	case "CANCELED", "CANCELLED", "EXPIRED", "REJECTED":
		return store.Dead
	case "NEW", "PENDING_NEW":
		return store.Sent
	default:
		return store.Uncertain
	}
}

func FillFromAck(ack binance.OrderAck, cid string, theory, mid, fallbackQty float64) (ledger.Fill, error) {
	exec := parseExecQty(ack.ExecutedQty, fallbackQty)
	if exec <= 0 {
		return ledger.Fill{Source: "hedge-binance", ClientOrderID: cid, OrderID: ack.OrderID}, nil
	}
	px, err := ack.AvgPrice()
	if err != nil {
		return ledger.Fill{}, err
	}
	side := types.Side(strings.ToLower(ack.Side))
	if side != types.SideBuy && side != types.SideSell {
		side = ""
	}
	return ledger.Fill{
		Source:        "hedge-binance",
		Side:          side,
		Price:         px,
		Qty:           exec,
		Fee:           ack.FeeUSDT(mid),
		TheoryPx:      theory,
		OrderID:       ack.OrderID,
		ClientOrderID: cid,
	}, nil
}

func execQtyOf(ack binance.OrderAck) float64 {
	v, err := strconv.ParseFloat(ack.ExecutedQty, 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

func filledQty(book *ledger.Book, cid string) float64 {
	if book == nil || cid == "" {
		return 0
	}
	var q float64
	for _, f := range book.Fills {
		if f.ClientOrderID == cid {
			q += f.Qty
		}
	}
	return q
}

package hedge

import (
	"fmt"
	"log/slog"

	"rfq-mm/internal/binance"
	"rfq-mm/internal/ledger"
	"rfq-mm/internal/store"
)

type OrderQuery interface {
	QueryOrder(symbol, clientOrderID string) (binance.OrderAck, error)
}

type RecoverReport struct {
	Applied []ledger.Fill
	Notes   []string
}

// RecoverPending queries every locally-open clientOrderId and books any
// exchange qty the ledger does not already have. Crash after persist-before-send
// is how we find a live order without minting a new id.
func RecoverPending(st *store.File, q OrderQuery, symbol string, book *ledger.Book, mid float64, log *slog.Logger) RecoverReport {
	rep := RecoverReport{}
	if st == nil || q == nil {
		return rep
	}
	if log == nil {
		log = slog.Default()
	}
	for _, rec := range st.OpenOrders() {
		if rec.ClientOrderID == "" {
			continue
		}
		sym := rec.Symbol
		if sym == "" {
			sym = symbol
		}
		ack, err := q.QueryOrder(sym, rec.ClientOrderID)
		if err != nil {
			if binance.NotFound(err) {
				rec.Status = store.Dead
				rec.Note = "exchange: not found"
				_ = st.Put(rec)
				rep.Notes = append(rep.Notes, rec.ClientOrderID+": dead, not on exchange")
				continue
			}
			rec.Status = store.Uncertain
			rec.Note = err.Error()
			_ = st.Put(rec)
			rep.Notes = append(rep.Notes, rec.ClientOrderID+": still uncertain: "+err.Error())
			continue
		}
		fill, ferr := FillFromAck(ack, rec.ClientOrderID, rec.Price, mid, rec.Qty)
		if ferr != nil {
			rec.Status = store.Uncertain
			rec.Note = ferr.Error()
			_ = st.Put(rec)
			rep.Notes = append(rep.Notes, rec.ClientOrderID+": "+ferr.Error())
			continue
		}
		already := filledQty(book, rec.ClientOrderID)
		delta := fill.Qty - already
		if delta > 1e-12 && fill.Side != "" && book != nil {
			fill.Qty = delta
			book.Apply(fill)
			rep.Applied = append(rep.Applied, fill)
			log.Info("recover_fill", "clientOrderId", rec.ClientOrderID, "orderId", ack.OrderID, "qty", delta, "px", fill.Price)
		}
		rec.OrderID = ack.OrderID
		rec.ExecQty = execQtyOf(ack)
		rec.Status = StatusFromAck(ack)
		rec.Note = ""
		_ = st.Put(rec)
		rep.Notes = append(rep.Notes, fmt.Sprintf("%s: %s exec=%.8f", rec.ClientOrderID, rec.Status, rec.ExecQty))
	}
	return rep
}

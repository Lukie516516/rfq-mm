package hedge

import (
	"path/filepath"
	"testing"

	"rfq-mm/internal/binance"
	"rfq-mm/internal/ledger"
	"rfq-mm/internal/store"
	"rfq-mm/internal/types"
)

type stubQuery struct {
	acks map[string]binance.OrderAck
	err  error
}

func (s stubQuery) QueryOrder(symbol, cid string) (binance.OrderAck, error) {
	if s.err != nil {
		return binance.OrderAck{}, s.err
	}
	if ack, ok := s.acks[cid]; ok {
		return ack, nil
	}
	return binance.OrderAck{}, &binance.APIError{Code: -2013, Msg: "Order does not exist"}
}

func TestRecoverAppliesMissingFill(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "orders.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Put(store.Record{ClientOrderID: "hedge-1", Symbol: "BNBUSDT", Side: "BUY", Qty: 0.007, Price: 600.10, Status: store.Sent}); err != nil {
		t.Fatal(err)
	}
	book := &ledger.Book{}
	q := stubQuery{acks: map[string]binance.OrderAck{
		"hedge-1": {OrderID: 9, Status: "FILLED", Side: "BUY", ExecutedQty: "0.007", CummulativeQuoteQty: "4.2007"},
	}}
	rep := RecoverPending(st, q, "BNBUSDT", book, 600, nil)
	if len(rep.Applied) != 1 {
		t.Fatalf("applied=%v notes=%v", rep.Applied, rep.Notes)
	}
	if book.BNB < 0.006 || book.Fills[0].ClientOrderID != "hedge-1" {
		t.Fatalf("book=%+v", book)
	}
	if rec, _ := st.Get("hedge-1"); rec.Status != store.Filled {
		t.Fatalf("status=%s", rec.Status)
	}
}

func TestRecoverDoesNotDoubleBook(t *testing.T) {
	st, _ := store.Open(filepath.Join(t.TempDir(), "orders.json"))
	_ = st.Put(store.Record{ClientOrderID: "hedge-1", Status: store.Partial, Qty: 0.007, ExecQty: 0.007})
	book := &ledger.Book{}
	book.Apply(ledger.Fill{Source: "hedge-binance", Side: types.SideBuy, Price: 600, Qty: 0.007, ClientOrderID: "hedge-1"})
	q := stubQuery{acks: map[string]binance.OrderAck{
		"hedge-1": {OrderID: 9, Status: "FILLED", Side: "BUY", ExecutedQty: "0.007", CummulativeQuoteQty: "4.2"},
	}}
	before := book.BNB
	rep := RecoverPending(st, q, "BNBUSDT", book, 600, nil)
	if len(rep.Applied) != 0 {
		t.Fatalf("should be idempotent, applied=%v", rep.Applied)
	}
	if book.BNB != before {
		t.Fatalf("bnb changed %f -> %f", before, book.BNB)
	}
}

func TestRecoverMissingOrderIsDead(t *testing.T) {
	st, _ := store.Open(filepath.Join(t.TempDir(), "orders.json"))
	_ = st.Put(store.Record{ClientOrderID: "hedge-gone", Status: store.Sent})
	RecoverPending(st, stubQuery{}, "BNBUSDT", &ledger.Book{}, 600, nil)
	if rec, _ := st.Get("hedge-gone"); rec.Status != store.Dead {
		t.Fatalf("status=%s", rec.Status)
	}
}

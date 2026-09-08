package binance

import (
	"fmt"
	"testing"
	"time"
)

type stubVenue struct {
	places []error
	acks   []OrderAck
	query  map[string]OrderAck
	qerr   error
	nPlace int
	nQuery int
}

func (s *stubVenue) PlaceMarketID(symbol, side string, qty, step float64, cid string) (OrderAck, error) {
	i := s.nPlace
	s.nPlace++
	if i < len(s.places) && s.places[i] != nil {
		return OrderAck{}, s.places[i]
	}
	if i < len(s.acks) {
		return s.acks[i], nil
	}
	return OrderAck{OrderID: 1, ExecutedQty: "0.007", CummulativeQuoteQty: "5.3", Status: "FILLED", Side: side}, nil
}

func (s *stubVenue) QueryOrder(symbol, cid string) (OrderAck, error) {
	s.nQuery++
	if s.qerr != nil {
		return OrderAck{}, s.qerr
	}
	if ack, ok := s.query[cid]; ok {
		return ack, nil
	}
	return OrderAck{}, &APIError{Code: -2013, Msg: "Order does not exist"}
}

func TestSubmitMarketQueryAfterTimeout(t *testing.T) {
	cid := "hedge-abc"
	v := &stubVenue{
		places: []error{fmt.Errorf("timeout waiting for response")},
		query: map[string]OrderAck{
			cid: {OrderID: 99, ExecutedQty: "0.007", CummulativeQuoteQty: "5.3", Status: "FILLED", Side: "BUY"},
		},
	}
	ack, err := SubmitMarket(v, "BNBUSDT", "BUY", 0.007, 0.001, cid, 3, func(time.Duration) {})
	if err != nil {
		t.Fatal(err)
	}
	if ack.OrderID != 99 {
		t.Fatalf("want recovered order 99, got %d", ack.OrderID)
	}
	if v.nPlace != 1 {
		t.Fatalf("must not place a second order, places=%d", v.nPlace)
	}
	if v.nQuery == 0 {
		t.Fatal("expected a query after uncertain send")
	}
}

func TestSubmitMarketRetryWhenNeverArrived(t *testing.T) {
	v := &stubVenue{
		places: []error{fmt.Errorf("connection reset"), nil},
		acks:   []OrderAck{{}, {OrderID: 7, ExecutedQty: "0.007", CummulativeQuoteQty: "5.3", Status: "FILLED", Side: "BUY"}},
	}
	ack, err := SubmitMarket(v, "BNBUSDT", "BUY", 0.007, 0.001, "hedge-x", 3, func(time.Duration) {})
	if err != nil {
		t.Fatal(err)
	}
	if ack.OrderID != 7 || v.nPlace != 2 {
		t.Fatalf("ack=%+v places=%d", ack, v.nPlace)
	}
}

func TestErrorClasses(t *testing.T) {
	if !Uncertain(fmt.Errorf("timeout waiting for response")) {
		t.Fatal("timeout is uncertain")
	}
	if Uncertain(&APIError{Code: -1021, Msg: "timestamp"}) {
		t.Fatal("clock error is retryable but not an uncertain fill")
	}
	if !NotFound(&APIError{Code: -2013, Msg: "Order does not exist"}) {
		t.Fatal("expected not found")
	}
	if !DuplicateOrder(&APIError{Code: -2010, Msg: "Duplicate order sent."}) {
		t.Fatal("expected duplicate")
	}
}

func TestFeeUSDT(t *testing.T) {
	ack := OrderAck{Fills: []struct {
		Price           string `json:"price"`
		Qty             string `json:"qty"`
		Commission      string `json:"commission"`
		CommissionAsset string `json:"commissionAsset"`
	}{
		{Commission: "0.001", CommissionAsset: "BNB"},
		{Commission: "0.02", CommissionAsset: "USDT"},
	}}
	got := ack.FeeUSDT(100)
	if got < 0.119 || got > 0.121 {
		t.Fatalf("fee=%f", got)
	}
}

func TestAccountCheck(t *testing.T) {
	a := Account{Free: map[string]float64{"USDT": 1, "BNB": 0.001}}
	if err := a.CheckHedge("BUY", 0.007, 760); err == nil {
		t.Fatal("expected USDT short")
	}
	if err := a.CheckHedge("SELL", 0.01, 760); err == nil {
		t.Fatal("expected BNB short")
	}
	rich := Account{Free: map[string]float64{"USDT": 100, "BNB": 1}}
	if err := rich.CheckHedge("BUY", 0.007, 760); err != nil {
		t.Fatal(err)
	}
}

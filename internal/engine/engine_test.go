package engine

import (
	"math"
	"os"
	"testing"
	"time"

	"rfq-mm/internal/hedge"
	"rfq-mm/internal/ledger"
	"rfq-mm/internal/market"
	"rfq-mm/internal/metrics"
	"rfq-mm/internal/types"
)

const eps = 1e-9

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < eps
}

func defaultCfg() Config {
	return Config{Qty: 1, ProfitPad: 0.15}
}

func TestTakerBuyFlattensAndLocksPnL(t *testing.T) {
	book := &ledger.Book{}
	round, err := RunRound(book, market.Fixture(), defaultCfg(), types.TakerBuy)
	if err != nil {
		t.Fatal(err)
	}
	if round.Status != StatusFilledHedged {
		t.Fatalf("status=%s reject=%s", round.Status, round.Reject)
	}
	if round.RFQ.Side != types.SideSell {
		t.Fatalf("taker buy must map to MM sell, got %s", round.RFQ.Side)
	}
	if round.Hedge.Side != types.SideBuy {
		t.Fatalf("hedge must buy after MM sell, got %s", round.Hedge.Side)
	}
	if !almostEqual(round.RFQ.Price, 600.25) {
		t.Fatalf("rfq ask = %f, want 600.25", round.RFQ.Price)
	}
	if !almostEqual(round.Hedge.Price, 600.10) {
		t.Fatalf("hedge px = %f, want 600.10", round.Hedge.Price)
	}
	if !almostEqual(book.BNB, 0) {
		t.Fatalf("bnb after hedge = %f, want 0", book.BNB)
	}
	if !almostEqual(book.USDT, 0.15) || !almostEqual(book.Realized, 0.15) {
		t.Fatalf("usdt=%f realized=%f want 0.15", book.USDT, book.Realized)
	}
	if !almostEqual(round.AfterHedge.Equity, 0.15) {
		t.Fatalf("equity = %f, want 0.15", round.AfterHedge.Equity)
	}
}

func TestTakerSellFlattensAndLocksPnL(t *testing.T) {
	book := &ledger.Book{}
	round, err := RunRound(book, market.Fixture(), defaultCfg(), types.TakerSell)
	if err != nil {
		t.Fatal(err)
	}
	if round.RFQ.Side != types.SideBuy {
		t.Fatalf("taker sell must map to MM buy, got %s", round.RFQ.Side)
	}
	if round.Hedge.Side != types.SideSell {
		t.Fatalf("hedge must sell after MM buy, got %s", round.Hedge.Side)
	}
	if !almostEqual(round.RFQ.Price, 599.85) || !almostEqual(round.Hedge.Price, 600.00) {
		t.Fatalf("rfq=%f hedge=%f", round.RFQ.Price, round.Hedge.Price)
	}
	if !almostEqual(book.BNB, 0) || !almostEqual(book.USDT, 0.15) {
		t.Fatalf("book after hedge bnb=%f usdt=%f", book.BNB, book.USDT)
	}
}

func TestQuoteDoesNotMutateLedger(t *testing.T) {
	book := &ledger.Book{BNB: 2, USDT: 100}
	before := *book
	_, err := RunRound(&ledger.Book{BNB: 2, USDT: 100}, market.Fixture(), defaultCfg(), types.TakerBuy)
	if err != nil {
		t.Fatal(err)
	}
	if book.BNB != before.BNB || book.USDT != before.USDT {
		t.Fatal("quote/run on another book mutated the original")
	}
}

func TestEquityIdentity(t *testing.T) {
	book := &ledger.Book{}
	round, err := RunRound(book, market.Fixture(), defaultCfg(), types.TakerBuy)
	if err != nil {
		t.Fatal(err)
	}
	want := round.AfterRFQ.USDT + round.AfterRFQ.BNB*round.AfterRFQ.Mark
	if !almostEqual(round.AfterRFQ.Equity, want) {
		t.Fatalf("equity identity failed: %f vs %f", round.AfterRFQ.Equity, want)
	}
	if !almostEqual(round.AfterRFQ.BNB, -1) {
		t.Fatalf("after taker buy, mm should be short 1, got %f", round.AfterRFQ.BNB)
	}
	if !almostEqual(round.AfterRFQ.Unrealized, 0.20) {
		t.Fatalf("unhedged unrealized=%f want 0.20", round.AfterRFQ.Unrealized)
	}
}

func TestHedgeFailureLeavesInventoryAndSkewsNextQuote(t *testing.T) {
	book := &ledger.Book{}
	cfg := Config{Qty: 1, ProfitPad: 0.15, SkewPerBNB: 0.05, Hedger: &hedge.FailOnce{Left: 1, Reason: "timeout"}}
	failed, err := RunRound(book, market.Fixture(), cfg, types.TakerSell)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != StatusFilledUnhedged || book.BNB != 1 {
		t.Fatalf("status=%s bnb=%f", failed.Status, book.BNB)
	}
	cfg.Hedger = hedge.Immediate{}
	next, err := RunRound(book, market.Fixture(), cfg, types.TakerBuy)
	if err != nil {
		t.Fatal(err)
	}
	if next.Quote.Skew <= 0 {
		t.Fatalf("expected downside skew with long inventory, got %f", next.Quote.Skew)
	}
	if next.Quote.Ask >= 600.25 {
		t.Fatalf("long inventory should cheapen the ask, got %f", next.Quote.Ask)
	}
	if next.Hedge.Qty != 0 {
		t.Fatalf("RFQ already flattened inventory; hedge should be a no-op, got %+v", next.Hedge)
	}
	if !almostEqual(book.BNB, 0) {
		t.Fatalf("should be flat after inventory-aware sell, bnb=%f", book.BNB)
	}
}

func TestRejectOversizeDoesNotFill(t *testing.T) {
	book := &ledger.Book{}
	round, err := RunRound(book, market.Fixture(), Config{Qty: 2, ProfitPad: 0.15, MaxOrderQty: 1}, types.TakerBuy)
	if err != nil {
		t.Fatal(err)
	}
	if round.Status != StatusRejected || book.BNB != 0 || len(book.Fills) != 0 {
		t.Fatalf("expected clean reject, got %+v book=%+v", round, book)
	}
}

func TestRejectAtInventoryLimit(t *testing.T) {
	book := &ledger.Book{BNB: 1, AvgPx: 600}
	round, err := RunRound(book, market.Fixture(), Config{Qty: 1, ProfitPad: 0.15, MaxAbsInventory: 1}, types.TakerSell)
	if err != nil {
		t.Fatal(err)
	}
	if round.Status != StatusRejected {
		t.Fatalf("status=%s", round.Status)
	}
	if book.BNB != 1 {
		t.Fatalf("inventory changed on reject: %f", book.BNB)
	}
}

func TestRejectStaleBook(t *testing.T) {
	book := &ledger.Book{}
	mkt := market.Fixture()
	mkt.Ts = time.Unix(0, 0)
	round, err := RunRound(book, mkt, Config{
		Qty: 1, ProfitPad: 0.15,
		MaxBookAge: time.Second,
		Now:        func() time.Time { return time.Unix(100, 0) },
	}, types.TakerBuy)
	if err != nil {
		t.Fatal(err)
	}
	if round.Status != StatusRejected || book.BNB != 0 {
		t.Fatalf("stale book should reject, status=%s bnb=%f", round.Status, book.BNB)
	}
}

func TestRejectKillSwitch(t *testing.T) {
	path := t.TempDir() + "/KILL"
	if err := os.WriteFile(path, []byte("stop"), 0o644); err != nil {
		t.Fatal(err)
	}
	book := &ledger.Book{}
	round, err := RunRound(book, market.Fixture(), Config{Qty: 1, ProfitPad: 0.15, KillSwitch: path}, types.TakerBuy)
	if err != nil {
		t.Fatal(err)
	}
	if round.Status != StatusRejected {
		t.Fatalf("status=%s", round.Status)
	}
}

func TestSlipTicksCutsLockedPnL(t *testing.T) {
	book := &ledger.Book{}
	round, err := RunRound(book, market.Fixture(), Config{Qty: 1, ProfitPad: 0.15, SlipTicks: 1, TickSize: 0.01}, types.TakerBuy)
	if err != nil {
		t.Fatal(err)
	}
	if !almostEqual(round.Hedge.Price, 600.11) {
		t.Fatalf("hedge px=%f want 600.11", round.Hedge.Price)
	}
	if !almostEqual(book.USDT, 0.14) {
		t.Fatalf("usdt=%f want 0.14", book.USDT)
	}
}

func TestQuoteTTLRejectsLateHit(t *testing.T) {
	book := &ledger.Book{}
	round, err := RunRound(book, market.Fixture(), Config{
		Qty: 1, ProfitPad: 0.15,
		QuoteTTL: time.Millisecond,
		HitDelay: time.Second,
		Now:      func() time.Time { return time.Unix(10, 0) },
	}, types.TakerBuy)
	if err != nil {
		t.Fatal(err)
	}
	if round.Status != StatusRejected || round.Reject != "quote expired" || book.BNB != 0 {
		t.Fatalf("status=%s reject=%s bnb=%f", round.Status, round.Reject, book.BNB)
	}
}

func TestVolPadWidensQuote(t *testing.T) {
	book := &ledger.Book{}
	round, err := RunRound(book, market.Fixture(), Config{Qty: 1, ProfitPad: 0.15, VolPad: 0.05}, types.TakerBuy)
	if err != nil {
		t.Fatal(err)
	}
	if !almostEqual(round.Quote.HalfSpread, 0.25) || !almostEqual(round.RFQ.Price, 600.30) {
		t.Fatalf("half=%f rfq=%f", round.Quote.HalfSpread, round.RFQ.Price)
	}
}

func TestMetricsCountRejectAndFill(t *testing.T) {
	desk := &metrics.Desk{}
	book := &ledger.Book{}
	_, err := RunRound(book, market.Fixture(), Config{Qty: 2, ProfitPad: 0.15, MaxOrderQty: 1, Metrics: desk}, types.TakerBuy)
	if err != nil {
		t.Fatal(err)
	}
	book2 := &ledger.Book{}
	_, err = RunRound(book2, market.Fixture(), Config{Qty: 1, ProfitPad: 0.15, Metrics: desk}, types.TakerBuy)
	if err != nil {
		t.Fatal(err)
	}
	s := desk.Snapshot()
	if s.Rounds != 2 || s.Rejects != 1 || s.Fills != 1 {
		t.Fatalf("%+v", s)
	}
}

func TestRejectDailyLoss(t *testing.T) {
	book := &ledger.Book{Realized: -10}
	round, err := RunRound(book, market.Fixture(), Config{Qty: 1, ProfitPad: 0.15, MaxDailyLoss: 1}, types.TakerBuy)
	if err != nil {
		t.Fatal(err)
	}
	if round.Status != StatusRejected {
		t.Fatalf("status=%s", round.Status)
	}
}

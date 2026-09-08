package engine

import (
	"fmt"
	"time"

	"rfq-mm/internal/audit"
	"rfq-mm/internal/hedge"
	"rfq-mm/internal/ledger"
	"rfq-mm/internal/market"
	"rfq-mm/internal/metrics"
	"rfq-mm/internal/quote"
	"rfq-mm/internal/rfq"
	"rfq-mm/internal/risk"
	"rfq-mm/internal/types"
)

type Status string

const (
	StatusFilledHedged   Status = "filled_hedged"
	StatusFilledUnhedged Status = "filled_unhedged"
	StatusRejected       Status = "rejected"
)

type Config struct {
	Qty             float64
	ProfitPad       float64
	Fee             float64
	SkewPerBNB      float64
	MaxOrderQty     float64
	MaxAbsInventory float64
	MinHalfSpread   float64
	HalfSpread      float64
	MaxVenueSpread  float64
	MaxBookAge      time.Duration
	MaxDailyLoss    float64
	MaxUnhedged     int
	KillSwitch      string
	VolPad          float64
	QuoteTTL        time.Duration
	HitDelay        time.Duration
	SlipTicks       int
	TickSize        float64
	Hedger          hedge.Executor
	Journal         audit.Writer
	Metrics         *metrics.Desk
	Now             func() time.Time
}

type Desk struct {
	Unhedged int
}

func (c Config) normalized() Config {
	if c.Qty == 0 {
		c.Qty = 1
	}
	if c.ProfitPad == 0 && c.HalfSpread == 0 {
		c.ProfitPad = 0.15
	}
	if c.Hedger == nil {
		c.Hedger = hedge.Immediate{SlipTicks: c.SlipTicks, Tick: c.TickSize}
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	return c
}

type Round struct {
	ID         string
	Action     types.TakerAction
	Status     Status
	Reject     string
	HedgeErr   string
	Decisions  []string
	Market     market.Book
	Quote      quote.Quote
	RFQ        ledger.Fill
	Hedge      ledger.Fill
	AfterRFQ   ledger.Snapshot
	AfterHedge ledger.Snapshot
	Start      ledger.Snapshot
}

func RunRound(book *ledger.Book, mkt market.Book, cfg Config, action types.TakerAction) (Round, error) {
	return RunRoundDesk(book, mkt, cfg, action, nil)
}

func RunRoundDesk(book *ledger.Book, mkt market.Book, cfg Config, action types.TakerAction, desk *Desk) (Round, error) {
	if book == nil {
		return Round{}, fmt.Errorf("nil ledger")
	}
	cfg = cfg.normalized()
	start := book.Snapshot(mkt.Mid())
	round := Round{ID: fmt.Sprintf("%d", cfg.Now().UTC().UnixNano()), Action: action, Market: mkt, Start: start}

	reject := func(reason string) (Round, error) {
		round.Status = StatusRejected
		round.Reject = reason
		round.Decisions = append(round.Decisions, "reject: "+reason)
		observe(cfg, book, round, start)
		writeJournal(cfg.Journal, round)
		return round, nil
	}

	if err := risk.CheckKillSwitch(cfg.KillSwitch); err != nil {
		return reject(err.Error())
	}
	if err := risk.CheckLoss(book.Realized, cfg.MaxDailyLoss); err != nil {
		return reject(err.Error())
	}
	if desk != nil {
		if err := risk.CheckUnhedged(desk.Unhedged, cfg.MaxUnhedged); err != nil {
			return reject(err.Error())
		}
	}
	if err := mkt.Validate(); err != nil {
		return reject(err.Error())
	}
	if err := risk.CheckFresh(mkt, cfg.MaxBookAge, cfg.Now()); err != nil {
		return reject(err.Error())
	}
	if err := risk.CheckVenueSpread(mkt.Spread(), cfg.MaxVenueSpread); err != nil {
		return reject(err.Error())
	}

	q, err := quote.Make(mkt, quote.Params{
		Qty:                cfg.Qty,
		ProfitPad:          cfg.ProfitPad,
		FeePerUnit:         cfg.Fee,
		Inventory:          book.BNB,
		SkewPerBNB:         cfg.SkewPerBNB,
		MinHalfSpread:      cfg.MinHalfSpread,
		HalfSpreadOverride: cfg.HalfSpread,
		VolPad:             cfg.VolPad,
		TTL:                cfg.QuoteTTL,
		Now:                cfg.Now(),
	})
	if err != nil {
		return Round{}, err
	}
	hitAt := cfg.Now()
	if cfg.HitDelay > 0 {
		hitAt = hitAt.Add(cfg.HitDelay)
	}
	if !q.Alive(hitAt) {
		return reject("quote expired")
	}
	q.BidEnabled, q.AskEnabled = risk.Sides(book.BNB, cfg.MaxAbsInventory)
	round.Quote = q
	round.Decisions = append(round.Decisions, fmt.Sprintf("quote bid=%.8f ask=%.8f skew=%.8f half=%.8f ttl=%s", q.Bid, q.Ask, q.Skew, q.HalfSpread, cfg.QuoteTTL))

	if err := risk.CheckQty(cfg.Qty, cfg.MaxOrderQty); err != nil {
		return reject(err.Error())
	}
	if err := risk.CheckHit(action, q.BidEnabled, q.AskEnabled); err != nil {
		return reject(err.Error())
	}
	projected := risk.ProjectedInventory(book.BNB, action, cfg.Qty)
	if err := risk.CheckInventory(projected, cfg.MaxAbsInventory); err != nil {
		return reject(err.Error())
	}

	rfqFill, err := rfq.FillFromTaker(action, q, cfg.Fee)
	if err != nil {
		return Round{}, err
	}
	book.Apply(rfqFill)
	round.RFQ = rfqFill
	round.AfterRFQ = book.Snapshot(q.Mid)
	round.Decisions = append(round.Decisions, fmt.Sprintf("rfq mm_%s qty=%.8f px=%.8f", rfqFill.Side, rfqFill.Qty, rfqFill.Price))

	hedgeFill, herr := cfg.Hedger.Hedge(book.BNB, mkt, cfg.Fee)
	if herr != nil {
		round.Status = StatusFilledUnhedged
		round.HedgeErr = herr.Error()
		round.AfterHedge = book.Snapshot(q.Mid)
		round.Decisions = append(round.Decisions, "hedge_fail: "+herr.Error())
		if desk != nil {
			desk.Unhedged++
		}
		observe(cfg, book, round, start)
		writeJournal(cfg.Journal, round)
		return round, nil
	}
	if hedgeFill.Qty > 0 {
		book.Apply(hedgeFill)
		round.Hedge = hedgeFill
		round.Decisions = append(round.Decisions, fmt.Sprintf("hedge mm_%s qty=%.8f px=%.8f order=%d", hedgeFill.Side, hedgeFill.Qty, hedgeFill.Price, hedgeFill.OrderID))
	} else {
		round.Decisions = append(round.Decisions, "hedge_skip_flat")
	}
	round.AfterHedge = book.Snapshot(q.Mid)
	round.Status = StatusFilledHedged
	if desk != nil {
		desk.Unhedged = 0
	}
	observe(cfg, book, round, start)
	writeJournal(cfg.Journal, round)
	return round, nil
}

func observe(cfg Config, book *ledger.Book, r Round, start ledger.Snapshot) {
	if cfg.Metrics == nil {
		return
	}
	fee := r.RFQ.Fee + r.Hedge.Fee
	slip := 0.0
	if r.Hedge.Qty > 0 {
		slip = r.Hedge.Price - r.Hedge.TheoryPx
	}
	eq := start.Equity
	if r.Status != StatusRejected {
		eq = r.AfterHedge.Equity
	}
	unhedged := 0.0
	if r.Status == StatusFilledUnhedged && book != nil {
		unhedged = book.BNB
	}
	cfg.Metrics.Observe(string(r.Status), slip, fee, eq-start.Equity, unhedged, cfg.Now())
}

func writeJournal(w audit.Writer, r Round) {
	if w == nil {
		return
	}
	_ = w.Write(audit.Event{
		Kind:    "round",
		RoundID: r.ID,
		Payload: map[string]any{
			"action":  r.Action,
			"status":  r.Status,
			"reject":  r.Reject,
			"hedge":   r.HedgeErr,
			"steps":   r.Decisions,
			"equity":  r.AfterHedge.Equity,
			"bnb":     r.AfterHedge.BNB,
			"orderId": r.Hedge.OrderID,
		},
	})
}

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"rfq-mm/internal/audit"
	"rfq-mm/internal/engine"
	"rfq-mm/internal/hedge"
	"rfq-mm/internal/ledger"
	"rfq-mm/internal/market"
	"rfq-mm/internal/metrics"
	"rfq-mm/internal/types"
)

func main() {
	live := flag.Bool("live", false, "use Binance REST bookTicker instead of the mock fixture")
	symbol := flag.String("symbol", "BNBUSDT", "Binance symbol when -live is set")
	scenario := flag.String("scenario", "all", "happy | stress | guards | all")
	outDir := flag.String("out", "reports", "directory for text/json reports")
	slipTicks := flag.Int("slip-ticks", 0, "hedge adverse ticks (0 keeps +0.15)")
	tick := flag.Float64("tick", 0.01, "tick size used with -slip-ticks")
	volPad := flag.Float64("vol-pad", 0, "extra half-spread from volatility")
	ttl := flag.Duration("ttl", 0, "quote lifetime (0 = no expiry)")
	hitDelay := flag.Duration("hit-delay", 0, "simulated delay between quote and hit")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		slog.Error("mkdir reports", "err", err)
		os.Exit(1)
	}

	journal := audit.NewJSONL(filepath.Join(*outDir, "journal_stage1.jsonl"))
	desk := &metrics.Desk{}
	extra := extraCfg{SlipTicks: *slipTicks, TickSize: *tick, VolPad: *volPad, QuoteTTL: *ttl, HitDelay: *hitDelay}
	mkt := market.Fixture()
	if *live {
		liveBook, err := market.FetchBinanceBookTicker(*symbol)
		if err != nil {
			slog.Error("fetch live book", "err", err)
			os.Exit(1)
		}
		mkt = liveBook
		slog.Info("market", "source", mkt.Source, "bid", mkt.Bid, "ask", mkt.Ask)
	}

	var sections []section
	switch strings.ToLower(*scenario) {
	case "happy":
		sections = []section{runHappy(mkt, journal, desk, extra)}
	case "stress":
		sections = []section{runStress(mkt, journal, desk, extra)}
	case "guards":
		sections = []section{runGuards(mkt, journal, desk, extra)}
	case "all":
		sections = []section{runHappy(mkt, journal, desk, extra), runStress(mkt, journal, desk, extra), runGuards(mkt, journal, desk, extra)}
	default:
		fmt.Fprintf(os.Stderr, "unknown scenario %q\n", *scenario)
		os.Exit(2)
	}

	text := render(mkt, sections)
	fmt.Print(text)

	stamp := time.Now().Format("20060102-150405")
	txtPath := filepath.Join(*outDir, "stage1_"+*scenario+".txt")
	jsPath := filepath.Join(*outDir, "stage1_"+*scenario+".json")
	if err := os.WriteFile(txtPath, []byte(text), 0o644); err != nil {
		slog.Error("write text report", "err", err)
		os.Exit(1)
	}
	payload, _ := json.MarshalIndent(struct {
		Generated time.Time `json:"generated"`
		Market    market.Book
		Sections  []section `json:"sections"`
	}{time.Now().UTC(), mkt, sections}, "", "  ")
	if err := os.WriteFile(jsPath, payload, 0o644); err != nil {
		slog.Error("write json report", "err", err)
		os.Exit(1)
	}
	deskPath := filepath.Join(*outDir, "desk_stage1.json")
	if err := desk.Write(deskPath); err != nil {
		slog.Error("write desk metrics", "err", err)
		os.Exit(1)
	}
	slog.Info("wrote reports", "txt", txtPath, "json", jsPath, "desk", deskPath, "stamp", stamp)
}

type section struct {
	Name   string         `json:"name"`
	Note   string         `json:"note"`
	Rounds []engine.Round `json:"rounds"`
	Final  ledger.Snapshot `json:"final"`
}

type extraCfg struct {
	SlipTicks int
	TickSize  float64
	VolPad    float64
	QuoteTTL  time.Duration
	HitDelay  time.Duration
}

func baseCfg(j audit.Writer, d *metrics.Desk, extra extraCfg) engine.Config {
	return engine.Config{
		Qty:             1,
		ProfitPad:       0.15,
		SkewPerBNB:      0.05,
		MaxOrderQty:     5,
		MaxAbsInventory: 2,
		Journal:         j,
		Metrics:         d,
		SlipTicks:       extra.SlipTicks,
		TickSize:        extra.TickSize,
		VolPad:          extra.VolPad,
		QuoteTTL:        extra.QuoteTTL,
		HitDelay:        extra.HitDelay,
	}
}

func runHappy(mkt market.Book, j audit.Writer, d *metrics.Desk, extra extraCfg) section {
	book := &ledger.Book{}
	cfg := baseCfg(j, d, extra)
	rounds := mustRun(book, mkt, cfg, types.TakerBuy, types.TakerSell)
	return section{
		Name:   "A. happy path",
		Note:   "Both sides fill and flatten. Locked PnL is RFQ spread minus venue half-spread.",
		Rounds: rounds,
		Final:  book.Snapshot(mkt.Mid()),
	}
}

func runStress(mkt market.Book, j audit.Writer, d *metrics.Desk, extra extraCfg) section {
	book := &ledger.Book{}
	cfg := baseCfg(j, d, extra)
	cfg.Hedger = &hedge.FailOnce{Left: 1, Reason: "binance timeout"}
	rounds := mustRun(book, mkt, cfg,
		types.TakerSell, // hedge fails, leftover long 1
		types.TakerBuy,  // quote is skewed down; RFQ itself flattens
	)
	// oversize must reject without touching the book
	cfg.Hedger = hedge.Immediate{}
	cfg.Qty = 9
	over, err := engine.RunRound(book, mkt, cfg, types.TakerBuy)
	if err != nil {
		slog.Error("stress oversize", "err", err)
		os.Exit(1)
	}
	rounds = append(rounds, over)
	return section{
		Name:   "B. hedge fail + skew + reject",
		Note:   "Failed hedge leaves inventory. Next quote skews. Oversize is rejected and does not fill.",
		Rounds: rounds,
		Final:  book.Snapshot(mkt.Mid()),
	}
}

func runGuards(mkt market.Book, j audit.Writer, d *metrics.Desk, extra extraCfg) section {
	book := &ledger.Book{}
	stale := mkt
	stale.Ts = time.Unix(1, 0)
	stale.Source = "stale-mock"
	r1, _ := engine.RunRound(book, stale, engine.Config{
		Qty: 1, ProfitPad: 0.15, MaxBookAge: time.Second, Journal: j, Metrics: d,
		Now: func() time.Time { return time.Unix(30, 0) },
	}, types.TakerBuy)

	wide := mkt
	wide.Ask = mkt.Bid + 5
	r2, _ := engine.RunRound(book, wide, engine.Config{
		Qty: 1, ProfitPad: 0.15, MaxVenueSpread: 0.2, Journal: j, Metrics: d,
		VolPad: extra.VolPad, QuoteTTL: extra.QuoteTTL, HitDelay: extra.HitDelay,
	}, types.TakerBuy)

	lossBook := &ledger.Book{Realized: -25}
	r3, _ := engine.RunRound(lossBook, mkt, engine.Config{
		Qty: 1, ProfitPad: 0.15, MaxDailyLoss: 10, Journal: j, Metrics: d,
	}, types.TakerBuy)

	return section{
		Name:   "C. production guards",
		Note:   "Stale book, blown venue spread, and daily loss all reject without filling.",
		Rounds: []engine.Round{r1, r2, r3},
		Final:  book.Snapshot(mkt.Mid()),
	}
}

func mustRun(book *ledger.Book, mkt market.Book, cfg engine.Config, actions ...types.TakerAction) []engine.Round {
	out := make([]engine.Round, 0, len(actions))
	for _, action := range actions {
		round, err := engine.RunRound(book, mkt, cfg, action)
		if err != nil {
			slog.Error("run round", "action", action, "err", err)
			os.Exit(1)
		}
		out = append(out, round)
	}
	return out
}

func render(mkt market.Book, sections []section) string {
	var b strings.Builder
	fmt.Fprintf(&b, "RFQ MM Stage 1\n")
	fmt.Fprintf(&b, "market source=%s bid=%.4f ask=%.4f mid=%.4f venue_spread=%.4f\n",
		mkt.Source, mkt.Bid, mkt.Ask, mkt.Mid(), mkt.Spread())
	fmt.Fprintf(&b, "quote = reservation ± (venue_half + 2*fee + profit_pad); reservation = mid - skew*inventory\n\n")
	for _, sec := range sections {
		fmt.Fprintf(&b, "## %s\n%s\n\n", sec.Name, sec.Note)
		for i, r := range sec.Rounds {
			fmt.Fprintf(&b, "--- round %d: %s [%s] ---\n", i+1, r.Action, r.Status)
			fmt.Fprintf(&b, "1) quote   bid=%.4f ask=%.4f mid=%.4f reservation=%.4f half=%.4f skew=%.4f bid_on=%v ask_on=%v\n",
				r.Quote.Bid, r.Quote.Ask, r.Quote.Mid, r.Quote.Reservation, r.Quote.HalfSpread, r.Quote.Skew, r.Quote.BidEnabled, r.Quote.AskEnabled)
			if r.Status == engine.StatusRejected {
				fmt.Fprintf(&b, "2) reject  %s\n\n", r.Reject)
				continue
			}
			fmt.Fprintf(&b, "2) fill    taker=%s => MM %s %.4f @ %.4f\n", r.Action, r.RFQ.Side, r.RFQ.Qty, r.RFQ.Price)
			fmt.Fprintf(&b, "   after RFQ   bnb=%+.4f usdt=%+.4f realized=%.4f unrealized=%.4f equity=%.4f\n",
				r.AfterRFQ.BNB, r.AfterRFQ.USDT, r.AfterRFQ.Realized, r.AfterRFQ.Unrealized, r.AfterRFQ.Equity)
			if r.HedgeErr != "" {
				fmt.Fprintf(&b, "3) hedge   FAILED: %s\n", r.HedgeErr)
			} else if r.Hedge.Qty == 0 {
				fmt.Fprintf(&b, "3) hedge   skip (already flat after RFQ)\n")
			} else {
				fmt.Fprintf(&b, "3) hedge   MM %s %.4f @ %.4f\n", r.Hedge.Side, r.Hedge.Qty, r.Hedge.Price)
			}
			fmt.Fprintf(&b, "4) book    bnb=%+.4f usdt=%+.4f realized=%.4f unrealized=%.4f equity=%.4f\n\n",
				r.AfterHedge.BNB, r.AfterHedge.USDT, r.AfterHedge.Realized, r.AfterHedge.Unrealized, r.AfterHedge.Equity)
		}
		fmt.Fprintf(&b, "final bnb=%+.8f usdt=%+.8f realized=%.8f unrealized=%.8f equity=%.8f\n\n",
			sec.Final.BNB, sec.Final.USDT, sec.Final.Realized, sec.Final.Unrealized, sec.Final.Equity)
	}
	return b.String()
}

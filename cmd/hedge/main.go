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
	"rfq-mm/internal/binance"
	"rfq-mm/internal/dotenv"
	"rfq-mm/internal/engine"
	"rfq-mm/internal/hedge"
	"rfq-mm/internal/ledger"
	"rfq-mm/internal/market"
	"rfq-mm/internal/metrics"
	"rfq-mm/internal/store"
	"rfq-mm/internal/types"
	"rfq-mm/internal/wallet"
)

func main() {
	mode := flag.String("mode", "public", "public | dry-run | fail | order")
	symbol := flag.String("symbol", "BNBUSDT", "spot symbol")
	outDir := flag.String("out", "reports", "report directory")
	send := flag.Bool("send", false, "actually POST /api/v3/order (testnet keys required)")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if err := dotenv.Load(".env"); err != nil {
		slog.Error("dotenv", "err", err)
		os.Exit(1)
	}

	client := binance.NewFromEnv()
	if err := binance.GuardLiveSend(client.BaseURL, *send); err != nil {
		slog.Error("guard", "err", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		slog.Error("mkdir", "err", err)
		os.Exit(1)
	}
	report, err := run(*mode, *symbol, *send, client, *outDir, audit.NewJSONL(filepath.Join(*outDir, "journal_stage2.jsonl")))
	if err != nil {
		slog.Error("stage2", "err", err)
		os.Exit(1)
	}
	fmt.Print(report)

	name := "stage2_" + strings.ReplaceAll(*mode, "-", "")
	txt := filepath.Join(*outDir, name+".txt")
	if err := os.WriteFile(txt, []byte(report), 0o644); err != nil {
		slog.Error("write report", "err", err)
		os.Exit(1)
	}
	slog.Info("wrote report", "path", txt)
}

func run(mode, symbol string, send bool, client *binance.Client, outDir string, journal audit.Writer) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "RFQ MM Stage 2 — %s\n", mode)
	fmt.Fprintf(&b, "base=%s symbol=%s send=%v generated=%s\n\n", client.BaseURL, symbol, send, time.Now().Format(time.RFC3339))

	filters, book, src, err := loadVenue(client, symbol)
	fmt.Fprintf(&b, "venue source=%s\n", src)
	if err != nil {
		fmt.Fprintf(&b, "live venue unavailable (%v); using documented BNBUSDT fixture filters\n", err)
		filters = fixtureFilters()
		book = market.Fixture()
		book.Source = "fixture-fallback"
	}
	fmt.Fprintf(&b, "book bid=%.8f ask=%.8f mid=%.8f\n", book.Bid, book.Ask, book.Mid())
	fmt.Fprintf(&b, "filters tick=%s step=%s minQty=%s minNotional=%s\n\n",
		binance.FormatStep(filters.TickSize, filters.TickSize),
		binance.FormatStep(filters.StepSize, filters.StepSize),
		binance.FormatStep(filters.MinQty, filters.StepSize),
		binance.FormatStep(filters.MinNotional, filters.TickSize),
	)

	switch mode {
	case "public":
		writePublic(&b, filters, book)
	case "fail":
		writeFail(&b, filters, book, client, symbol, send)
	case "dry-run", "order":
		if err := writeRound(&b, filters, book, client, symbol, mode == "order" && send, outDir, journal); err != nil {
			return b.String(), err
		}
	default:
		return "", fmt.Errorf("unknown mode %q", mode)
	}
	return b.String(), nil
}

func loadVenue(client *binance.Client, symbol string) (binance.Filters, market.Book, string, error) {
	filters, err := client.ExchangeFilters(symbol)
	if err != nil && client.BaseURL != binance.ProdBase {
		prod := *client
		prod.BaseURL = binance.ProdBase
		filters, err = prod.ExchangeFilters(symbol)
		if err != nil {
			return binance.Filters{}, market.Book{}, client.BaseURL, err
		}
		book, berr := prod.BookTicker(symbol)
		return filters, book, prod.BaseURL + " (public fallback)", berr
	}
	if err != nil {
		return binance.Filters{}, market.Book{}, client.BaseURL, err
	}
	book, err := client.BookTicker(symbol)
	return filters, book, client.BaseURL, err
}

func fixtureFilters() binance.Filters {
	return binance.Filters{
		Symbol:      "BNBUSDT",
		TickSize:    0.01,
		StepSize:    0.001,
		MinQty:      0.001,
		MaxQty:      900000,
		MinPrice:    0.01,
		MaxPrice:    100000,
		MinNotional: 5,
	}
}

func writePublic(b *strings.Builder, f binance.Filters, book market.Book) {
	fmt.Fprintf(b, "## why theory qty cannot be sent raw\n")
	raw := 1.0007
	aligned, err := f.AlignQty(raw)
	fmt.Fprintf(b, "theory qty=%.8f  aligned=%s  err=%v\n", raw, binance.FormatStep(aligned, f.StepSize), err)
	minQ, err := f.MinHedgeQty(book.Ask)
	fmt.Fprintf(b, "min hedge qty at ask %.4f = %s err=%v\n", book.Ask, binance.FormatStep(minQ, f.StepSize), err)
	fmt.Fprintf(b, "choice: REST bookTicker + REST exchangeInfo, not WebSocket.\n")
	fmt.Fprintf(b, "hedge is event-driven after an RFQ fill; a streaming book is not required to prove filters and order wiring.\n")
}

func writeFail(b *strings.Builder, f binance.Filters, book market.Book, client *binance.Client, symbol string, send bool) {
	fmt.Fprintf(b, "## manufactured filter failure (local)\n")
	badQty := 1.0000001
	if err := f.CheckLot(badQty); err != nil {
		fmt.Fprintf(b, "LOT_SIZE reject: %v\n", err)
	}
	tiny := f.MinQty
	if err := f.CheckNotional(tiny, book.Ask); err != nil {
		fmt.Fprintf(b, "MIN_NOTIONAL reject: %v\n", err)
	} else {
		fmt.Fprintf(b, "minQty %s * ask %.4f clears notional on this book\n", binance.FormatStep(tiny, f.StepSize), book.Ask)
	}

	fmt.Fprintf(b, "\n## exchange failure\n")
	if !send {
		fmt.Fprintf(b, "skipped POST. Re-run with -mode fail -send and testnet keys to record a live -1013.\n")
		fmt.Fprintf(b, "we refuse to send a known-illegal order unless -send is set.\n")
		return
	}
	fmt.Fprintf(b, "sending unaligned MARKET qty=%g on purpose\n", badQty)
	_, err := client.PlaceMarket(symbol, "BUY", badQty, 0.0000001)
	if err != nil {
		fmt.Fprintf(b, "exchange rejected: %v\n", err)
		return
	}
	fmt.Fprintf(b, "unexpected accept; exchange filters may be looser on this venue\n")
}

func writeRound(b *strings.Builder, f binance.Filters, book market.Book, client *binance.Client, symbol string, send bool, outDir string, journal audit.Writer) error {
	minQ, err := f.MinHedgeQty(book.Ask)
	if err != nil {
		return err
	}
	st, err := store.Open(filepath.Join(outDir, "orders.json"))
	if err != nil {
		return err
	}
	bookLedger := &ledger.Book{}
	desk := &metrics.Desk{}
	if send && client != nil {
		rep := hedge.RecoverPending(st, client, symbol, bookLedger, book.Mid(), slog.Default())
		if len(rep.Notes) > 0 {
			fmt.Fprintf(b, "## recover pending orders\n%s\n\n", strings.Join(rep.Notes, "\n"))
		}
	}
	live := hedge.Live{
		Client:  client,
		Symbol:  symbol,
		Filters: f,
		DryRun:  !send,
		Log:     slog.Default(),
		Store:   st,
	}
	cfg := engine.Config{
		Qty:             minQ,
		ProfitPad:       0.15,
		SkewPerBNB:      0.05,
		MaxOrderQty:     minQ * 10,
		MaxAbsInventory: minQ * 10,
		MaxVenueSpread:  book.Spread() * 20,
		MaxBookAge:      5 * time.Second,
		Hedger:          live,
		Journal:         journal,
		Metrics:         desk,
	}
	var watch wallet.Watch
	watching := false
	sessionFills := len(bookLedger.Fills)
	if send && client != nil {
		if acct, aerr := client.Account(); aerr == nil {
			watch = wallet.New(acct)
			watching = true
			fmt.Fprintf(b, "wallet start BNB=%.8f USDT=%.8f\n", watch.StartBNB, watch.StartUSDT)
		} else {
			fmt.Fprintf(b, "wallet start skipped: %v\n", aerr)
		}
	}
	round, err := engine.RunRound(bookLedger, book, cfg, types.TakerBuy)
	if err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(round, "", "  ")
	fmt.Fprintf(b, "## engine round taker_buy qty=%s send=%v status=%s\n", binance.FormatStep(minQ, f.StepSize), send, round.Status)
	fmt.Fprintf(b, "rfq mm_side=%s px=%.8f qty=%.8f\n", round.RFQ.Side, round.RFQ.Price, round.RFQ.Qty)
	if round.HedgeErr != "" {
		fmt.Fprintf(b, "hedge error: %s\n", round.HedgeErr)
	} else {
		fmt.Fprintf(b, "hedge source=%s mm_side=%s px=%.8f qty=%.8f order=%d client=%s slip=%.8f\n",
			round.Hedge.Source, round.Hedge.Side, round.Hedge.Price, round.Hedge.Qty, round.Hedge.OrderID, round.Hedge.ClientOrderID, round.Hedge.Price-round.Hedge.TheoryPx)
	}
	if len(round.Decisions) > 0 {
		fmt.Fprintf(b, "decisions: %s\n", strings.Join(round.Decisions, " | "))
	}
	fmt.Fprintf(b, "after hedge bnb=%+.8f usdt=%+.8f realized=%.8f equity=%.8f\n\n",
		round.AfterHedge.BNB, round.AfterHedge.USDT, round.AfterHedge.Realized, round.AfterHedge.Equity)
	if watching && client != nil {
		acct, aerr := client.Account()
		if aerr != nil {
			fmt.Fprintf(b, "wallet check skipped: %v\n", aerr)
		} else if err := watch.Check(acct, wallet.ExchangeFills(bookLedger.Fills[sessionFills:]), 0.002, 0.05); err != nil {
			fmt.Fprintf(b, "wallet check: %v\n", err)
		} else {
			fmt.Fprintf(b, "wallet check: ok exch BNB=%.8f USDT=%.8f\n", acct.FreeOf("BNB"), acct.FreeOf("USDT"))
		}
	}
	snap := desk.Snapshot()
	fmt.Fprintf(b, "desk rounds=%d rejects=%d hedge_fails=%d avg_slip=%.8f total_pnl=%.8f\n",
		snap.Rounds, snap.Rejects, snap.HedgeFails, snap.AvgSlip, snap.TotalPnL)
	if err := desk.Write(filepath.Join(outDir, "desk_stage2.json")); err != nil {
		return err
	}
	fmt.Fprintf(b, "%s\n", raw)
	return nil
}

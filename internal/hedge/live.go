package hedge

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"rfq-mm/internal/binance"
	"rfq-mm/internal/ledger"
	"rfq-mm/internal/market"
	"rfq-mm/internal/store"
	"rfq-mm/internal/types"
)

// Live sends the flatten order to Binance.
// Stage 1 keeps using Immediate / FailOnce. This type is Stage 2 only.
type Live struct {
	Client  *binance.Client
	Symbol  string
	Filters binance.Filters
	DryRun  bool
	Log     *slog.Logger
	Store   *store.File
}

func (l Live) logger() *slog.Logger {
	if l.Log != nil {
		return l.Log
	}
	return slog.Default()
}

func (l Live) Hedge(inventory float64, m market.Book, fee float64) (ledger.Fill, error) {
	planned, err := FromInventory(inventory, m, fee)
	if err != nil {
		return ledger.Fill{}, err
	}
	if planned.Qty == 0 {
		return planned, nil
	}

	qty, err := l.Filters.AlignQty(planned.Qty)
	if err != nil {
		return ledger.Fill{}, fmt.Errorf("align qty: %w", err)
	}
	if err := l.Filters.CheckNotional(qty, planned.Price); err != nil {
		return ledger.Fill{}, fmt.Errorf("notional: %w", err)
	}

	side := strings.ToUpper(string(planned.Side))
	l.logger().Info("hedge_plan",
		"symbol", l.Symbol,
		"mm_side", planned.Side,
		"theory_qty", planned.Qty,
		"aligned_qty", qty,
		"theory_px", planned.Price,
		"dry_run", l.DryRun,
	)

	if l.DryRun {
		planned.Qty = qty
		planned.Source = "hedge-dryrun"
		planned.TheoryPx = planned.Price
		return planned, nil
	}
	if l.Client == nil {
		return ledger.Fill{}, fmt.Errorf("nil binance client")
	}

	acct, err := l.Client.Account()
	if err != nil {
		return ledger.Fill{}, fmt.Errorf("account precheck: %w", err)
	}
	if err := acct.CheckHedge(side, qty, planned.Price); err != nil {
		return ledger.Fill{}, err
	}

	cid := binance.NewClientOrderID("hedge")
	rec := store.Record{
		ClientOrderID: cid,
		Symbol:        l.Symbol,
		Side:          side,
		Qty:           qty,
		Price:         planned.Price,
		Status:        store.New,
	}
	if err := l.Store.Put(rec); err != nil {
		return ledger.Fill{}, fmt.Errorf("persist before send: %w", err)
	}
	rec.Status = store.Sent
	if err := l.Store.Put(rec); err != nil {
		return ledger.Fill{}, fmt.Errorf("persist sent: %w", err)
	}

	ack, err := l.Client.PlaceAndReconcile(l.Symbol, side, qty, l.Filters.StepSize, cid)
	if err != nil {
		rec.Status = store.Dead
		if binance.Uncertain(err) {
			rec.Status = store.Uncertain
		}
		rec.Note = err.Error()
		_ = l.Store.Put(rec)
		return ledger.Fill{}, err
	}

	fill, ferr := FillFromAck(ack, cid, planned.Price, m.Mid(), qty)
	if ferr != nil {
		rec.Status = store.Uncertain
		rec.OrderID = ack.OrderID
		rec.Note = ferr.Error()
		_ = l.Store.Put(rec)
		return ledger.Fill{}, fmt.Errorf("fill without price after order %d: %w", ack.OrderID, ferr)
	}
	if fill.Side != types.SideBuy && fill.Side != types.SideSell {
		fill.Side = planned.Side
	}
	rec.OrderID = ack.OrderID
	rec.ExecQty = fill.Qty
	rec.Status = StatusFromAck(ack)
	if rec.Status == store.Filled && fill.Qty+1e-12 < qty {
		rec.Status = store.Partial
	}
	_ = l.Store.Put(rec)

	l.logger().Info("hedge_fill",
		"orderId", ack.OrderID,
		"clientOrderId", cid,
		"status", ack.Status,
		"local_status", rec.Status,
		"theory_px", planned.Price,
		"fill_px", fill.Price,
		"slippage", fill.Price-planned.Price,
		"exec_qty", fill.Qty,
		"residual", qty-fill.Qty,
		"fee_usdt", fill.Fee,
	)
	return fill, nil
}

func parseExecQty(s string, fallback float64) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

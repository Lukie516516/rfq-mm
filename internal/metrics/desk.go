package metrics

import (
	"encoding/json"
	"math"
	"os"
	"time"
)

type Desk struct {
	Rounds       int       `json:"rounds"`
	Rejects      int       `json:"rejects"`
	HedgeFails   int       `json:"hedge_fails"`
	Fills        int       `json:"fills"`
	UnhedgedSec  float64   `json:"unhedged_seconds"`
	Slips        []float64 `json:"slips,omitempty"`
	SpreadPnL    float64   `json:"spread_pnl"`
	FeePnL       float64   `json:"fee_pnl"`
	InventoryPnL float64   `json:"inventory_pnl"`
	lastUnhedged time.Time
}

type Snapshot struct {
	Rounds         int     `json:"rounds"`
	Rejects        int     `json:"rejects"`
	HedgeFails     int     `json:"hedge_fails"`
	Fills          int     `json:"fills"`
	RejectRate     float64 `json:"reject_rate"`
	HedgeFailRate  float64 `json:"hedge_fail_rate"`
	UnhedgedSec    float64 `json:"unhedged_seconds"`
	AvgSlip        float64 `json:"avg_slip"`
	SpreadPnL      float64 `json:"spread_pnl"`
	FeePnL         float64 `json:"fee_pnl"`
	InventoryPnL   float64 `json:"inventory_pnl"`
	TotalPnL       float64 `json:"total_pnl"`
}

func (d *Desk) Observe(status string, slip, fee, equityDelta, unhedgedBNB float64, now time.Time) {
	if d == nil {
		return
	}
	d.Rounds++
	switch status {
	case "rejected":
		d.Rejects++
	case "filled_unhedged":
		d.HedgeFails++
		if d.lastUnhedged.IsZero() {
			d.lastUnhedged = now
		}
	case "filled_hedged":
		d.Fills++
		if !d.lastUnhedged.IsZero() {
			d.UnhedgedSec += now.Sub(d.lastUnhedged).Seconds()
			d.lastUnhedged = time.Time{}
		}
	}
	if slip != 0 {
		d.Slips = append(d.Slips, slip)
	}
	d.FeePnL -= fee
	if unhedgedBNB == 0 {
		d.SpreadPnL += equityDelta + fee
	} else {
		d.InventoryPnL += equityDelta
	}
}

func (d Desk) Snapshot() Snapshot {
	s := Snapshot{
		Rounds:       d.Rounds,
		Rejects:      d.Rejects,
		HedgeFails:   d.HedgeFails,
		Fills:        d.Fills,
		UnhedgedSec:  d.UnhedgedSec,
		SpreadPnL:    d.SpreadPnL,
		FeePnL:       d.FeePnL,
		InventoryPnL: d.InventoryPnL,
		TotalPnL:     d.SpreadPnL + d.FeePnL + d.InventoryPnL,
	}
	if d.Rounds > 0 {
		s.RejectRate = float64(d.Rejects) / float64(d.Rounds)
		s.HedgeFailRate = float64(d.HedgeFails) / float64(d.Rounds)
	}
	if len(d.Slips) > 0 {
		var sum float64
		for _, v := range d.Slips {
			sum += v
		}
		s.AvgSlip = sum / float64(len(d.Slips))
	}
	if math.Abs(s.TotalPnL) < 1e-12 {
		s.TotalPnL = 0
	}
	return s
}

func (d Desk) Write(path string) error {
	raw, err := json.MarshalIndent(d.Snapshot(), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

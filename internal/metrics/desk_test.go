package metrics

import (
	"testing"
	"time"
)

func TestObserveRatesAndPnL(t *testing.T) {
	d := &Desk{}
	now := time.Unix(0, 0)
	d.Observe("rejected", 0, 0, 0, 0, now)
	d.Observe("filled_unhedged", 0, 0, 0.2, 1, now.Add(time.Second))
	d.Observe("filled_hedged", 0.01, 0.02, 0.13, 0, now.Add(3*time.Second))
	s := d.Snapshot()
	if s.Rounds != 3 || s.Rejects != 1 || s.HedgeFails != 1 || s.Fills != 1 {
		t.Fatalf("%+v", s)
	}
	if s.RejectRate < 0.33 || s.RejectRate > 0.34 {
		t.Fatalf("reject rate %f", s.RejectRate)
	}
	if s.UnhedgedSec != 2 {
		t.Fatalf("unhedged sec=%f", s.UnhedgedSec)
	}
	if s.AvgSlip != 0.01 {
		t.Fatalf("slip=%f", s.AvgSlip)
	}
	if s.FeePnL != -0.02 {
		t.Fatalf("fee pnl=%f", s.FeePnL)
	}
	if s.SpreadPnL < 0.149 || s.SpreadPnL > 0.151 {
		t.Fatalf("spread pnl=%f want ~0.15", s.SpreadPnL)
	}
}

package risk

import (
	"fmt"
	"math"
	"os"
	"time"

	"rfq-mm/internal/market"
	"rfq-mm/internal/types"
)

type Limits struct {
	MaxOrderQty     float64
	MaxAbsInventory float64
}

func CheckQty(qty, maxOrderQty float64) error {
	if qty <= 0 {
		return fmt.Errorf("qty must be > 0")
	}
	if maxOrderQty > 0 && qty > maxOrderQty {
		return fmt.Errorf("qty %.8f exceeds max order %.8f", qty, maxOrderQty)
	}
	return nil
}

// ProjectedInventory is inventory after an RFQ fill, before hedge.
func ProjectedInventory(current float64, action types.TakerAction, qty float64) float64 {
	switch action {
	case types.TakerBuy:
		return current - qty
	case types.TakerSell:
		return current + qty
	default:
		return current
	}
}

func CheckInventory(projected, maxAbs float64) error {
	if maxAbs <= 0 {
		return nil
	}
	if math.Abs(projected) > maxAbs+1e-12 {
		return fmt.Errorf("projected inventory %.8f exceeds limit ±%.8f", projected, maxAbs)
	}
	return nil
}

// Sides reports which RFQ sides we still want to show.
// At max long we stop bidding (won't buy more). At max short we stop offering.
func Sides(inventory, maxAbs float64) (bidOK, askOK bool) {
	if maxAbs <= 0 {
		return true, true
	}
	return inventory < maxAbs-1e-12, inventory > -maxAbs+1e-12
}

func CheckVenueSpread(spread, maxSpread float64) error {
	if maxSpread <= 0 {
		return nil
	}
	if spread > maxSpread+1e-12 {
		return fmt.Errorf("venue spread %.8f wider than max %.8f", spread, maxSpread)
	}
	return nil
}

func CheckFresh(m market.Book, maxAge time.Duration, now time.Time) error {
	return m.Stale(maxAge, now)
}

func CheckKillSwitch(path string) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("kill switch present: %s", path)
	}
	return nil
}

func CheckLoss(realized, maxLoss float64) error {
	if maxLoss <= 0 {
		return nil
	}
	if realized < -maxLoss {
		return fmt.Errorf("realized %.8f breached daily loss limit %.8f", realized, maxLoss)
	}
	return nil
}

func CheckUnhedged(streak, max int) error {
	if max <= 0 {
		return nil
	}
	if streak >= max {
		return fmt.Errorf("unhedged streak %d reached stop %d", streak, max)
	}
	return nil
}

func CheckHit(action types.TakerAction, bidOK, askOK bool) error {
	switch action {
	case types.TakerBuy:
		if !askOK {
			return fmt.Errorf("ask disabled: inventory already short to the limit")
		}
	case types.TakerSell:
		if !bidOK {
			return fmt.Errorf("bid disabled: inventory already long to the limit")
		}
	}
	return nil
}

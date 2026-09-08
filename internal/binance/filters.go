package binance

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type Filters struct {
	Symbol      string
	TickSize    float64
	StepSize    float64
	MinQty      float64
	MaxQty      float64
	MinPrice    float64
	MaxPrice    float64
	MinNotional float64
	Raw         json.RawMessage
}

type exchangeInfo struct {
	Symbols []struct {
		Symbol  string          `json:"symbol"`
		Status  string          `json:"status"`
		Filters []map[string]any `json:"filters"`
	} `json:"symbols"`
}

func ParseExchangeInfo(body []byte, symbol string) (Filters, error) {
	var info exchangeInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return Filters{}, err
	}
	for _, s := range info.Symbols {
		if s.Symbol != symbol {
			continue
		}
		f := Filters{Symbol: symbol, Raw: body}
		for _, raw := range s.Filters {
			switch fmt.Sprint(raw["filterType"]) {
			case "PRICE_FILTER":
				f.TickSize = parseFloat(raw["tickSize"])
				f.MinPrice = parseFloat(raw["minPrice"])
				f.MaxPrice = parseFloat(raw["maxPrice"])
			case "LOT_SIZE":
				f.StepSize = parseFloat(raw["stepSize"])
				f.MinQty = parseFloat(raw["minQty"])
				f.MaxQty = parseFloat(raw["maxQty"])
			case "MIN_NOTIONAL", "NOTIONAL":
				if v := parseFloat(raw["minNotional"]); v > 0 {
					f.MinNotional = v
				}
			}
		}
		if f.StepSize <= 0 || f.TickSize <= 0 {
			return Filters{}, fmt.Errorf("incomplete filters for %s", symbol)
		}
		return f, nil
	}
	return Filters{}, fmt.Errorf("symbol %s not found", symbol)
}

func parseFloat(v any) float64 {
	switch x := v.(type) {
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	case float64:
		return x
	default:
		return 0
	}
}

func FloorToStep(value, step float64) float64 {
	if step <= 0 {
		return value
	}
	inv := 1 / step
	return math.Floor(value*inv+1e-8) / inv
}

func CeilToStep(value, step float64) float64 {
	if step <= 0 {
		return value
	}
	inv := 1 / step
	return math.Ceil(value*inv-1e-8) / inv
}

func FormatStep(value, step float64) string {
	prec := stepDecimals(step)
	s := strconv.FormatFloat(value, 'f', prec, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	if s == "" || s == "-0" {
		return "0"
	}
	return s
}

func stepDecimals(step float64) int {
	s := strings.TrimRight(strconv.FormatFloat(step, 'f', 12, 64), "0")
	i := strings.IndexByte(s, '.')
	if i < 0 {
		return 0
	}
	return len(s) - i - 1
}

func (f Filters) AlignQty(qty float64) (float64, error) {
	aligned := FloorToStep(qty, f.StepSize)
	if aligned+1e-12 < f.MinQty {
		return 0, fmt.Errorf("qty %s below minQty %s after stepSize %s",
			FormatStep(qty, f.StepSize), FormatStep(f.MinQty, f.StepSize), FormatStep(f.StepSize, f.StepSize))
	}
	if f.MaxQty > 0 && aligned > f.MaxQty+1e-12 {
		return 0, fmt.Errorf("qty %s above maxQty %s", FormatStep(aligned, f.StepSize), FormatStep(f.MaxQty, f.StepSize))
	}
	return aligned, nil
}

func (f Filters) AlignPrice(px float64) (float64, error) {
	aligned := FloorToStep(px, f.TickSize)
	if f.MinPrice > 0 && aligned < f.MinPrice-1e-12 {
		return 0, fmt.Errorf("price %s below minPrice %s", FormatStep(aligned, f.TickSize), FormatStep(f.MinPrice, f.TickSize))
	}
	if f.MaxPrice > 0 && aligned > f.MaxPrice+1e-12 {
		return 0, fmt.Errorf("price %s above maxPrice %s", FormatStep(aligned, f.TickSize), FormatStep(f.MaxPrice, f.TickSize))
	}
	return aligned, nil
}

func (f Filters) CheckNotional(qty, price float64) error {
	if f.MinNotional <= 0 {
		return nil
	}
	notional := qty * price
	if notional+1e-9 < f.MinNotional {
		return fmt.Errorf("notional %.8f = qty %s * px %s below minNotional %s",
			notional, FormatStep(qty, f.StepSize), FormatStep(price, f.TickSize), FormatStep(f.MinNotional, f.TickSize))
	}
	return nil
}

func (f Filters) MinHedgeQty(price float64) (float64, error) {
	if price <= 0 {
		return 0, fmt.Errorf("price must be > 0")
	}
	q := f.MinQty
	if f.MinNotional > 0 {
		need := CeilToStep(f.MinNotional/price, f.StepSize)
		if need > q {
			q = need
		}
	}
	if err := f.CheckNotional(q, price); err != nil {
		q = CeilToStep(f.MinNotional/price+f.StepSize, f.StepSize)
	}
	return q, f.CheckNotional(q, price)
}

func (f Filters) CheckLot(qty float64) error {
	if f.StepSize > 0 {
		n := qty / f.StepSize
		if math.Abs(n-math.Round(n)) > 1e-6 {
			return fmt.Errorf("qty %g is not a multiple of stepSize %s", qty, FormatStep(f.StepSize, f.StepSize))
		}
	}
	if qty+1e-12 < f.MinQty {
		return fmt.Errorf("qty %g below minQty %s", qty, FormatStep(f.MinQty, f.StepSize))
	}
	return nil
}

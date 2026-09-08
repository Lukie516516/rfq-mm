package money

import (
	"math"
	"math/bits"
)

// Scale is 1e8. Two-decimal crypto prices stay exact.
const Scale int64 = 100_000_000

func FromFloat(v float64) int64 {
	return int64(math.Round(v * float64(Scale)))
}

func ToFloat(v int64) float64 {
	return float64(v) / float64(Scale)
}

func Mul(qty, px int64) int64 {
	neg := (qty < 0) != (px < 0)
	hi, lo := bits.Mul64(uint64(Abs(qty)), uint64(Abs(px)))
	quo, _ := bits.Div64(hi, lo, uint64(Scale))
	out := int64(quo)
	if neg {
		return -out
	}
	return out
}

// Div is notional / qty → price, both at Scale.
func Div(notional, qty int64) int64 {
	if qty == 0 {
		return 0
	}
	neg := (notional < 0) != (qty < 0)
	hi, lo := bits.Mul64(uint64(Abs(notional)), uint64(Scale))
	quo, _ := bits.Div64(hi, lo, uint64(Abs(qty)))
	out := int64(quo)
	if neg {
		return -out
	}
	return out
}

func Abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

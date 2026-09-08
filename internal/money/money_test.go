package money

import "testing"

func TestRoundTrip(t *testing.T) {
	for _, v := range []float64{600.25, 600.10, 0.15, 599.85, 0.007} {
		if ToFloat(FromFloat(v)) != v && abs(ToFloat(FromFloat(v))-v) > 1e-10 {
			// 0.007 may not be exact in float; allow one ulp around scale
			if abs(ToFloat(FromFloat(v))-v) > 5e-9 {
				t.Fatalf("%g -> %d -> %g", v, FromFloat(v), ToFloat(FromFloat(v)))
			}
		}
	}
	if FromFloat(600.25)*FromFloat(1)/Scale != FromFloat(600.25) {
		t.Fatal("1 * px")
	}
	if Mul(FromFloat(1), FromFloat(600.25)) != FromFloat(600.25) {
		t.Fatal("mul 1 * 600.25")
	}
	if Div(FromFloat(1200.50), FromFloat(2)) != FromFloat(600.25) {
		t.Fatalf("div %d", Div(FromFloat(1200.50), FromFloat(2)))
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

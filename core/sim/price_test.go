package sim

import "testing"

func TestPriceFromAcceptance(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{name: "zero", in: 0, want: 0},
		{name: "one", in: 1, want: 1},
		{name: "fraction", in: 0.7, want: 0.7},
		{name: "clamps low", in: -0.2, want: 0},
		{name: "clamps high", in: 1.2, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertApprox(t, PriceFromAcceptance(tt.in), tt.want)
		})
	}
}

func TestRiskFromAcceptance(t *testing.T) {
	assertApprox(t, RiskFromAcceptance(0), 1)
	assertApprox(t, RiskFromAcceptance(1), 0)
	assertApprox(t, RiskFromAcceptance(0.7), 0.3)
	assertApprox(t, RiskFromAcceptance(-0.1), 1)
	assertApprox(t, RiskFromAcceptance(1.1), 0)
}

func TestPriceTableSetGet(t *testing.T) {
	pt := NewPriceTable()
	pt.Set("A", "asset-1", 0.7)

	price, ok := pt.Get("A", "asset-1")
	if !ok {
		t.Fatal("expected price")
	}
	assertApprox(t, price, 0.7)

	if _, ok := pt.Get("A", "missing"); ok {
		t.Fatal("expected missing asset")
	}
	if _, ok := pt.Get("missing", "asset-1"); ok {
		t.Fatal("expected missing node")
	}
}

func TestPriceTableSetCopiesNodePriceMap(t *testing.T) {
	pt := NewPriceTable()
	pt.Set("A", "asset-1", 0.4)
	originalAssets := pt.Prices["A"]

	pt.Set("A", "asset-2", 0.8)
	originalAssets["asset-1"] = 0.1

	price, ok := pt.Get("A", "asset-1")
	if !ok {
		t.Fatal("expected price")
	}
	assertApprox(t, price, 0.4)
}

func TestPriceTableFromAcceptance(t *testing.T) {
	state := AcceptanceState{Scores: map[string]float64{"A": 0.2, "B": 0.9}}
	pt := PriceTableFromAcceptance("asset-1", state)

	price, ok := pt.Get("A", "asset-1")
	if !ok {
		t.Fatal("expected A price")
	}
	assertApprox(t, price, 0.2)

	price, ok = pt.Get("B", "asset-1")
	if !ok {
		t.Fatal("expected B price")
	}
	assertApprox(t, price, 0.9)
}

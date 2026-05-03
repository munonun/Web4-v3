package sim

import "testing"

func TestAssetFlowMetricsInitializedAssetsExist(t *testing.T) {
	flows := NewAssetFlowMetrics([]string{"WEB4", "SKUG", "WEB4"})

	if _, ok := flows.TradeVolume["SKUG"]; !ok {
		t.Fatal("expected SKUG trade flow key")
	}
	if _, ok := flows.PaymentVolume["WEB4"]; !ok {
		t.Fatal("expected WEB4 payment flow key")
	}
}

func TestAssetFlowMetricsAddMethods(t *testing.T) {
	flows := NewAssetFlowMetrics([]string{"SKUG"})
	flows.AddTrade("SKUG", 2)
	flows.AddPayment("SKUG", 3)
	flows.AddConsumption("SKUG", 4)
	flows.AddDemandFulfilled("SKUG", 5)
	flows.AddTrade("SKUG", -10)

	assertApprox(t, flows.TradeVolume["SKUG"], 2)
	assertApprox(t, flows.PaymentVolume["SKUG"], 3)
	assertApprox(t, flows.ConsumptionVolume["SKUG"], 4)
	assertApprox(t, flows.DemandFulfilled["SKUG"], 5)
}

func TestAssetFlowMetricsCopyDoesNotMutateOriginal(t *testing.T) {
	flows := NewAssetFlowMetrics([]string{"SKUG"})
	flows.AddTrade("SKUG", 2)

	copied := flows.Copy()
	copied.AddTrade("SKUG", 3)

	assertApprox(t, flows.TradeVolume["SKUG"], 2)
	assertApprox(t, copied.TradeVolume["SKUG"], 5)
}

func TestFlowShare(t *testing.T) {
	shares := FlowShare(map[string]float64{"SKUG": 3, "WEB4": 1})

	assertApprox(t, shares["SKUG"], 0.75)
	assertApprox(t, shares["WEB4"], 0.25)
}

func TestFlowShareZeroTotal(t *testing.T) {
	shares := FlowShare(map[string]float64{"SKUG": 0, "WEB4": 0})

	assertApprox(t, shares["SKUG"], 0)
	assertApprox(t, shares["WEB4"], 0)
	if DominantByFlow(map[string]float64{"SKUG": 0, "WEB4": 0}) != "" {
		t.Fatal("expected empty dominant asset")
	}
}

func TestDominantByFlowTieBreaksByAssetID(t *testing.T) {
	dominant := DominantByFlow(map[string]float64{"WEB4": 1, "SKUG": 1})

	if dominant != "SKUG" {
		t.Fatalf("dominant %q, want SKUG", dominant)
	}
}

func TestFlowConcentration(t *testing.T) {
	assertApprox(t, FlowConcentration(map[string]float64{"SKUG": 3, "WEB4": 1}), 0.75)
}

package sim

import "testing"

func TestMultiMarketBasicProducesCrossAssetTrades(t *testing.T) {
	cfg := DefaultMultiMarketConfig()
	cfg.Scenario = "multi-basic"
	cfg.Steps = 100

	result, err := RunMultiMarketSimulation(cfg)
	if err != nil {
		t.Fatalf("run multi market: %v", err)
	}

	if result.Summary.TotalCrossAssetTrades == 0 {
		t.Fatalf("expected cross-asset trades, got %+v", result.Summary)
	}
	if result.Summary.ExampleExchangeRate == 0 {
		t.Fatalf("expected exchange rate, got %+v", result.Summary)
	}
	if result.Summary.TotalTradeFlow == 0 {
		t.Fatalf("expected trade flow, got %+v", result.Summary)
	}
}

func TestMultiMarketFragmentedPreservesLocalDominance(t *testing.T) {
	cfg := DefaultMultiMarketConfig()
	cfg.Scenario = "multi-fragmented"
	cfg.Topology = "clustered"
	cfg.Steps = 50

	result, err := RunMultiMarketSimulation(cfg)
	if err != nil {
		t.Fatalf("run multi market: %v", err)
	}

	clusterOneSKUG := (result.Final.Get("A", "SKUG") + result.Final.Get("B", "SKUG")) / 2
	clusterOneWEB4 := (result.Final.Get("A", "WEB4") + result.Final.Get("B", "WEB4")) / 2
	clusterTwoSKUG := (result.Final.Get("C", "SKUG") + result.Final.Get("D", "SKUG")) / 2
	clusterTwoWEB4 := (result.Final.Get("C", "WEB4") + result.Final.Get("D", "WEB4")) / 2

	if clusterOneSKUG <= clusterOneWEB4 || clusterTwoWEB4 <= clusterTwoSKUG {
		t.Fatalf("expected cluster asset zones, got c1 SKUG %.3f WEB4 %.3f c2 SKUG %.3f WEB4 %.3f", clusterOneSKUG, clusterOneWEB4, clusterTwoSKUG, clusterTwoWEB4)
	}
}

func TestMultiMarketDeterministicWithSameSeed(t *testing.T) {
	cfg := DefaultMultiMarketConfig()
	cfg.Scenario = "multi-compete"
	cfg.Steps = 25
	cfg.Seed = 42

	first, err := RunMultiMarketSimulation(cfg)
	if err != nil {
		t.Fatalf("run first: %v", err)
	}
	second, err := RunMultiMarketSimulation(cfg)
	if err != nil {
		t.Fatalf("run second: %v", err)
	}

	if len(first.Metrics) != len(second.Metrics) {
		t.Fatalf("metrics length %d, want %d", len(first.Metrics), len(second.Metrics))
	}
	for i := range first.Metrics {
		if first.Metrics[i].ExecutedTrades != second.Metrics[i].ExecutedTrades {
			t.Fatalf("step %d trades differ", i)
		}
		assertApprox(t, first.Metrics[i].TotalVolume, second.Metrics[i].TotalVolume)
		if first.Metrics[i].DominantAsset != second.Metrics[i].DominantAsset {
			t.Fatalf("step %d dominant asset differs", i)
		}
	}
}

func TestMultiMarketSameAssetTradeIncrementsFlow(t *testing.T) {
	cfg := DefaultMultiMarketConfig()
	cfg.Scenario = "multi-basic"
	cfg.Steps = 1
	cfg.EnableSubstitution = false

	result, err := RunMultiMarketSimulation(cfg)
	if err != nil {
		t.Fatalf("run multi market: %v", err)
	}
	step := result.Metrics[1]
	if totalFlowVolume(step.Flows.DemandFulfilled) == 0 {
		t.Fatalf("expected demand fulfilled flow, got %+v", step.Flows)
	}
	if totalFlowVolume(step.Flows.TradeVolume) == 0 {
		t.Fatalf("expected trade flow, got %+v", step.Flows)
	}
}

func TestMultiMarketCrossAssetTradeIncrementsPaymentFlow(t *testing.T) {
	cfg := DefaultMultiMarketConfig()
	cfg.Scenario = "multi-basic"
	cfg.Steps = 5
	cfg.EnableSubstitution = true

	result, err := RunMultiMarketSimulation(cfg)
	if err != nil {
		t.Fatalf("run multi market: %v", err)
	}
	if result.Summary.TotalCrossAssetTrades == 0 {
		t.Fatalf("expected cross-asset trades, got %+v", result.Summary)
	}
	if result.Summary.TotalPaymentFlow == 0 {
		t.Fatalf("expected payment flow, got %+v", result.Summary)
	}
}

func TestMultiMarketConsumptionIncrementsFlow(t *testing.T) {
	cfg := DefaultMultiMarketConfig()
	cfg.Scenario = "multi-basic"
	cfg.Steps = 1

	result, err := RunMultiMarketSimulation(cfg)
	if err != nil {
		t.Fatalf("run multi market: %v", err)
	}
	if result.Summary.TotalConsumptionFlow == 0 {
		t.Fatalf("expected consumption flow, got %+v", result.Summary)
	}
}

func TestMultiMarketDominantHoldingsCanDifferFromFlow(t *testing.T) {
	flows := NewAssetFlowMetrics([]string{"SKUG", "WEB4"})
	flows.AddTrade("WEB4", 10)
	metrics := []MultiMarketMetrics{{
		AssetMeans:    map[string]float64{"SKUG": 0.5, "WEB4": 0.5},
		AssetSpreads:  map[string]float64{"SKUG": 0.1, "WEB4": 0.1},
		AssetShares:   map[string]float64{"SKUG": 0.8, "WEB4": 0.2},
		DominantAsset: "SKUG",
	}}

	summary := MultiMarketSummaryFromMetrics(metrics, flows, 1, 0, 0, 10, NewAssetUniverse([]string{"SKUG", "WEB4"}))

	if summary.DominantAssetByHoldings != "SKUG" || summary.DominantAssetByFlow != "WEB4" {
		t.Fatalf("expected holdings SKUG and flow WEB4, got %+v", summary)
	}
}

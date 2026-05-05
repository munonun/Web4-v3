package sim

import (
	"testing"

	modelcore "web4-v3/core/model"
	pricecore "web4-v3/core/price"
)

func TestPriceTableFromMultiPipelineNoTradesUsesSeed(t *testing.T) {
	universe := NewAssetUniverse([]string{"SKUG", "WEB4"})
	state := newPipelineTestState()
	ps := NewMultiPriceState(universe, state)
	cfg := testPipelineConfig()

	prices := PriceTableFromMultiPipeline(ps, cfg, 0)
	skug := prices[UnitIDForAsset("SKUG")]
	web4 := prices[UnitIDForAsset("WEB4")]
	if skug <= 0 || web4 <= 0 {
		t.Fatalf("expected seed prices, got SKUG=%f WEB4=%f", skug, web4)
	}
	if skug == web4 {
		t.Fatalf("expected independent seed prices, got both %f", skug)
	}
}

func TestPriceTableFromMultiPipelineVolumeBlending(t *testing.T) {
	universe := NewAssetUniverse([]string{"SKUG", "WEB4"})
	state := newPipelineTestState()
	cfg := testPipelineConfig()

	low := NewMultiPriceState(universe, state)
	recordSameAssetObservation(&low, "SKUG", 10, 1, 1, 1)
	lowPrice := PriceTableFromMultiPipeline(low, cfg, 1)[UnitIDForAsset("SKUG")]

	high := NewMultiPriceState(universe, state)
	recordSameAssetObservation(&high, "SKUG", 10, 10, 1, 1)
	highPrice := PriceTableFromMultiPipeline(high, cfg, 1)[UnitIDForAsset("SKUG")]

	if highPrice <= lowPrice {
		t.Fatalf("expected high-volume market price to dominate: low=%f high=%f", lowPrice, highPrice)
	}
	if highPrice != 10 {
		t.Fatalf("high-volume price = %f, want 10", highPrice)
	}
}

func TestPriceTableFromMultiPipelineDecay(t *testing.T) {
	universe := NewAssetUniverse([]string{"SKUG", "WEB4"})
	state := newPipelineTestState()
	cfg := testPipelineConfig()
	cfg.DecayK = 0.1

	ps := NewMultiPriceState(universe, state)
	recordSameAssetObservation(&ps, "SKUG", 10, 10, 1, 1)
	active := PriceTableFromMultiPipeline(ps, cfg, 1)[UnitIDForAsset("SKUG")]
	inactive := PriceTableFromMultiPipeline(ps, cfg, 11)[UnitIDForAsset("SKUG")]

	if active != 10 {
		t.Fatalf("active price = %f, want 10", active)
	}
	if inactive >= active {
		t.Fatalf("inactive price %f should decay below active %f", inactive, active)
	}
}

func TestRecordCrossAssetObservationsUpdatesBothPipelines(t *testing.T) {
	universe := NewAssetUniverse([]string{"SKUG", "WEB4"})
	ps := NewMultiPriceState(universe, newPipelineTestState())
	quote := CrossAssetTradeQuote{
		SellAsset:  "SKUG",
		BuyAsset:   "WEB4",
		SellQty:    2,
		BuyQty:     4,
		Executable: true,
	}

	recordCrossAssetObservations(&ps, quote, 1, 1, 7)

	skugUnit := UnitIDForAsset("SKUG")
	web4Unit := UnitIDForAsset("WEB4")
	if len(ps.Trades[skugUnit]) != 1 || len(ps.Trades[web4Unit]) != 1 {
		t.Fatalf("expected one observation per asset: %+v", ps.Trades)
	}
	if ps.Trades[skugUnit][0].Price != 2 {
		t.Fatalf("SKUG price = %f, want 2", ps.Trades[skugUnit][0].Price)
	}
	if ps.Trades[web4Unit][0].Price != 0.5 {
		t.Fatalf("WEB4 price = %f, want 0.5", ps.Trades[web4Unit][0].Price)
	}
	if ps.SettledVolume[skugUnit] != modelcore.FromFloat(2) || ps.SettledVolume[web4Unit] != modelcore.FromFloat(4) {
		t.Fatalf("bad settled volume: %+v", ps.SettledVolume)
	}
}

func TestMultiMarketPipelineRunsAndReportsExchangeRate(t *testing.T) {
	cfg := DefaultMultiMarketConfig()
	cfg.PriceModel = PriceModelPipeline
	cfg.Scenario = "multi-basic"
	cfg.Steps = 50

	result, err := RunMultiMarketSimulation(cfg)
	if err != nil {
		t.Fatalf("run multi pipeline: %v", err)
	}
	if result.Summary.TotalTrades == 0 {
		t.Fatalf("expected trades with pipeline pricing: %+v", result.Summary)
	}
	if result.Summary.ExampleExchangeRate <= 0 {
		t.Fatalf("expected exchange rate: %+v", result.Summary)
	}
}

func TestMultiMarketPipelineBehaviorScenariosRun(t *testing.T) {
	for _, scenario := range []string{"multi-flight", "multi-coexist", "multi-compete"} {
		cfg := DefaultMultiMarketConfig()
		cfg.PriceModel = PriceModelPipeline
		cfg.Scenario = scenario
		cfg.EnableSubstitution = true
		cfg.Steps = 50
		if scenario == "multi-coexist" {
			cfg.Topology = "clustered"
			cfg.UtilityMode = "clustered"
		}

		result, err := RunMultiMarketSimulation(cfg)
		if err != nil {
			t.Fatalf("%s pipeline run: %v", scenario, err)
		}
		if result.Summary.DominantAsset == "" {
			t.Fatalf("%s missing dominant asset: %+v", scenario, result.Summary)
		}
	}
}

func newPipelineTestState() MultiAcceptanceState {
	state := NewMultiAcceptanceState()
	state.Set("A", "SKUG", 0.8)
	state.Set("B", "SKUG", 0.7)
	state.Set("A", "WEB4", 0.3)
	state.Set("B", "WEB4", 0.4)
	return state
}

func testPipelineConfig() pricecore.PriceConfig {
	return pricecore.PriceConfig{
		BasePrice:       1,
		Weights:         pricecore.FeatureWeights{Cost: 1},
		VolumeThreshold: modelcore.FromFloat(10),
	}
}

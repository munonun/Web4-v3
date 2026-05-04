package sim

import (
	"testing"
)

func TestMarketSimulationDeterministicWithSameSeed(t *testing.T) {
	cfg := DefaultMarketConfig()
	cfg.Scenario = "random"
	cfg.Steps = 25
	cfg.Seed = 42

	first, err := RunMarketSimulation(cfg)
	if err != nil {
		t.Fatalf("run first: %v", err)
	}
	second, err := RunMarketSimulation(cfg)
	if err != nil {
		t.Fatalf("run second: %v", err)
	}

	if len(first.Metrics) != len(second.Metrics) {
		t.Fatalf("metrics length %d, want %d", len(first.Metrics), len(second.Metrics))
	}
	for i := range first.Metrics {
		assertApprox(t, first.Metrics[i].MeanPrice, second.Metrics[i].MeanPrice)
		assertApprox(t, first.Metrics[i].PriceSpread, second.Metrics[i].PriceSpread)
		if first.Metrics[i].ExecutedTrades != second.Metrics[i].ExecutedTrades {
			t.Fatalf("step %d trades %d, want %d", i, first.Metrics[i].ExecutedTrades, second.Metrics[i].ExecutedTrades)
		}
	}
	for nodeID, score := range first.Final.Scores {
		assertApprox(t, score, second.Final.Scores[nodeID])
	}
}

func TestMarketSplitFullReducesPriceSpread(t *testing.T) {
	cfg := DefaultMarketConfig()
	cfg.Scenario = "split"
	cfg.Topology = "full"
	cfg.Steps = 50
	cfg.Alpha = 0.2

	result, err := RunMarketSimulation(cfg)
	if err != nil {
		t.Fatalf("run market: %v", err)
	}

	if !result.Summary.PriceSpreadDecreased {
		t.Fatalf("expected spread to decrease: %+v", result.Summary)
	}
	if result.Summary.FinalPriceSpread >= result.Summary.InitialPriceSpread {
		t.Fatalf("final spread %f, initial %f", result.Summary.FinalPriceSpread, result.Summary.InitialPriceSpread)
	}
}

func TestMarketDemandBasicProducesDemandTrades(t *testing.T) {
	cfg := DefaultMarketConfig()
	cfg.Scenario = "demand-basic"
	cfg.Topology = "full"
	cfg.Steps = 20
	cfg.EnableDemand = true
	cfg.MaxQty = 1

	result, err := RunMarketSimulation(cfg)
	if err != nil {
		t.Fatalf("run market: %v", err)
	}

	if result.Summary.DemandTrades == 0 {
		t.Fatalf("expected demand trades, got %+v", result.Summary)
	}
	if result.Summary.TotalVolume == 0 {
		t.Fatalf("expected non-zero volume, got %+v", result.Summary)
	}
}

func TestMarketCycleBasicProducesRepeatedVolume(t *testing.T) {
	cfg := DefaultMarketConfig()
	cfg.Scenario = "cycle-basic"
	cfg.Topology = "full"
	cfg.Steps = 50
	cfg.EnableDemand = true
	cfg.EnableCycle = true
	cfg.ConsumptionRate = 0.1
	cfg.ProductionRate = 0.3

	result, err := RunMarketSimulation(cfg)
	if err != nil {
		t.Fatalf("run market: %v", err)
	}

	if result.Summary.TotalVolume <= 8 {
		t.Fatalf("expected repeated volume beyond one-shot demand, got %+v", result.Summary)
	}
	if result.Summary.TotalProduced == 0 || result.Summary.TotalConsumed == 0 {
		t.Fatalf("expected production and consumption, got %+v", result.Summary)
	}
	if result.Summary.FinalUnmetDemand > 1 {
		t.Fatalf("expected bounded unmet demand, got %+v", result.Summary)
	}
}

func TestMarketCycleVolumeIncreasesWithMoreSteps(t *testing.T) {
	shortCfg := DefaultMarketConfig()
	shortCfg.Scenario = "cycle-basic"
	shortCfg.Topology = "full"
	shortCfg.Steps = 20
	shortCfg.EnableDemand = true
	shortCfg.EnableCycle = true
	shortCfg.ConsumptionRate = 0.1
	shortCfg.ProductionRate = 0.3

	longCfg := shortCfg
	longCfg.Steps = 60

	shortResult, err := RunMarketSimulation(shortCfg)
	if err != nil {
		t.Fatalf("run short market: %v", err)
	}
	longResult, err := RunMarketSimulation(longCfg)
	if err != nil {
		t.Fatalf("run long market: %v", err)
	}

	if longResult.Summary.TotalVolume <= shortResult.Summary.TotalVolume {
		t.Fatalf("expected volume to grow with steps, short %f long %f", shortResult.Summary.TotalVolume, longResult.Summary.TotalVolume)
	}
}

func TestMarketCycleConsumptionRecreatesUnmetDemand(t *testing.T) {
	cfg := DefaultMarketConfig()
	cfg.Scenario = "cycle-basic"
	cfg.Topology = "full"
	cfg.Steps = 12
	cfg.EnableDemand = true
	cfg.EnableCycle = true
	cfg.ConsumptionRate = 0.1
	cfg.ProductionRate = 0.3

	result, err := RunMarketSimulation(cfg)
	if err != nil {
		t.Fatalf("run market: %v", err)
	}

	recreated := false
	for _, metric := range result.Metrics[1:] {
		if metric.ConsumedVolume > 0 && metric.UnmetDemand > 0 {
			recreated = true
			break
		}
	}
	if !recreated {
		t.Fatalf("expected consumption to recreate unmet demand")
	}
}

func TestMarketCycleProductionPreventsPermanentCollapse(t *testing.T) {
	cfg := DefaultMarketConfig()
	cfg.Scenario = "cycle-basic"
	cfg.Topology = "full"
	cfg.Steps = 100
	cfg.EnableDemand = true
	cfg.EnableCycle = true
	cfg.ConsumptionRate = 0.1
	cfg.ProductionRate = 0.3

	result, err := RunMarketSimulation(cfg)
	if err != nil {
		t.Fatalf("run market: %v", err)
	}

	if result.Summary.Collapsed {
		t.Fatalf("expected adequate production to avoid collapse, got %+v", result.Summary)
	}
}

func TestMarketDemandOnlyWorksWhenCycleDisabled(t *testing.T) {
	cfg := DefaultMarketConfig()
	cfg.Scenario = "demand-basic"
	cfg.Topology = "full"
	cfg.Steps = 20
	cfg.EnableDemand = true
	cfg.EnableCycle = false

	result, err := RunMarketSimulation(cfg)
	if err != nil {
		t.Fatalf("run market: %v", err)
	}

	if result.Summary.DemandTrades == 0 {
		t.Fatalf("expected demand-only trades, got %+v", result.Summary)
	}
	if result.Summary.TotalProduced != 0 || result.Summary.TotalConsumed != 0 {
		t.Fatalf("expected no cycle flows, got %+v", result.Summary)
	}
}

func TestMarketDemandBasicReducesUnmetDemand(t *testing.T) {
	cfg := DefaultMarketConfig()
	cfg.Scenario = "demand-basic"
	cfg.Topology = "full"
	cfg.Steps = 20
	cfg.EnableDemand = true

	result, err := RunMarketSimulation(cfg)
	if err != nil {
		t.Fatalf("run market: %v", err)
	}

	initial := result.Metrics[0].UnmetDemand
	final := result.Metrics[len(result.Metrics)-1].UnmetDemand
	if final >= initial {
		t.Fatalf("expected unmet demand to decrease, initial %f final %f", initial, final)
	}
}

func TestMarketDemandDisabledKeepsArbitrageOnlyScenariosWorking(t *testing.T) {
	cfg := DefaultMarketConfig()
	cfg.Scenario = "split"
	cfg.Topology = "full"
	cfg.Steps = 10
	cfg.EnableDemand = false

	result, err := RunMarketSimulation(cfg)
	if err != nil {
		t.Fatalf("run market: %v", err)
	}

	if result.Summary.DemandTrades != 0 || result.Summary.TotalVolume != 0 {
		t.Fatalf("expected no demand activity, got %+v", result.Summary)
	}
	if result.Summary.TotalTrades == 0 {
		t.Fatalf("expected existing arbitrage trades to still run, got %+v", result.Summary)
	}
}

func TestMarketCollapseScenarioCollapsesOrHasLowLiquidity(t *testing.T) {
	cfg := DefaultMarketConfig()
	cfg.Scenario = "collapse"
	cfg.Topology = "full"
	cfg.Steps = 20

	result, err := RunMarketSimulation(cfg)
	if err != nil {
		t.Fatalf("run market: %v", err)
	}

	if !result.Summary.Collapsed && result.Summary.TotalTrades > 1 {
		t.Fatalf("expected collapse or very low liquidity, got %+v", result.Summary)
	}
}

func TestMarketClusteredTopologyCanRemainFragmented(t *testing.T) {
	cfg := DefaultMarketConfig()
	cfg.Scenario = "fragmented"
	cfg.Topology = "clustered"
	cfg.Steps = 100
	cfg.Alpha = 0.2

	result, err := RunMarketSimulation(cfg)
	if err != nil {
		t.Fatalf("run market: %v", err)
	}

	if !result.Summary.Fragmented {
		t.Fatalf("expected fragmented market, got %+v", result.Summary)
	}
}

func TestValidateMarketConfigRejectsInvalidValues(t *testing.T) {
	cfg := DefaultMarketConfig()
	cfg.Scenario = "missing"
	if err := ValidateMarketConfig(cfg); err == nil {
		t.Fatal("expected invalid scenario error")
	}

	cfg = DefaultMarketConfig()
	cfg.Topology = "missing"
	if err := ValidateMarketConfig(cfg); err == nil {
		t.Fatal("expected invalid topology error")
	}

	cfg = DefaultMarketConfig()
	cfg.Steps = -1
	if err := ValidateMarketConfig(cfg); err == nil {
		t.Fatal("expected invalid steps error")
	}

	cfg = DefaultMarketConfig()
	cfg.Alpha = 1.1
	if err := ValidateMarketConfig(cfg); err == nil {
		t.Fatal("expected invalid alpha error")
	}

	cfg = DefaultMarketConfig()
	cfg.PriceModel = "missing"
	if err := ValidateMarketConfig(cfg); err == nil {
		t.Fatal("expected invalid price model error")
	}
}

func TestStepMarketTopologyDoesNotMutateInput(t *testing.T) {
	state := AcceptanceState{Scores: map[string]float64{"A": 1, "B": 0}}
	topology := FullMeshTopology([]string{"A", "B"})

	next := StepMarketTopology(state, topology, 0.5)

	assertApprox(t, state.Scores["A"], 1)
	assertApprox(t, state.Scores["B"], 0)
	next.Scores["A"] = 0
	assertApprox(t, state.Scores["A"], 1)
}

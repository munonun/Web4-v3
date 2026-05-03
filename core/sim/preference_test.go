package sim

import "testing"

func TestPortfolioPreferenceEffectiveValue(t *testing.T) {
	pt := NewPriceTable()
	pt.Set("A", "SKUG", 0.5)
	pref := NewPortfolioPreference()
	pref.SetWeight("A", "SKUG", 1.6)

	assertApprox(t, pref.Weight("A", "SKUG"), 1.6)
	assertApprox(t, EffectiveValue(pt, pref, "A", "SKUG"), 0.8)
}

func TestPortfolioPreferencePreferredAsset(t *testing.T) {
	pt := NewPriceTable()
	pt.Set("A", "SKUG", 0.6)
	pt.Set("A", "WEB4", 0.5)
	pref := NewPortfolioPreference()
	pref.SetWeight("A", "SKUG", 0.8)
	pref.SetWeight("A", "WEB4", 1.4)

	asset := PreferredAsset(pt, pref, "A", NewAssetUniverse([]string{"SKUG", "WEB4"}))

	if asset != "WEB4" {
		t.Fatalf("preferred asset %q, want WEB4", asset)
	}
}

func TestValueDemandNeed(t *testing.T) {
	pt := NewPriceTable()
	pt.Set("A", "SKUG", 0.5)
	pt.Set("A", "WEB4", 1)
	inv := NewInventoryState()
	inv.Set("A", "SKUG", 2)
	inv.Set("A", "WEB4", 1)
	pref := NewPortfolioPreference()
	demand := NewValueDemand()
	demand.SetTarget("A", 3)

	need := demand.Need("A", inv, pt, pref, NewAssetUniverse([]string{"SKUG", "WEB4"}))

	assertApprox(t, need, 1)
}

func TestMultiMarketSubstitutionProducesSwitches(t *testing.T) {
	cfg := DefaultMultiMarketConfig()
	cfg.Scenario = "multi-flight"
	cfg.Steps = 100
	cfg.EnableSubstitution = true

	result, err := RunMultiMarketSimulation(cfg)
	if err != nil {
		t.Fatalf("run multi market: %v", err)
	}
	if result.Summary.TotalSwitchCount == 0 {
		t.Fatalf("expected substitution switches, got %+v", result.Summary)
	}
}

func TestMultiMarketCoexistClusteredUtility(t *testing.T) {
	cfg := DefaultMultiMarketConfig()
	cfg.Scenario = "multi-coexist"
	cfg.Topology = "clustered"
	cfg.UtilityMode = "clustered"
	cfg.EnableSubstitution = true
	cfg.Steps = 50

	result, err := RunMultiMarketSimulation(cfg)
	if err != nil {
		t.Fatalf("run multi market: %v", err)
	}
	skugShare := result.Summary.FinalAssetShares["SKUG"]
	web4Share := result.Summary.FinalAssetShares["WEB4"]
	if skugShare <= 0 || web4Share <= 0 {
		t.Fatalf("expected coexistence shares, got %+v", result.Summary.FinalAssetShares)
	}
}

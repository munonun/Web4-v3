package sim

import (
	"fmt"
	"math"
	"math/rand"
)

type MultiMarketConfig struct {
	Scenario           string
	Topology           string
	Universe           AssetUniverse
	Steps              int
	Alpha              float64
	Spread             float64
	TradesPerStep      int
	Seed               int64
	EnableDemand       bool
	EnableCycle        bool
	EnableSubstitution bool
	UtilityMode        string
	MaxQty             float64
}

type MultiMarketMetrics struct {
	Step                int
	AssetMeans          map[string]float64
	AssetSpreads        map[string]float64
	ExchangeRates       map[string]float64
	AssetShares         map[string]float64
	FlowShares          map[string]float64
	Flows               AssetFlowMetrics
	ExecutedTrades      int
	CrossAssetTrades    int
	SwitchCount         int
	TotalVolume         float64
	DominantAsset       string
	DominantAssetByFlow string
	FlowConcentration   float64
}

type MultiMarketSummary struct {
	FinalAssetMeans         map[string]float64 `json:"final_asset_means"`
	FinalAssetSpreads       map[string]float64 `json:"final_asset_spreads"`
	FinalAssetShares        map[string]float64 `json:"final_asset_shares"`
	DominantAsset           string             `json:"dominant_asset"`
	DominantAssetByHoldings string             `json:"dominant_asset_by_holdings"`
	DominantAssetByFlow     string             `json:"dominant_asset_by_flow"`
	FlowConcentration       float64            `json:"flow_concentration"`
	FlowShares              map[string]float64 `json:"flow_shares"`
	TradeFlow               map[string]float64 `json:"trade_flow"`
	PaymentFlow             map[string]float64 `json:"payment_flow"`
	ConsumptionFlow         map[string]float64 `json:"consumption_flow"`
	DemandFulfilled         map[string]float64 `json:"demand_fulfilled"`
	TotalTradeFlow          float64            `json:"total_trade_flow"`
	TotalPaymentFlow        float64            `json:"total_payment_flow"`
	TotalConsumptionFlow    float64            `json:"total_consumption_flow"`
	TotalDemandFulfilled    float64            `json:"total_demand_fulfilled"`
	TotalTrades             int                `json:"total_trades"`
	TotalCrossAssetTrades   int                `json:"total_cross_asset_trades"`
	TotalSwitchCount        int                `json:"total_switch_count"`
	TotalVolume             float64            `json:"total_volume"`
	ExampleExchangeRate     float64            `json:"example_exchange_rate"`
}

type MultiMarketResult struct {
	Config  MultiMarketConfig
	Final   MultiAcceptanceState
	Metrics []MultiMarketMetrics
	Summary MultiMarketSummary
}

func DefaultMultiMarketConfig() MultiMarketConfig {
	return MultiMarketConfig{
		Scenario:      "multi-basic",
		Topology:      "full",
		Universe:      NewAssetUniverse([]string{"SKUG", "WEB4"}),
		Steps:         1000,
		Alpha:         0.2,
		Spread:        0.01,
		TradesPerStep: 0,
		Seed:          1,
		EnableDemand:  true,
		EnableCycle:   true,
		UtilityMode:   "fixed",
		MaxQty:        1,
	}
}

func RunMultiMarketSimulation(cfg MultiMarketConfig) (MultiMarketResult, error) {
	if err := ValidateMultiMarketConfig(cfg); err != nil {
		return MultiMarketResult{}, err
	}

	state, inventory, demand, consumption, production, preference, valueDemand, err := MultiMarketScenario(cfg)
	if err != nil {
		return MultiMarketResult{}, err
	}
	nodeIDs := multiNodeIDs(state)
	if cfg.TradesPerStep == 0 {
		cfg.TradesPerStep = len(nodeIDs) * len(cfg.Universe.AssetIDs)
	}
	topology, _, err := MarketTopology(cfg.Topology, cfg.Scenario, nodeIDs)
	if err != nil {
		return MultiMarketResult{}, err
	}

	current := state.Copy()
	currentInventory := inventory.Copy()
	metrics := make([]MultiMarketMetrics, 0, cfg.Steps+1)
	totalTrades := 0
	totalCrossTrades := 0
	totalSwitches := 0
	totalVolume := 0.0
	totalFlows := NewAssetFlowMetrics(cfg.Universe.IDs())
	metrics = append(metrics, MultiMarketStepMetrics(0, current, currentInventory, cfg.Universe, NewAssetFlowMetrics(cfg.Universe.IDs()), 0, 0, 0, 0))

	for step := 1; step <= cfg.Steps; step++ {
		stepFlows := NewAssetFlowMetrics(cfg.Universe.IDs())
		if cfg.EnableCycle {
			var consumptionFlows AssetFlowMetrics
			currentInventory, consumptionFlows = ApplyConsumptionWithFlow(consumption, currentInventory, cfg.Universe)
			addAssetFlows(stepFlows, consumptionFlows)
			currentInventory = production.Apply(currentInventory)
			current = StepMultiDemandPricePressure(current, currentInventory, demand, cfg.Universe, cfg.Alpha)
		}

		pt := PriceTableFromMultiAcceptance(current)
		executed := 0
		crossTrades := 0
		stepVolume := 0.0
		if cfg.EnableDemand {
			if cfg.EnableSubstitution {
				current = StepSubstitutionPressure(current, currentInventory, preference, cfg.Universe, cfg.Alpha)
				pt = PriceTableFromMultiAcceptance(current)
				_ = valueDemand
			}
			for _, assetID := range cfg.Universe.IDs() {
				for _, sellerID := range nodeIDs {
					for _, buyerID := range nodeIDs {
						if executed >= cfg.TradesPerStep {
							break
						}
						if sellerID == buyerID || !marketPairConnected(topology, sellerID, buyerID) {
							continue
						}
						quote := QuoteDemandTrade(pt, currentInventory, demand, sellerID, buyerID, assetID, cfg.MaxQty, cfg.Spread)
						if !quote.Executable {
							continue
						}
						assetState := current.AssetState(assetID)
						currentInventory, assetState = ApplyDemandTrade(currentInventory, assetState, quote, cfg.Alpha)
						current = current.SetAssetState(assetID, assetState)
						executed++
						stepVolume += quote.Quantity
						stepFlows.AddTrade(assetID, quote.Quantity)
						stepFlows.AddDemandFulfilled(assetID, quote.Quantity)
					}
				}
			}
			for _, sellAsset := range cfg.Universe.IDs() {
				for _, buyAsset := range cfg.Universe.IDs() {
					if sellAsset == buyAsset {
						continue
					}
					for _, sellerID := range nodeIDs {
						for _, buyerID := range nodeIDs {
							if executed >= cfg.TradesPerStep {
								break
							}
							if sellerID == buyerID || !marketPairConnected(topology, sellerID, buyerID) {
								continue
							}
							sellerOK := demand.Surplus(sellerID, sellAsset, currentInventory) > 0 && demand.Need(sellerID, buyAsset, currentInventory) > 0
							buyerOK := demand.Surplus(buyerID, buyAsset, currentInventory) > 0 && demand.Need(buyerID, sellAsset, currentInventory) > 0
							if cfg.EnableSubstitution {
								sellerOK = sellerOK || substitutionSellerCanSwitch(pt, currentInventory, preference, cfg.Universe, sellerID, sellAsset, buyAsset)
								buyerOK = buyerOK || substitutionSellerCanSwitch(pt, currentInventory, preference, cfg.Universe, buyerID, buyAsset, sellAsset)
							}
							if !sellerOK || !buyerOK {
								continue
							}
							quote := QuoteCrossAssetTrade(pt, currentInventory, sellerID, buyerID, sellAsset, buyAsset, cfg.MaxQty, cfg.Spread)
							if !quote.Executable {
								if !cfg.EnableSubstitution {
									continue
								}
								quote = QuoteSubstitutionSwitch(pt, currentInventory, sellerID, buyerID, sellAsset, buyAsset, cfg.MaxQty, cfg.Spread)
								if !quote.Executable {
									continue
								}
							}
							currentInventory, current = ApplyCrossAssetTrade(currentInventory, current, quote, cfg.Alpha)
							executed++
							crossTrades++
							if cfg.EnableSubstitution {
								totalSwitches++
							}
							stepVolume += quote.SellQty
							stepFlows.AddTrade(quote.SellAsset, quote.SellQty)
							stepFlows.AddTrade(quote.BuyAsset, quote.BuyQty)
							stepFlows.AddPayment(quote.BuyAsset, quote.BuyQty)
						}
					}
				}
			}
		}

		for _, assetID := range cfg.Universe.IDs() {
			current = current.SetAssetState(assetID, StepMarketTopology(current.AssetState(assetID), topology, cfg.Alpha))
		}
		totalTrades += executed
		totalCrossTrades += crossTrades
		totalVolume += stepVolume
		addAssetFlows(totalFlows, stepFlows)
		metrics = append(metrics, MultiMarketStepMetrics(step, current, currentInventory, cfg.Universe, stepFlows, executed, crossTrades, totalSwitches, stepVolume))
	}

	summary := MultiMarketSummaryFromMetrics(metrics, totalFlows, totalTrades, totalCrossTrades, totalSwitches, totalVolume, cfg.Universe)
	return MultiMarketResult{Config: cfg, Final: current, Metrics: metrics, Summary: summary}, nil
}

func ValidateMultiMarketConfig(cfg MultiMarketConfig) error {
	if err := cfg.Universe.Validate(); err != nil {
		return err
	}
	switch cfg.Scenario {
	case "multi-basic", "multi-compete", "multi-fragmented", "multi-flight", "multi-coexist":
	default:
		return fmt.Errorf("unknown multi-market scenario %q", cfg.Scenario)
	}
	if cfg.Steps < 0 {
		return fmt.Errorf("steps must be >= 0")
	}
	if cfg.Alpha < 0 || cfg.Alpha > 1 {
		return fmt.Errorf("alpha must be in [0,1]")
	}
	if cfg.Spread < 0 {
		return fmt.Errorf("spread must be >= 0")
	}
	if cfg.TradesPerStep < 0 {
		return fmt.Errorf("trades-per-step must be >= 0")
	}
	if cfg.MaxQty <= 0 {
		return fmt.Errorf("max-qty must be > 0")
	}
	switch cfg.UtilityMode {
	case "", "fixed", "random", "clustered":
	default:
		return fmt.Errorf("unknown utility mode %q", cfg.UtilityMode)
	}
	switch cfg.Topology {
	case "full", "chain", "clustered":
	default:
		return fmt.Errorf("unknown market topology %q", cfg.Topology)
	}

	return nil
}

func MultiMarketScenario(cfg MultiMarketConfig) (MultiAcceptanceState, InventoryState, DemandState, ConsumptionState, ProductionState, PortfolioPreference, ValueDemand, error) {
	assets := cfg.Universe.IDs()
	if len(assets) < 2 {
		return MultiAcceptanceState{}, InventoryState{}, DemandState{}, ConsumptionState{}, ProductionState{}, PortfolioPreference{}, ValueDemand{}, fmt.Errorf("multi-asset simulation requires at least two assets")
	}
	state := NewMultiAcceptanceState()
	inv := NewInventoryState()
	demand := NewDemandState()
	consumption := NewConsumptionState()
	production := NewProductionState()
	preference := NewPortfolioPreference()
	valueDemand := NewValueDemand()
	nodeIDs := []string{"A", "B", "C", "D"}
	if cfg.Scenario == "multi-flight" || cfg.Scenario == "multi-compete" {
		nodeIDs = []string{"A", "B", "C", "D", "E", "F"}
	}
	if cfg.Scenario == "multi-basic" {
		a, b := assets[0], assets[1]
		setMultiNode(state, inv, demand, "A", a, 0.8, 8, 2)
		setMultiNode(state, inv, demand, "A", b, 0.45, 0, 3)
		setMultiNode(state, inv, demand, "B", a, 0.75, 0, 3)
		setMultiNode(state, inv, demand, "B", b, 0.7, 8, 2)
		setMultiNode(state, inv, demand, "C", a, 0.65, 1, 3)
		setMultiNode(state, inv, demand, "C", b, 0.75, 1, 3)
		setMultiNode(state, inv, demand, "D", a, 0.6, 0, 2)
		setMultiNode(state, inv, demand, "D", b, 0.65, 0, 2)
		production.SetRate("A", a, 0.2)
		production.SetRate("B", b, 0.2)
		for _, nodeID := range nodeIDs {
			consumption.SetRate(nodeID, a, 0.03)
			consumption.SetRate(nodeID, b, 0.03)
			valueDemand.SetTarget(nodeID, 3)
		}
		setScenarioPreferences(preference, cfg, nodeIDs, assets)
		return state, inv, demand, consumption, production, preference, valueDemand, nil
	}

	rng := rand.New(rand.NewSource(cfg.Seed))
	for _, nodeID := range nodeIDs {
		for i, assetID := range assets {
			score := 0.45 + 0.2*rng.Float64()
			holding := 2 * rng.Float64()
			target := 2 + rng.Float64()
			switch cfg.Scenario {
			case "multi-compete":
				if i == 0 {
					score += 0.25
				}
			case "multi-fragmented":
				if (nodeID == "A" || nodeID == "B") && i == 0 {
					score = 0.85
				}
				if (nodeID == "C" || nodeID == "D") && i == 1 {
					score = 0.85
				}
				if (nodeID == "A" || nodeID == "B") && i == 1 {
					score = 0.35
				}
				if (nodeID == "C" || nodeID == "D") && i == 0 {
					score = 0.35
				}
			case "multi-flight":
				if i == 0 {
					score = 0.25
				} else {
					score = 0.85
				}
			}
			setMultiNode(state, inv, demand, nodeID, assetID, score, holding, target)
			consumption.SetRate(nodeID, assetID, 0.02)
		}
		valueDemand.SetTarget(nodeID, 4)
	}
	for i, assetID := range assets {
		producer := nodeIDs[i%len(nodeIDs)]
		production.SetRate(producer, assetID, 0.15)
		inv.Set(producer, assetID, inv.Get(producer, assetID)+5)
	}

	setScenarioPreferences(preference, cfg, nodeIDs, assets)
	return state, inv, demand, consumption, production, preference, valueDemand, nil
}

func MultiMarketStepMetrics(step int, state MultiAcceptanceState, inv InventoryState, universe AssetUniverse, flows AssetFlowMetrics, executedTrades int, crossTrades int, switchCount int, volume float64) MultiMarketMetrics {
	pt := PriceTableFromMultiAcceptance(state)
	assetMeans := map[string]float64{}
	assetSpreads := map[string]float64{}
	for _, assetID := range universe.IDs() {
		assetState := state.AssetState(assetID)
		assetMeans[assetID] = AcceptanceMean(assetState)
		assetSpreads[assetID] = assetSpread(assetState)
	}
	exchangeRates := map[string]float64{}
	assetShares := assetInventoryShares(inv, universe)
	ids := universe.IDs()
	if len(ids) >= 2 {
		key := ids[0] + "/" + ids[1]
		sum := 0.0
		count := 0
		for _, nodeID := range multiNodeIDs(state) {
			quote := QuoteExchange(pt, nodeID, ids[0], ids[1])
			if quote.Valid {
				sum += quote.Rate
				count++
			}
		}
		if count > 0 {
			exchangeRates[key] = sum / float64(count)
		}
	}

	return MultiMarketMetrics{
		Step:                step,
		AssetMeans:          assetMeans,
		AssetSpreads:        assetSpreads,
		ExchangeRates:       exchangeRates,
		AssetShares:         assetShares,
		FlowShares:          FlowShare(flows.TradeVolume),
		Flows:               flows.Copy(),
		ExecutedTrades:      executedTrades,
		CrossAssetTrades:    crossTrades,
		SwitchCount:         switchCount,
		TotalVolume:         volume,
		DominantAsset:       dominantAsset(assetShares),
		DominantAssetByFlow: DominantByFlow(flows.TradeVolume),
		FlowConcentration:   FlowConcentration(flows.TradeVolume),
	}
}

func MultiMarketSummaryFromMetrics(metrics []MultiMarketMetrics, flows AssetFlowMetrics, totalTrades int, totalCrossTrades int, totalSwitches int, totalVolume float64, universe AssetUniverse) MultiMarketSummary {
	if len(metrics) == 0 {
		return MultiMarketSummary{}
	}
	final := metrics[len(metrics)-1]
	summary := MultiMarketSummary{
		FinalAssetMeans:         copyFloatMap(final.AssetMeans),
		FinalAssetSpreads:       copyFloatMap(final.AssetSpreads),
		FinalAssetShares:        copyFloatMap(final.AssetShares),
		DominantAsset:           final.DominantAsset,
		DominantAssetByHoldings: final.DominantAsset,
		DominantAssetByFlow:     DominantByFlow(flows.TradeVolume),
		FlowConcentration:       FlowConcentration(flows.TradeVolume),
		FlowShares:              FlowShare(flows.TradeVolume),
		TradeFlow:               copyFloatMap(flows.TradeVolume),
		PaymentFlow:             copyFloatMap(flows.PaymentVolume),
		ConsumptionFlow:         copyFloatMap(flows.ConsumptionVolume),
		DemandFulfilled:         copyFloatMap(flows.DemandFulfilled),
		TotalTradeFlow:          totalFlowVolume(flows.TradeVolume),
		TotalPaymentFlow:        totalFlowVolume(flows.PaymentVolume),
		TotalConsumptionFlow:    totalFlowVolume(flows.ConsumptionVolume),
		TotalDemandFulfilled:    totalFlowVolume(flows.DemandFulfilled),
		TotalTrades:             totalTrades,
		TotalCrossAssetTrades:   totalCrossTrades,
		TotalSwitchCount:        totalSwitches,
		TotalVolume:             totalVolume,
	}
	ids := universe.IDs()
	if len(ids) >= 2 {
		summary.ExampleExchangeRate = final.ExchangeRates[ids[0]+"/"+ids[1]]
	}

	return summary
}

func StepSubstitutionPressure(state MultiAcceptanceState, inv InventoryState, pref PortfolioPreference, universe AssetUniverse, alpha float64) MultiAcceptanceState {
	next := state.Copy()
	pt := PriceTableFromMultiAcceptance(state)
	for _, nodeID := range multiNodeIDs(state) {
		preferred := PreferredAsset(pt, pref, nodeID, universe)
		for _, assetID := range universe.IDs() {
			score := state.Get(nodeID, assetID)
			if assetID == preferred {
				score += alpha * 0.1 * (1 - score)
			} else if inv.Get(nodeID, assetID) > 0 {
				score -= alpha * 0.1 * score
			}
			next.Set(nodeID, assetID, score)
		}
	}
	return next
}

func substitutionSellerCanSwitch(pt PriceTable, inv InventoryState, pref PortfolioPreference, universe AssetUniverse, nodeID, sellAsset, buyAsset string) bool {
	if inv.Get(nodeID, sellAsset) <= 0 {
		return false
	}
	if PreferredAsset(pt, pref, nodeID, universe) != buyAsset {
		return false
	}
	return EffectiveValue(pt, pref, nodeID, buyAsset) > EffectiveValue(pt, pref, nodeID, sellAsset)
}

func assetInventoryShares(inv InventoryState, universe AssetUniverse) map[string]float64 {
	totals := map[string]float64{}
	total := 0.0
	for _, assets := range inv.Holdings {
		for _, assetID := range universe.IDs() {
			qty := assets[assetID]
			totals[assetID] += qty
			total += qty
		}
	}
	if total == 0 {
		return totals
	}
	for _, assetID := range universe.IDs() {
		totals[assetID] = totals[assetID] / total
	}
	return totals
}

func setScenarioPreferences(pref PortfolioPreference, cfg MultiMarketConfig, nodeIDs []string, assets []string) {
	rng := rand.New(rand.NewSource(cfg.Seed + 3000))
	for i, nodeID := range nodeIDs {
		for j, assetID := range assets {
			weight := 1.0
			switch cfg.UtilityMode {
			case "random":
				weight = 0.8 + 0.6*rng.Float64()
			case "clustered":
				if i < len(nodeIDs)/2 && j == 0 {
					weight = 1.5
				} else if i >= len(nodeIDs)/2 && j == 1 {
					weight = 1.5
				} else {
					weight = 0.75
				}
			default:
				switch cfg.Scenario {
				case "multi-compete":
					if j == 0 {
						weight = 1.6
					} else {
						weight = 0.9
					}
				case "multi-flight":
					if j == 0 {
						weight = 0.55
					} else {
						weight = 1.7
					}
				case "multi-coexist", "multi-fragmented":
					if i < len(nodeIDs)/2 && j == 0 {
						weight = 1.6
					} else if i >= len(nodeIDs)/2 && j == 1 {
						weight = 1.6
					} else {
						weight = 0.8
					}
				}
			}
			pref.SetWeight(nodeID, assetID, weight)
		}
	}
}

func StepMultiDemandPricePressure(state MultiAcceptanceState, inventory InventoryState, demand DemandState, universe AssetUniverse, alpha float64) MultiAcceptanceState {
	next := state.Copy()
	for _, assetID := range universe.IDs() {
		assetState := next.AssetState(assetID)
		updated := copyState(assetState)
		alpha = clamp01(alpha)
		for _, nodeID := range stateNodeIDs(assetState) {
			score := assetState.Scores[nodeID]
			need := demand.Need(nodeID, assetID, inventory)
			surplus := demand.Surplus(nodeID, assetID, inventory)
			target := demand.Target(nodeID, assetID)
			holding := inventory.Get(nodeID, assetID)
			if need > 0 {
				pressure := need / (target + 1)
				if pressure > 1 {
					pressure = 1
				}
				score = score + alpha*pressure*(1-score)
			}
			if surplus > 0 {
				pressure := surplus / (holding + target + 1)
				if pressure > 1 {
					pressure = 1
				}
				score = score - alpha*pressure*score
			}
			updated.Scores[nodeID] = clamp01(score)
		}
		next = next.SetAssetState(assetID, updated)
	}

	return next
}

func assetSpread(state AcceptanceState) float64 {
	if len(state.Scores) == 0 {
		return 0
	}
	minScore := math.Inf(1)
	maxScore := math.Inf(-1)
	for _, score := range state.Scores {
		if score < minScore {
			minScore = score
		}
		if score > maxScore {
			maxScore = score
		}
	}
	return maxScore - minScore
}

func dominantAsset(means map[string]float64) string {
	ids := make([]string, 0, len(means))
	for assetID := range means {
		ids = append(ids, assetID)
	}
	ids = sortedUnique(ids)
	best := ""
	bestMean := math.Inf(-1)
	for _, assetID := range ids {
		if means[assetID] > bestMean {
			best = assetID
			bestMean = means[assetID]
		}
	}
	return best
}

func setMultiNode(state MultiAcceptanceState, inv InventoryState, demand DemandState, nodeID, assetID string, score float64, holding float64, target float64) {
	state.Set(nodeID, assetID, score)
	inv.Set(nodeID, assetID, holding)
	demand.SetTarget(nodeID, assetID, target)
}

package sim

import (
	"fmt"
	"math"
	"math/rand"

	modelcore "web4-v3/core/model"
	pricecore "web4-v3/core/price"
)

const MarketAssetID = "market-asset"

const (
	PriceModelAcceptance = "acceptance"
	PriceModelPipeline   = "pipeline"
)

type MarketConfig struct {
	Scenario           string
	Topology           string
	Steps              int
	Alpha              float64
	Spread             float64
	MinProfit          float64
	Seed               int64
	TradesPerStep      int
	EnableDemand       bool
	MaxQty             float64
	EnableCycle        bool
	EnableSubstitution bool
	UtilityMode        string
	PriceModel         string
	ConsumptionRate    float64
	ProductionRate     float64
}

type MarketStepMetrics struct {
	Step                    int
	MeanPrice               float64
	MinPrice                float64
	MaxPrice                float64
	PriceSpread             float64
	PriceVariance           float64
	ExecutedTrades          int
	DemandTrades            int
	ArbitrageTrades         int
	TotalVolume             float64
	ProducedVolume          float64
	ConsumedVolume          float64
	AvailableArbitrageCount int
	MaxArbitrageProfit      float64
	LiquidityScore          float64
	GlobalMeanAcceptance    float64
	UnmetDemand             float64
	TotalSurplus            float64
	ClusterMeans            map[string]float64
}

type MarketSummary struct {
	InitialPriceSpread   float64
	FinalPriceSpread     float64
	InitialMeanPrice     float64
	FinalMeanPrice       float64
	TotalTrades          int
	DemandTrades         int
	ArbitrageTrades      int
	TotalVolume          float64
	TotalProduced        float64
	TotalConsumed        float64
	AverageTradesPerStep float64
	AverageVolumePerStep float64
	AverageUnmetDemand   float64
	FinalLiquidityScore  float64
	FinalUnmetDemand     float64
	FinalTotalSurplus    float64
	PriceSpreadDecreased bool
	Converged            bool
	Fragmented           bool
	Collapsed            bool
	Liquid               bool
}

type MarketResult struct {
	Config  MarketConfig
	Initial AcceptanceState
	Final   AcceptanceState
	Metrics []MarketStepMetrics
	Summary MarketSummary
}

func DefaultMarketConfig() MarketConfig {
	return MarketConfig{
		Scenario:   "split",
		Topology:   "full",
		Steps:      1000,
		Alpha:      0.2,
		Spread:     0.01,
		MinProfit:  0.05,
		Seed:       1,
		MaxQty:     1,
		PriceModel: PriceModelAcceptance,
	}
}

func RunMarketSimulation(cfg MarketConfig) (MarketResult, error) {
	if err := ValidateMarketConfig(cfg); err != nil {
		return MarketResult{}, err
	}

	initial, err := MarketScenarioState(cfg.Scenario, cfg.Seed)
	if err != nil {
		return MarketResult{}, err
	}
	inventory, demand := MarketInventoryDemandState(cfg.Scenario, cfg.Seed)
	consumption, production := MarketCycleState(cfg.Scenario, cfg.Seed, cfg.ConsumptionRate, cfg.ProductionRate)
	nodeIDs := stateNodeIDs(initial)
	if cfg.TradesPerStep == 0 {
		cfg.TradesPerStep = len(nodeIDs)
	}
	topology, clusters, err := MarketTopology(cfg.Topology, cfg.Scenario, nodeIDs)
	if err != nil {
		return MarketResult{}, err
	}

	current := copyState(initial)
	currentInventory := inventory.Copy()
	metrics := make([]MarketStepMetrics, 0, cfg.Steps+1)
	totalTrades := 0
	totalDemandTrades := 0
	totalArbitrageTrades := 0
	totalVolume := 0.0
	totalProduced := 0.0
	totalConsumed := 0.0
	tradeObservations := []pricecore.TradeObservation{}
	settledVolume := modelcore.Amount(0)
	lastTradeStep := 0
	metrics = append(metrics, MarketMetrics(0, current, currentInventory, demand, nil, 0, 0, 0, 0, 0, 0, clusters, marketPriceTable(cfg.PriceModel, MarketAssetID, current, tradeObservations, settledVolume, lastTradeStep, 0)))

	for step := 1; step <= cfg.Steps; step++ {
		demandTrades := 0
		arbitrageTrades := 0
		stepVolume := 0.0
		producedVolume := 0.0
		consumedVolume := 0.0
		next := current
		nextInventory := currentInventory
		if cfg.EnableDemand && cfg.EnableCycle {
			nextInventory, consumedVolume = consumption.ApplyWithVolume(nextInventory)
			nextInventory, producedVolume = production.ApplyWithVolume(nextInventory)
			next = StepDemandPricePressure(next, nextInventory, demand, cfg.Alpha)
		}
		pt := marketPriceTable(cfg.PriceModel, MarketAssetID, next, tradeObservations, settledVolume, lastTradeStep, step)
		arbitrage := FindArbitrage(pt, MarketAssetID, cfg.MinProfit)
		if cfg.EnableDemand {
			for _, sellerID := range nodeIDs {
				for _, buyerID := range nodeIDs {
					if demandTrades+arbitrageTrades >= cfg.TradesPerStep {
						break
					}
					if sellerID == buyerID || !marketPairConnected(topology, sellerID, buyerID) {
						continue
					}
					quote := QuoteDemandTrade(pt, nextInventory, demand, sellerID, buyerID, MarketAssetID, cfg.MaxQty, cfg.Spread)
					if !quote.Executable {
						continue
					}
					nextInventory, next = ApplyDemandTrade(nextInventory, next, quote, cfg.Alpha)
					demandTrades++
					stepVolume += quote.Quantity
					volume := modelcore.FromFloat(quote.Quantity)
					if volume > 0 {
						tradeObservations = append(tradeObservations, pricecore.TradeObservation{
							Price:    quote.ClearingPrice,
							Volume:   volume,
							Weight:   tradeObservationWeight(next, quote.Seller, quote.Buyer),
							TimeUnix: int64(step),
						})
						settledVolume = modelcore.Add(settledVolume, volume)
						lastTradeStep = step
					}
				}
			}
		}
		for _, opp := range arbitrage {
			if demandTrades+arbitrageTrades >= cfg.TradesPerStep {
				break
			}
			if !marketPairConnected(topology, opp.BuyFrom, opp.SellTo) {
				continue
			}
			quote := QuoteTrade(pt, opp.BuyFrom, opp.SellTo, MarketAssetID, cfg.Spread)
			if !quote.Executable {
				continue
			}
			next = ApplyTradeFeedback(next, quote, cfg.Alpha)
			arbitrageTrades++
			volume := modelcore.FromFloat(1)
			tradeObservations = append(tradeObservations, pricecore.TradeObservation{
				Price:    quote.ClearingPrice,
				Volume:   volume,
				Weight:   tradeObservationWeight(next, quote.Seller, quote.Buyer),
				TimeUnix: int64(step),
			})
			settledVolume = modelcore.Add(settledVolume, volume)
			lastTradeStep = step
		}
		next = StepMarketTopology(next, topology, cfg.Alpha)
		current = next
		currentInventory = nextInventory
		executed := demandTrades + arbitrageTrades
		totalTrades += executed
		totalDemandTrades += demandTrades
		totalArbitrageTrades += arbitrageTrades
		totalVolume += stepVolume
		totalProduced += producedVolume
		totalConsumed += consumedVolume
		metrics = append(metrics, MarketMetrics(step, current, currentInventory, demand, arbitrage, executed, demandTrades, arbitrageTrades, stepVolume, producedVolume, consumedVolume, clusters, marketPriceTable(cfg.PriceModel, MarketAssetID, current, tradeObservations, settledVolume, lastTradeStep, step)))
	}

	summary := MarketSummaryFromMetrics(metrics, totalTrades, totalDemandTrades, totalArbitrageTrades, totalVolume, totalProduced, totalConsumed, len(nodeIDs), cfg.Steps)
	return MarketResult{
		Config:  cfg,
		Initial: initial,
		Final:   current,
		Metrics: metrics,
		Summary: summary,
	}, nil
}

func MarketScenarioState(name string, seed int64) (AcceptanceState, error) {
	switch name {
	case "split":
		return AcceptanceState{Scores: map[string]float64{"A": 1, "B": 0, "C": 1, "D": 0}}, nil
	case "clustered":
		return AcceptanceState{Scores: map[string]float64{"A": 0.9, "B": 0.85, "C": 0.15, "D": 0.1}}, nil
	case "collapse":
		return AcceptanceState{Scores: map[string]float64{"A": 0.04, "B": 0.03, "C": 0.02, "D": 0.01}}, nil
	case "high-liquidity":
		return AcceptanceState{Scores: map[string]float64{"A": 0.65, "B": 0.55, "C": 0.45, "D": 0.35, "E": 0.75, "F": 0.25}}, nil
	case "fragmented":
		return AcceptanceState{Scores: map[string]float64{"A": 0.95, "B": 0.9, "C": 0.85, "D": 0.15, "E": 0.1, "F": 0.05}}, nil
	case "random":
		rng := rand.New(rand.NewSource(seed))
		scores := make(map[string]float64, 8)
		for _, nodeID := range []string{"A", "B", "C", "D", "E", "F", "G", "H"} {
			scores[nodeID] = rng.Float64()
		}
		return AcceptanceState{Scores: scores}, nil
	case "demand-basic":
		return AcceptanceState{Scores: map[string]float64{"A": 0.25, "B": 0.85, "C": 0.8, "D": 0.75}}, nil
	case "demand-fragmented":
		return AcceptanceState{Scores: map[string]float64{"A": 0.9, "B": 0.85, "C": 0.3, "D": 0.25}}, nil
	case "demand-random":
		rng := rand.New(rand.NewSource(seed))
		scores := make(map[string]float64, 6)
		for _, nodeID := range []string{"A", "B", "C", "D", "E", "F"} {
			scores[nodeID] = 0.25 + 0.65*rng.Float64()
		}
		return AcceptanceState{Scores: scores}, nil
	case "cycle-basic":
		return AcceptanceState{Scores: map[string]float64{"A": 0.25, "B": 0.85, "C": 0.8, "D": 0.75}}, nil
	case "cycle-fragmented":
		return AcceptanceState{Scores: map[string]float64{"A": 0.9, "B": 0.85, "C": 0.35, "D": 0.3}}, nil
	case "cycle-random":
		rng := rand.New(rand.NewSource(seed))
		scores := make(map[string]float64, 6)
		for _, nodeID := range []string{"A", "B", "C", "D", "E", "F"} {
			scores[nodeID] = 0.2 + 0.7*rng.Float64()
		}
		return AcceptanceState{Scores: scores}, nil
	default:
		return AcceptanceState{}, fmt.Errorf("unknown market scenario %q", name)
	}
}

func MarketInventoryDemandState(scenario string, seed int64) (InventoryState, DemandState) {
	inventory := NewInventoryState()
	demand := NewDemandState()
	switch scenario {
	case "demand-basic", "cycle-basic":
		setInventoryDemand(inventory, demand, "A", 10, 2)
		setInventoryDemand(inventory, demand, "B", 0, 3)
		setInventoryDemand(inventory, demand, "C", 0, 3)
		setInventoryDemand(inventory, demand, "D", 0, 2)
	case "demand-fragmented", "cycle-fragmented":
		setInventoryDemand(inventory, demand, "A", 8, 2)
		setInventoryDemand(inventory, demand, "B", 7, 2)
		setInventoryDemand(inventory, demand, "C", 0, 5)
		setInventoryDemand(inventory, demand, "D", 0, 4)
	case "demand-random", "cycle-random":
		rng := rand.New(rand.NewSource(seed + 1000))
		for _, nodeID := range []string{"A", "B", "C", "D", "E", "F"} {
			setInventoryDemand(inventory, demand, nodeID, 5*rng.Float64(), 1+5*rng.Float64())
		}
	}

	return inventory, demand
}

func MarketCycleState(scenario string, seed int64, consumptionRate float64, productionRate float64) (ConsumptionState, ProductionState) {
	consumption := NewConsumptionState()
	production := NewProductionState()
	switch scenario {
	case "cycle-basic":
		consumerRate := consumptionRate
		if consumerRate == 0 {
			consumerRate = 0.1
		}
		producerRate := productionRate
		if producerRate == 0 {
			producerRate = 0.3
		}
		production.SetRate("A", MarketAssetID, producerRate)
		consumption.SetRate("B", MarketAssetID, consumerRate)
		consumption.SetRate("C", MarketAssetID, consumerRate)
		consumption.SetRate("D", MarketAssetID, consumerRate)
	case "cycle-fragmented":
		consumerRate := consumptionRate
		if consumerRate == 0 {
			consumerRate = 0.1
		}
		producerRate := productionRate
		if producerRate == 0 {
			producerRate = 0.25
		}
		production.SetRate("A", MarketAssetID, producerRate)
		production.SetRate("B", MarketAssetID, producerRate)
		consumption.SetRate("C", MarketAssetID, consumerRate)
		consumption.SetRate("D", MarketAssetID, consumerRate)
	case "cycle-random":
		rng := rand.New(rand.NewSource(seed + 2000))
		for _, nodeID := range []string{"A", "B", "C", "D", "E", "F"} {
			consumption.SetRate(nodeID, MarketAssetID, consumptionRate*rng.Float64())
			production.SetRate(nodeID, MarketAssetID, productionRate*rng.Float64())
		}
	}

	return consumption, production
}

func MarketTopology(name string, scenario string, nodeIDs []string) (Topology, [][]string, error) {
	clusters := marketClusters(scenario, nodeIDs)
	switch name {
	case "full":
		return FullMeshTopology(nodeIDs), clusters, nil
	case "chain":
		return ChainTopology(nodeIDs), clusters, nil
	case "clustered":
		return ClusteredTopology(clusters), clusters, nil
	default:
		return Topology{}, nil, fmt.Errorf("unknown market topology %q", name)
	}
}

func StepMarketTopology(state AcceptanceState, topology Topology, alpha float64) AcceptanceState {
	next := AcceptanceState{Scores: make(map[string]float64, len(state.Scores))}
	alpha = clamp01(alpha)
	for _, nodeID := range stateNodeIDs(state) {
		neighborhood := topology.Neighborhood(nodeID, true)
		sum := 0.0
		count := 0
		for _, neighborID := range neighborhood {
			score, ok := state.Scores[neighborID]
			if !ok {
				continue
			}
			sum += score
			count++
		}
		if count == 0 {
			next.Scores[nodeID] = clamp01(state.Scores[nodeID])
			continue
		}
		localMean := sum / float64(count)
		score := state.Scores[nodeID]
		next.Scores[nodeID] = clamp01(score + alpha*(localMean-score))
	}

	return next
}

func StepDemandPricePressure(state AcceptanceState, inventory InventoryState, demand DemandState, alpha float64) AcceptanceState {
	next := copyState(state)
	alpha = clamp01(alpha)
	for _, nodeID := range stateNodeIDs(state) {
		score := state.Scores[nodeID]
		need := demand.Need(nodeID, MarketAssetID, inventory)
		surplus := demand.Surplus(nodeID, MarketAssetID, inventory)
		target := demand.Target(nodeID, MarketAssetID)
		holding := inventory.Get(nodeID, MarketAssetID)
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
		next.Scores[nodeID] = clamp01(score)
	}

	return next
}

func marketPriceTable(
	priceModel string,
	assetID string,
	state AcceptanceState,
	trades []pricecore.TradeObservation,
	settledVolume modelcore.Amount,
	lastTradeStep int,
	step int,
) PriceTable {
	if priceModel == "" || priceModel == PriceModelAcceptance {
		return PriceTableFromAcceptance(assetID, state)
	}

	pt := NewPriceTable()
	mean := AcceptanceMean(state)
	age := clamp01(float64(step) / 1000.0)
	cfg := pricecore.PriceConfig{
		BasePrice:       1,
		Weights:         pricecore.FeatureWeights{Cost: 1, Age: 0.25, Stability: 0.25},
		VolumeThreshold: modelcore.FromFloat(10),
		DecayK:          0.001,
	}
	for _, nodeID := range stateNodeIDs(state) {
		score := clamp01(state.Scores[nodeID])
		features := pricecore.AssetFeatures{
			Cost:      score,
			Age:       age,
			Stability: 1 - math.Abs(score-mean),
		}
		result := pricecore.ComputePrice(features, trades, settledVolume, int64(lastTradeStep), int64(step), cfg)
		pt.Set(nodeID, assetID, result.FinalPrice)
	}
	return pt
}

func tradeObservationWeight(state AcceptanceState, a string, b string) float64 {
	scoreA := clamp01(state.Scores[a])
	scoreB := clamp01(state.Scores[b])
	weight := (scoreA + scoreB) / 2
	if weight <= 0 {
		return 1
	}
	return weight
}

func MarketMetrics(
	step int,
	state AcceptanceState,
	inventory InventoryState,
	demand DemandState,
	arbitrage []ArbitrageOpportunity,
	executedTrades int,
	demandTrades int,
	arbitrageTrades int,
	totalVolume float64,
	producedVolume float64,
	consumedVolume float64,
	clusters [][]string,
	pt PriceTable,
) MarketStepMetrics {
	nodeIDs := stateNodeIDs(state)
	metrics := MarketStepMetrics{
		Step:                    step,
		ExecutedTrades:          executedTrades,
		DemandTrades:            demandTrades,
		ArbitrageTrades:         arbitrageTrades,
		TotalVolume:             totalVolume,
		ProducedVolume:          producedVolume,
		ConsumedVolume:          consumedVolume,
		AvailableArbitrageCount: len(arbitrage),
		GlobalMeanAcceptance:    AcceptanceMean(state),
		UnmetDemand:             TotalUnmetDemand(demand, inventory, MarketAssetID),
		TotalSurplus:            TotalDemandSurplus(demand, inventory, MarketAssetID),
		ClusterMeans:            clusterMeans(state, clusters),
	}
	if len(arbitrage) > 0 {
		metrics.MaxArbitrageProfit = arbitrage[0].Profit
	}
	if len(nodeIDs) == 0 {
		return metrics
	}

	metrics.MinPrice = math.Inf(1)
	metrics.MaxPrice = math.Inf(-1)
	prices := make([]float64, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		price, ok := pt.Get(nodeID, MarketAssetID)
		if !ok {
			continue
		}
		prices = append(prices, price)
		metrics.MeanPrice += price
		if price < metrics.MinPrice {
			metrics.MinPrice = price
		}
		if price > metrics.MaxPrice {
			metrics.MaxPrice = price
		}
	}
	if len(prices) == 0 {
		metrics.MinPrice = 0
		metrics.MaxPrice = 0
		return metrics
	}

	metrics.MeanPrice /= float64(len(prices))
	for _, price := range prices {
		delta := price - metrics.MeanPrice
		metrics.PriceVariance += delta * delta
	}
	metrics.PriceVariance /= float64(len(prices))
	metrics.PriceSpread = metrics.MaxPrice - metrics.MinPrice
	metrics.LiquidityScore = liquidityScore(executedTrades, len(nodeIDs))

	return metrics
}

func MarketSummaryFromMetrics(
	metrics []MarketStepMetrics,
	totalTrades int,
	demandTrades int,
	arbitrageTrades int,
	totalVolume float64,
	totalProduced float64,
	totalConsumed float64,
	nodeCount int,
	steps int,
) MarketSummary {
	if len(metrics) == 0 {
		return MarketSummary{}
	}

	initial := metrics[0]
	final := metrics[len(metrics)-1]
	possiblePairs := nodeCount * (nodeCount - 1)
	summary := MarketSummary{
		InitialPriceSpread:   initial.PriceSpread,
		FinalPriceSpread:     final.PriceSpread,
		InitialMeanPrice:     initial.MeanPrice,
		FinalMeanPrice:       final.MeanPrice,
		TotalTrades:          totalTrades,
		DemandTrades:         demandTrades,
		ArbitrageTrades:      arbitrageTrades,
		TotalVolume:          totalVolume,
		TotalProduced:        totalProduced,
		TotalConsumed:        totalConsumed,
		FinalLiquidityScore:  final.LiquidityScore,
		FinalUnmetDemand:     final.UnmetDemand,
		FinalTotalSurplus:    final.TotalSurplus,
		PriceSpreadDecreased: final.PriceSpread < initial.PriceSpread,
		Converged:            final.PriceSpread < 0.05,
		Fragmented:           clusterSpread(final.ClusterMeans) > 0.2,
		Collapsed:            final.MeanPrice < 0.05,
	}
	if steps > 0 {
		summary.AverageTradesPerStep = float64(totalTrades) / float64(steps)
		summary.AverageVolumePerStep = totalVolume / float64(steps)
		summary.AverageUnmetDemand = averageUnmetDemand(metrics)
	}
	if possiblePairs > 0 {
		summary.Liquid = summary.AverageTradesPerStep > 0.1*float64(possiblePairs)
	}

	return summary
}

func ValidateMarketConfig(cfg MarketConfig) error {
	switch cfg.Scenario {
	case "split", "clustered", "random", "collapse", "high-liquidity", "fragmented", "demand-basic", "demand-fragmented", "demand-random", "cycle-basic", "cycle-fragmented", "cycle-random":
	default:
		return fmt.Errorf("unknown market scenario %q", cfg.Scenario)
	}
	switch cfg.Topology {
	case "full", "chain", "clustered":
	default:
		return fmt.Errorf("unknown market topology %q", cfg.Topology)
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
	if cfg.MinProfit < 0 {
		return fmt.Errorf("min-profit must be >= 0")
	}
	if cfg.TradesPerStep < 0 {
		return fmt.Errorf("trades-per-step must be >= 0")
	}
	if cfg.MaxQty <= 0 {
		return fmt.Errorf("max-qty must be > 0")
	}
	if cfg.ConsumptionRate < 0 {
		return fmt.Errorf("consumption-rate must be >= 0")
	}
	if cfg.ProductionRate < 0 {
		return fmt.Errorf("production-rate must be >= 0")
	}
	switch cfg.PriceModel {
	case "", PriceModelAcceptance, PriceModelPipeline:
	default:
		return fmt.Errorf("unknown price model %q", cfg.PriceModel)
	}

	return nil
}

func marketClusters(scenario string, nodeIDs []string) [][]string {
	switch scenario {
	case "split":
		return [][]string{{"A", "C"}, {"B", "D"}}
	case "clustered":
		return [][]string{{"A", "B"}, {"C", "D"}}
	case "fragmented":
		return [][]string{{"A", "B", "C"}, {"D", "E", "F"}}
	case "high-liquidity":
		return [][]string{{"A", "B", "C"}, {"D", "E", "F"}}
	case "random":
		if len(nodeIDs) <= 1 {
			return [][]string{append([]string(nil), nodeIDs...)}
		}
		mid := len(nodeIDs) / 2
		return [][]string{append([]string(nil), nodeIDs[:mid]...), append([]string(nil), nodeIDs[mid:]...)}
	case "demand-basic", "cycle-basic":
		return [][]string{{"A", "B", "C", "D"}}
	case "demand-fragmented", "cycle-fragmented":
		return [][]string{{"A", "B"}, {"C", "D"}}
	case "multi-fragmented":
		return [][]string{{"A", "B"}, {"C", "D"}}
	case "multi-basic":
		return [][]string{{"A", "B", "C", "D"}}
	case "multi-compete", "multi-flight":
		return [][]string{{"A", "B", "C"}, {"D", "E", "F"}}
	case "demand-random", "cycle-random":
		if len(nodeIDs) <= 1 {
			return [][]string{append([]string(nil), nodeIDs...)}
		}
		mid := len(nodeIDs) / 2
		return [][]string{append([]string(nil), nodeIDs[:mid]...), append([]string(nil), nodeIDs[mid:]...)}
	default:
		return [][]string{append([]string(nil), nodeIDs...)}
	}
}

func averageUnmetDemand(metrics []MarketStepMetrics) float64 {
	if len(metrics) == 0 {
		return 0
	}

	total := 0.0
	for _, metric := range metrics {
		total += metric.UnmetDemand
	}

	return total / float64(len(metrics))
}

func TotalUnmetDemand(demand DemandState, inventory InventoryState, assetID string) float64 {
	total := 0.0
	for nodeID := range demand.Targets {
		total += demand.Need(nodeID, assetID, inventory)
	}

	return total
}

func TotalDemandSurplus(demand DemandState, inventory InventoryState, assetID string) float64 {
	total := 0.0
	for nodeID := range demand.Targets {
		total += demand.Surplus(nodeID, assetID, inventory)
	}

	return total
}

func setInventoryDemand(inventory InventoryState, demand DemandState, nodeID string, holding float64, target float64) {
	inventory.Set(nodeID, MarketAssetID, holding)
	demand.SetTarget(nodeID, MarketAssetID, target)
}

func liquidityScore(executedTrades int, nodeCount int) float64 {
	possiblePairs := nodeCount * (nodeCount - 1)
	if possiblePairs == 0 {
		return 0
	}

	return float64(executedTrades) / float64(possiblePairs)
}

func clusterMeans(state AcceptanceState, clusters [][]string) map[string]float64 {
	if len(clusters) == 0 {
		return nil
	}

	means := make(map[string]float64, len(clusters))
	for i, cluster := range clusters {
		sum := 0.0
		count := 0
		for _, nodeID := range cluster {
			score, ok := state.Scores[nodeID]
			if !ok {
				continue
			}
			sum += PriceFromAcceptance(score)
			count++
		}
		if count == 0 {
			continue
		}
		means[fmt.Sprintf("cluster_%d", i+1)] = sum / float64(count)
	}

	return means
}

func clusterSpread(means map[string]float64) float64 {
	if len(means) <= 1 {
		return 0
	}

	minMean := math.Inf(1)
	maxMean := math.Inf(-1)
	for _, mean := range means {
		if mean < minMean {
			minMean = mean
		}
		if mean > maxMean {
			maxMean = mean
		}
	}

	return maxMean - minMean
}

func marketPairConnected(topology Topology, a string, b string) bool {
	for _, neighborID := range topology.Neighborhood(a, false) {
		if neighborID == b {
			return true
		}
	}

	return false
}

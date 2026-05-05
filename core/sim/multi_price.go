package sim

import (
	"web4-v3/core/crypto"
	modelcore "web4-v3/core/model"
	pricecore "web4-v3/core/price"
)

type MultiPriceState struct {
	Features            map[modelcore.UnitID]pricecore.AssetFeatures
	Trades              map[modelcore.UnitID][]pricecore.TradeObservation
	SettledVolume       map[modelcore.UnitID]modelcore.Amount
	LastTradeUnix       map[modelcore.UnitID]int64
	WeightedPriceVolume map[modelcore.UnitID]float64
	WeightedVolume      map[modelcore.UnitID]float64
}

func NewMultiPriceState(universe AssetUniverse, state MultiAcceptanceState) MultiPriceState {
	ps := MultiPriceState{
		Features:            make(map[modelcore.UnitID]pricecore.AssetFeatures),
		Trades:              make(map[modelcore.UnitID][]pricecore.TradeObservation),
		SettledVolume:       make(map[modelcore.UnitID]modelcore.Amount),
		LastTradeUnix:       make(map[modelcore.UnitID]int64),
		WeightedPriceVolume: make(map[modelcore.UnitID]float64),
		WeightedVolume:      make(map[modelcore.UnitID]float64),
	}
	for _, assetID := range universe.IDs() {
		unit := UnitIDForAsset(assetID)
		assetState := state.AssetState(assetID)
		ps.Features[unit] = pricecore.AssetFeatures{
			Cost:      AcceptanceMean(assetState),
			Age:       1,
			Stability: 1 - assetSpread(assetState),
		}
		ps.Trades[unit] = nil
		ps.SettledVolume[unit] = 0
		ps.LastTradeUnix[unit] = 0
	}
	return ps
}

func UnitIDForAsset(assetID string) modelcore.UnitID {
	return modelcore.UnitID(crypto.HashBytes([]byte("web4-sim-asset:" + assetID)))
}

func PriceTableFromMultiPipeline(
	state MultiPriceState,
	cfg pricecore.PriceConfig,
	nowUnix int64,
) map[modelcore.UnitID]float64 {
	out := make(map[modelcore.UnitID]float64, len(state.Features))
	for unit, features := range state.Features {
		trades := state.Trades[unit]
		if state.WeightedVolume[unit] > 0 {
			trades = []pricecore.TradeObservation{{
				Price:  state.WeightedPriceVolume[unit] / state.WeightedVolume[unit],
				Volume: modelcore.FromFloat(state.WeightedVolume[unit]),
				Weight: 1,
			}}
		}
		result := pricecore.ComputePrice(
			features,
			trades,
			state.SettledVolume[unit],
			state.LastTradeUnix[unit],
			nowUnix,
			cfg,
		)
		out[unit] = result.FinalPrice
	}
	return out
}

func priceTableFromMultiPipelineForNodes(ps MultiPriceState, cfg pricecore.PriceConfig, universe AssetUniverse, inv InventoryState, demand DemandState, nodeIDs []string, nowUnix int64) PriceTable {
	assetPrices := PriceTableFromMultiPipeline(ps, cfg, nowUnix)
	pt := NewPriceTable()
	for _, nodeID := range nodeIDs {
		for _, assetID := range universe.IDs() {
			pt.Set(nodeID, assetID, assetPrices[UnitIDForAsset(assetID)]*inventoryPressureMultiplier(inv, demand, nodeID, assetID))
		}
	}
	return pt
}

func inventoryPressureMultiplier(inv InventoryState, demand DemandState, nodeID string, assetID string) float64 {
	need := demand.Need(nodeID, assetID, inv)
	surplus := demand.Surplus(nodeID, assetID, inv)
	target := demand.Target(nodeID, assetID)
	holding := inv.Get(nodeID, assetID)
	if need > 0 {
		return 1 + 0.5*clamp01(need/(target+1))
	}
	if surplus > 0 {
		return 1 - 0.5*clamp01(surplus/(holding+target+1))
	}
	return 1
}

func recordSameAssetObservation(ps *MultiPriceState, assetID string, price float64, volume float64, weight float64, nowUnix int64) {
	amount := modelcore.FromFloat(volume)
	if amount <= 0 {
		return
	}
	unit := UnitIDForAsset(assetID)
	observation := pricecore.TradeObservation{
		Price:    price,
		Volume:   amount,
		Weight:   nonZeroWeight(weight),
		TimeUnix: nowUnix,
	}
	if len(ps.Trades[unit]) < 1024 {
		ps.Trades[unit] = append(ps.Trades[unit], observation)
	}
	weightedVolume := modelcore.ToFloat(amount) * observation.Weight
	ps.WeightedPriceVolume[unit] += observation.Price * weightedVolume
	ps.WeightedVolume[unit] += weightedVolume
	ps.SettledVolume[unit] = modelcore.Add(ps.SettledVolume[unit], amount)
	ps.LastTradeUnix[unit] = nowUnix
}

func recordCrossAssetObservations(ps *MultiPriceState, quote CrossAssetTradeQuote, sellWeight float64, buyWeight float64, nowUnix int64) {
	if !quote.Executable || quote.SellQty <= 0 || quote.BuyQty <= 0 {
		return
	}
	recordSameAssetObservation(ps, quote.SellAsset, quote.BuyQty/quote.SellQty, quote.SellQty, sellWeight, nowUnix)
	recordSameAssetObservation(ps, quote.BuyAsset, quote.SellQty/quote.BuyQty, quote.BuyQty, buyWeight, nowUnix)
}

func sameAssetObservationWeight(state MultiAcceptanceState, assetID string, sellerID string, buyerID string) float64 {
	return (clamp01(state.Get(sellerID, assetID)) + clamp01(state.Get(buyerID, assetID))) / 2
}

func crossAssetObservationWeight(state MultiAcceptanceState, assetID string, sellerID string, buyerID string) float64 {
	return (clamp01(state.Get(sellerID, assetID)) + clamp01(state.Get(buyerID, assetID))) / 2
}

func nonZeroWeight(weight float64) float64 {
	if weight <= 0 {
		return 1
	}
	return weight
}

func defaultMultiPipelinePriceConfig() pricecore.PriceConfig {
	return pricecore.PriceConfig{
		BasePrice:       1,
		Weights:         pricecore.FeatureWeights{Cost: 1, Age: 0.25, Stability: 0.25},
		VolumeThreshold: modelcore.FromFloat(10),
		DecayK:          0.001,
	}
}

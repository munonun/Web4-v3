package price

import (
	"math"

	"web4-v3/core/model"
)

type PriceConfig struct {
	BasePrice       float64
	Weights         FeatureWeights
	VolumeThreshold model.Amount
	DecayK          float64
}

type PriceResult struct {
	FeatureScore   float64
	SeedPrice      float64
	MarketPrice    float64
	HasMarketPrice bool
	Lambda         float64
	FinalPrice     float64
}

func ComputePrice(
	features AssetFeatures,
	trades []TradeObservation,
	settledVolume model.Amount,
	lastTradeUnix int64,
	nowUnix int64,
	cfg PriceConfig,
) PriceResult {
	score := FeatureScore(features, cfg.Weights)
	seed := SeedPrice(cfg.BasePrice, score)
	market, hasMarket := WeightedVWAP(trades)
	lambda := 0.0
	price := seed

	if hasMarket {
		lambda = VolumeLambda(settledVolume, cfg.VolumeThreshold)
		price = BlendPrice(seed, market, lambda)
	}

	if nowUnix > lastTradeUnix {
		price = ApplyDecay(price, cfg.DecayK, float64(nowUnix-lastTradeUnix))
	}

	return PriceResult{
		FeatureScore:   score,
		SeedPrice:      seed,
		MarketPrice:    market,
		HasMarketPrice: hasMarket,
		Lambda:         lambda,
		FinalPrice:     price,
	}
}

func clamp01(v float64) float64 {
	if math.IsNaN(v) || v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func nonNegative(v float64) float64 {
	if math.IsNaN(v) || v < 0 {
		return 0
	}
	if math.IsInf(v, 1) {
		return math.MaxFloat64
	}
	return v
}

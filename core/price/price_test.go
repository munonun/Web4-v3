package price

import (
	"math"
	"testing"

	"web4-v3/core/model"
)

func TestFeatureScore(t *testing.T) {
	weights := FeatureWeights{Cost: 1, Age: 1, Stability: 2}
	low := FeatureScore(AssetFeatures{Cost: 0.1, Age: 0.2, Stability: 0.3}, weights)
	high := FeatureScore(AssetFeatures{Cost: 0.8, Age: 0.9, Stability: 1.0}, weights)

	if low < 0 || low > 1 || high < 0 || high > 1 {
		t.Fatalf("scores out of range: low=%f high=%f", low, high)
	}
	if high <= low {
		t.Fatalf("high feature score %f should exceed low score %f", high, low)
	}
}

func TestFeatureWeightsNormalizeClampsNegatives(t *testing.T) {
	got := (FeatureWeights{Cost: -1, Age: 1, Stability: 3}).Normalize()
	if got.Cost != 0 {
		t.Fatalf("negative cost weight normalized to %f, want 0", got.Cost)
	}
	assertApprox(t, got.Age, 0.25)
	assertApprox(t, got.Stability, 0.75)

	equal := (FeatureWeights{}).Normalize()
	assertApprox(t, equal.Cost, 1.0/3.0)
	assertApprox(t, equal.Age, 1.0/3.0)
	assertApprox(t, equal.Stability, 1.0/3.0)
}

func TestSeedPrice(t *testing.T) {
	assertApprox(t, SeedPrice(10, 0.4), 4)
	assertApprox(t, SeedPrice(10, 2), 10)
	assertApprox(t, SeedPrice(-10, 0.5), 0)
}

func TestWeightedVWAP(t *testing.T) {
	price, ok := WeightedVWAP([]TradeObservation{{
		Price:  7,
		Volume: model.FromFloat(2),
		Weight: 1,
	}})
	if !ok {
		t.Fatal("expected single trade market price")
	}
	assertApprox(t, price, 7)

	price, ok = WeightedVWAP([]TradeObservation{
		{Price: 10, Volume: model.FromFloat(1), Weight: 1},
		{Price: 20, Volume: model.FromFloat(3), Weight: 2},
		{Price: 999, Volume: 0, Weight: 1},
		{Price: 999, Volume: model.FromFloat(1), Weight: 0},
	})
	if !ok {
		t.Fatal("expected weighted market price")
	}
	assertApprox(t, price, 130.0/7.0)

	if _, ok := WeightedVWAP([]TradeObservation{{Price: -1, Volume: model.FromFloat(1), Weight: 1}}); ok {
		t.Fatal("expected no valid trades")
	}
}

func TestVolumeLambda(t *testing.T) {
	threshold := model.FromFloat(10)
	assertApprox(t, VolumeLambda(0, threshold), 0)
	assertApprox(t, VolumeLambda(model.FromFloat(5), threshold), 0.5)
	assertApprox(t, VolumeLambda(model.FromFloat(15), threshold), 1)
	assertApprox(t, VolumeLambda(model.FromFloat(1), 0), 0)
}

func TestBlendPrice(t *testing.T) {
	assertApprox(t, BlendPrice(2, 10, 0), 2)
	assertApprox(t, BlendPrice(2, 10, 1), 10)
	assertApprox(t, BlendPrice(2, 10, 0.5), 6)
	assertApprox(t, BlendPrice(-2, 10, 0.5), 5)
}

func TestApplyDecay(t *testing.T) {
	assertApprox(t, ApplyDecay(10, 0, 100), 10)
	assertApprox(t, ApplyDecay(10, 0.1, 0), 10)
	got := ApplyDecay(10, 0.1, 5)
	if got >= 10 || got < 0 {
		t.Fatalf("decayed price %f, want in [0,10)", got)
	}
}

func TestComputePriceNoTradesUsesSeed(t *testing.T) {
	cfg := PriceConfig{
		BasePrice:       10,
		Weights:         FeatureWeights{Cost: 1, Age: 0, Stability: 0},
		VolumeThreshold: model.FromFloat(10),
	}
	result := ComputePrice(AssetFeatures{Cost: 0.6}, nil, 0, 100, 100, cfg)

	if result.HasMarketPrice {
		t.Fatal("expected no market price")
	}
	assertApprox(t, result.FeatureScore, 0.6)
	assertApprox(t, result.SeedPrice, 6)
	assertApprox(t, result.FinalPrice, 6)
}

func TestComputePriceLowVolumeSeedDominates(t *testing.T) {
	cfg := PriceConfig{
		BasePrice:       10,
		Weights:         FeatureWeights{Cost: 1},
		VolumeThreshold: model.FromFloat(10),
	}
	trades := []TradeObservation{{Price: 20, Volume: model.FromFloat(1), Weight: 1}}
	result := ComputePrice(AssetFeatures{Cost: 0.5}, trades, model.FromFloat(1), 100, 100, cfg)

	assertApprox(t, result.SeedPrice, 5)
	assertApprox(t, result.MarketPrice, 20)
	assertApprox(t, result.Lambda, 0.1)
	assertApprox(t, result.FinalPrice, 6.5)
}

func TestComputePriceHighVolumeMarketDominates(t *testing.T) {
	cfg := PriceConfig{
		BasePrice:       10,
		Weights:         FeatureWeights{Cost: 1},
		VolumeThreshold: model.FromFloat(10),
	}
	trades := []TradeObservation{{Price: 20, Volume: model.FromFloat(20), Weight: 1}}
	result := ComputePrice(AssetFeatures{Cost: 0.5}, trades, model.FromFloat(20), 100, 100, cfg)

	assertApprox(t, result.Lambda, 1)
	assertApprox(t, result.FinalPrice, 20)
}

func TestComputePriceInactiveAssetDecays(t *testing.T) {
	cfg := PriceConfig{
		BasePrice:       10,
		Weights:         FeatureWeights{Cost: 1},
		VolumeThreshold: model.FromFloat(10),
		DecayK:          0.1,
	}
	result := ComputePrice(AssetFeatures{Cost: 1}, nil, 0, 100, 110, cfg)

	if result.FinalPrice >= result.SeedPrice || result.FinalPrice < 0 {
		t.Fatalf("inactive final price %f, seed %f", result.FinalPrice, result.SeedPrice)
	}
}

func TestComputePriceActiveAssetPreservesMarketPrice(t *testing.T) {
	cfg := PriceConfig{
		BasePrice:       10,
		Weights:         FeatureWeights{Cost: 1},
		VolumeThreshold: model.FromFloat(10),
		DecayK:          0.1,
	}
	trades := []TradeObservation{{Price: 20, Volume: model.FromFloat(10), Weight: 1}}
	result := ComputePrice(AssetFeatures{Cost: 0.5}, trades, model.FromFloat(10), 100, 100, cfg)

	assertApprox(t, result.FinalPrice, 20)
}

func assertApprox(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got %f, want %f", got, want)
	}
}

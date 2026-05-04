package price

import "web4-v3/core/model"

type TradeObservation struct {
	Price    float64
	Volume   model.Amount
	Weight   float64
	TimeUnix int64
}

func WeightedVWAP(trades []TradeObservation) (float64, bool) {
	numerator := 0.0
	denominator := 0.0

	for _, trade := range trades {
		if !validTradeObservation(trade) {
			continue
		}
		weightedVolume := model.ToFloat(trade.Volume) * trade.Weight
		numerator += nonNegative(trade.Price) * weightedVolume
		denominator += weightedVolume
	}

	if denominator == 0 {
		return 0, false
	}
	return numerator / denominator, true
}

func VolumeLambda(settledVolume model.Amount, threshold model.Amount) float64 {
	if settledVolume <= 0 || threshold <= 0 {
		return 0
	}
	if settledVolume >= threshold {
		return 1
	}
	return float64(settledVolume) / float64(threshold)
}

func validTradeObservation(trade TradeObservation) bool {
	return trade.Price >= 0 && trade.Volume > 0 && trade.Weight > 0
}

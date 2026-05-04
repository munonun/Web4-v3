package price

type AssetFeatures struct {
	Cost      float64
	Age       float64
	Stability float64
}

type FeatureWeights struct {
	Cost      float64
	Age       float64
	Stability float64
}

// Normalize clamps negative weights to zero. If all weights are zero, it
// returns equal weights so feature scoring remains total and deterministic.
func (w FeatureWeights) Normalize() FeatureWeights {
	w.Cost = nonNegative(w.Cost)
	w.Age = nonNegative(w.Age)
	w.Stability = nonNegative(w.Stability)

	total := w.Cost + w.Age + w.Stability
	if total == 0 {
		return FeatureWeights{
			Cost:      1.0 / 3.0,
			Age:       1.0 / 3.0,
			Stability: 1.0 / 3.0,
		}
	}

	return FeatureWeights{
		Cost:      w.Cost / total,
		Age:       w.Age / total,
		Stability: w.Stability / total,
	}
}

func FeatureScore(features AssetFeatures, weights FeatureWeights) float64 {
	w := weights.Normalize()
	score := clamp01(features.Cost)*w.Cost +
		clamp01(features.Age)*w.Age +
		clamp01(features.Stability)*w.Stability

	return clamp01(score)
}

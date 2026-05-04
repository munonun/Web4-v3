package price

func SeedPrice(basePrice float64, score float64) float64 {
	return nonNegative(basePrice) * clamp01(score)
}

func BlendPrice(seedPrice float64, marketPrice float64, lambda float64) float64 {
	seed := nonNegative(seedPrice)
	market := nonNegative(marketPrice)
	l := clamp01(lambda)
	return (1-l)*seed + l*market
}

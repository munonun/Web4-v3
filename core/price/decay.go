package price

import "math"

func ApplyDecay(price float64, k float64, deltaSeconds float64) float64 {
	p := nonNegative(price)
	rate := nonNegative(k)
	delta := nonNegative(deltaSeconds)
	if p == 0 || rate == 0 || delta == 0 {
		return p
	}
	return p * math.Exp(-rate*delta)
}

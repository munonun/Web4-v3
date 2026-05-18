package model

import (
	"fmt"
	"math"
)

type Amount int64

const AmountScale int64 = 1_000_000

func FromFloat(f float64) Amount {
	amount, err := FromFloatChecked(f)
	if err != nil {
		panic(err)
	}
	return amount
}

func FromFloatChecked(f float64) (Amount, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("invalid amount")
	}
	if f < 0 {
		return 0, fmt.Errorf("amount must be non-negative")
	}
	scaled := f * float64(AmountScale)
	if math.IsNaN(scaled) || math.IsInf(scaled, 0) {
		return 0, fmt.Errorf("amount overflow")
	}
	rounded := math.Round(scaled)
	maxInt64Float := math.Nextafter(float64(math.MaxInt64), 0)
	if rounded < 0 || rounded > maxInt64Float {
		return 0, fmt.Errorf("amount overflow")
	}
	return Amount(rounded), nil
}

func ToFloat(a Amount) float64 {
	return float64(a) / float64(AmountScale)
}

func Add(a, b Amount) Amount {
	if (b > 0 && a > Amount(math.MaxInt64)-b) || (b < 0 && a < Amount(math.MinInt64)-b) {
		panic("amount overflow")
	}
	return a + b
}

func CheckedAdd(a, b Amount) (Amount, error) {
	if (b > 0 && a > Amount(math.MaxInt64)-b) || (b < 0 && a < Amount(math.MinInt64)-b) {
		return 0, fmt.Errorf("amount overflow")
	}
	return a + b, nil
}

func Sub(a, b Amount) (Amount, error) {
	if b < 0 {
		return 0, fmt.Errorf("amount must be non-negative")
	}
	if a < b {
		return 0, fmt.Errorf("insufficient amount")
	}
	return a - b, nil
}

func Mul(a Amount, ratio float64) Amount {
	if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 {
		panic("invalid amount ratio")
	}
	scaled := float64(a) * ratio
	if scaled > float64(math.MaxInt64) {
		panic("amount overflow")
	}
	return Amount(math.Round(scaled))
}

func validAmount(amount Amount) bool {
	return amount > 0
}

func validFlowAmount(amount Amount) bool {
	return amount > 0
}

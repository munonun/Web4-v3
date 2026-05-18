package model

import (
	"math"
	"testing"
)

func TestAmountFixedPointConversion(t *testing.T) {
	if got := FromFloat(1.0); got != Amount(AmountScale) {
		t.Fatalf("FromFloat(1.0) = %d, want %d", got, AmountScale)
	}
	if got := FromFloat(0.5); got != 500_000 {
		t.Fatalf("FromFloat(0.5) = %d, want 500000", got)
	}
	if got := FromFloat(0.0000014); got != 1 {
		t.Fatalf("round down = %d, want 1", got)
	}
	if got := FromFloat(0.0000015); got != 2 {
		t.Fatalf("round up = %d, want 2", got)
	}
	if got := ToFloat(1_250_000); got != 1.25 {
		t.Fatalf("ToFloat = %f, want 1.25", got)
	}
}

func TestFromFloatCheckedRejectsInvalidAndOverflow(t *testing.T) {
	for _, value := range []float64{
		math.NaN(),
		math.Inf(1),
		math.Inf(-1),
		-1,
		float64(math.MaxInt64) / float64(AmountScale),
		math.MaxFloat64,
	} {
		if _, err := FromFloatChecked(value); err == nil {
			t.Fatalf("FromFloatChecked(%v) succeeded", value)
		}
	}
}

func TestFromFloatMaxBoundaryDoesNotWrapNegative(t *testing.T) {
	value := math.Nextafter(float64(math.MaxInt64)/float64(AmountScale), 0)
	amount, err := FromFloatChecked(value)
	if err != nil {
		t.Fatalf("max safe boundary rejected: %v", err)
	}
	if amount < 0 {
		t.Fatalf("max boundary wrapped negative: %d", amount)
	}
}

func TestFromFloatPanicsPredictablyOnInvalidInput(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected FromFloat to panic on invalid input")
		}
	}()
	_ = FromFloat(math.Inf(1))
}

func TestAmountArithmeticExactness(t *testing.T) {
	a := FromFloat(0.1)
	b := FromFloat(0.2)
	c := FromFloat(0.3)

	if got := Add(a, b); got != c {
		t.Fatalf("0.1 + 0.2 = %d, want %d", got, c)
	}
	if got, err := Sub(c, b); err != nil || got != a {
		t.Fatalf("0.3 - 0.2 = %d, %v; want %d, nil", got, err, a)
	}
}

func TestAmountSubRejectsNegativeResult(t *testing.T) {
	if _, err := Sub(1, 2); err == nil {
		t.Fatal("expected negative result to fail")
	}
}

func TestAmountMulRoundsDeterministically(t *testing.T) {
	if got := Mul(FromFloat(1.0), 0.3333333); got != 333_333 {
		t.Fatalf("Mul rounded to %d, want 333333", got)
	}
}

func TestAmountNoDriftOverRepeatedOperations(t *testing.T) {
	total := Amount(0)
	step := FromFloat(0.1)
	for i := 0; i < 10_000; i++ {
		total = Add(total, step)
	}
	if want := FromFloat(1000); total != want {
		t.Fatalf("total = %d, want %d", total, want)
	}
}

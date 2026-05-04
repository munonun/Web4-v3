package model

import "testing"

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

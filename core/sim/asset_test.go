package sim

import "testing"

func TestAssetUniverseSortsAndDeduplicates(t *testing.T) {
	u := NewAssetUniverse([]string{"WEB4", "SKUG", "WEB4"})
	ids := u.IDs()

	if len(ids) != 2 || ids[0] != "SKUG" || ids[1] != "WEB4" {
		t.Fatalf("unexpected asset IDs: %#v", ids)
	}
	ids[0] = "changed"
	if u.IDs()[0] != "SKUG" {
		t.Fatal("expected IDs to return a copy")
	}
}

func TestMultiAcceptanceStateSetGetClamp(t *testing.T) {
	state := NewMultiAcceptanceState()
	state.Set("A", "SKUG", 1.2)
	state.Set("A", "WEB4", -0.2)

	assertApprox(t, state.Get("A", "SKUG"), 1)
	assertApprox(t, state.Get("A", "WEB4"), 0)
	assertApprox(t, state.Get("missing", "SKUG"), 0)
}

func TestMultiAcceptanceStateCopyDoesNotMutateOriginal(t *testing.T) {
	state := NewMultiAcceptanceState()
	state.Set("A", "SKUG", 0.5)

	copied := state.Copy()
	copied.Set("A", "SKUG", 0.9)

	assertApprox(t, state.Get("A", "SKUG"), 0.5)
	assertApprox(t, copied.Get("A", "SKUG"), 0.9)
}

func TestMultiAcceptanceAssetState(t *testing.T) {
	state := NewMultiAcceptanceState()
	state.Set("A", "SKUG", 0.5)
	state.Set("B", "SKUG", 0.8)
	state.Set("A", "WEB4", 0.2)

	assetState := state.AssetState("SKUG")

	assertApprox(t, assetState.Scores["A"], 0.5)
	assertApprox(t, assetState.Scores["B"], 0.8)
}

func TestMultiAcceptanceSetAssetState(t *testing.T) {
	state := NewMultiAcceptanceState()
	state.Set("A", "SKUG", 0.5)
	state.Set("A", "WEB4", 0.2)

	next := state.SetAssetState("SKUG", AcceptanceState{Scores: map[string]float64{"A": 0.9, "B": 0.4}})

	assertApprox(t, state.Get("A", "SKUG"), 0.5)
	assertApprox(t, next.Get("A", "SKUG"), 0.9)
	assertApprox(t, next.Get("B", "SKUG"), 0.4)
	assertApprox(t, next.Get("A", "WEB4"), 0.2)
}

func TestPriceTableFromMultiAcceptance(t *testing.T) {
	state := NewMultiAcceptanceState()
	state.Set("A", "SKUG", 0.6)
	state.Set("A", "WEB4", 0.4)

	pt := PriceTableFromMultiAcceptance(state)
	price, ok := pt.Get("A", "SKUG")
	if !ok {
		t.Fatal("expected SKUG price")
	}
	assertApprox(t, price, 0.6)
}

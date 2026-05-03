package sim

import "testing"

func TestProductionSetGetRate(t *testing.T) {
	production := NewProductionState()
	production.SetRate("A", "asset-1", 0.3)
	production.SetRate("B", "asset-1", -1)

	assertApprox(t, production.Rate("A", "asset-1"), 0.3)
	assertApprox(t, production.Rate("B", "asset-1"), 0)
	assertApprox(t, production.Rate("missing", "asset-1"), 0)
}

func TestProductionApplyIncreasesHoldings(t *testing.T) {
	inv := NewInventoryState()
	inv.Set("A", "asset-1", 1)
	production := NewProductionState()
	production.SetRate("A", "asset-1", 0.4)

	next, volume := production.ApplyWithVolume(inv)

	assertApprox(t, next.Get("A", "asset-1"), 1.4)
	assertApprox(t, volume, 0.4)
	assertApprox(t, inv.Get("A", "asset-1"), 1)
}

func TestProductionCopyDoesNotMutateOriginal(t *testing.T) {
	production := NewProductionState()
	production.SetRate("A", "asset-1", 0.3)

	copied := production.Copy()
	copied.SetRate("A", "asset-1", 0.9)

	assertApprox(t, production.Rate("A", "asset-1"), 0.3)
	assertApprox(t, copied.Rate("A", "asset-1"), 0.9)
}

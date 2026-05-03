package sim

import "testing"

func TestConsumptionSetGetRate(t *testing.T) {
	consumption := NewConsumptionState()
	consumption.SetRate("A", "asset-1", 0.3)
	consumption.SetRate("B", "asset-1", -1)

	assertApprox(t, consumption.Rate("A", "asset-1"), 0.3)
	assertApprox(t, consumption.Rate("B", "asset-1"), 0)
	assertApprox(t, consumption.Rate("missing", "asset-1"), 0)
}

func TestConsumptionApplyReducesHoldings(t *testing.T) {
	inv := NewInventoryState()
	inv.Set("A", "asset-1", 1)
	consumption := NewConsumptionState()
	consumption.SetRate("A", "asset-1", 0.4)

	next := consumption.Apply(inv)

	assertApprox(t, next.Get("A", "asset-1"), 0.6)
	assertApprox(t, inv.Get("A", "asset-1"), 1)
}

func TestConsumptionApplyDoesNotGoNegative(t *testing.T) {
	inv := NewInventoryState()
	inv.Set("A", "asset-1", 0.2)
	consumption := NewConsumptionState()
	consumption.SetRate("A", "asset-1", 1)

	next, volume := consumption.ApplyWithVolume(inv)

	assertApprox(t, next.Get("A", "asset-1"), 0)
	assertApprox(t, volume, 0.2)
}

func TestConsumptionCopyDoesNotMutateOriginal(t *testing.T) {
	consumption := NewConsumptionState()
	consumption.SetRate("A", "asset-1", 0.3)

	copied := consumption.Copy()
	copied.SetRate("A", "asset-1", 0.9)

	assertApprox(t, consumption.Rate("A", "asset-1"), 0.3)
	assertApprox(t, copied.Rate("A", "asset-1"), 0.9)
}

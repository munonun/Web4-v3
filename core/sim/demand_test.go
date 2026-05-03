package sim

import "testing"

func TestDemandTargetNeedSurplus(t *testing.T) {
	inv := NewInventoryState()
	demand := NewDemandState()
	demand.SetTarget("A", "asset-1", 5)

	inv.Set("A", "asset-1", 2)
	assertApprox(t, demand.Target("A", "asset-1"), 5)
	assertApprox(t, demand.Need("A", "asset-1", inv), 3)
	assertApprox(t, demand.Surplus("A", "asset-1", inv), 0)

	inv.Set("A", "asset-1", 7)
	assertApprox(t, demand.Need("A", "asset-1", inv), 0)
	assertApprox(t, demand.Surplus("A", "asset-1", inv), 2)
}

func TestDemandCopyDoesNotMutateOriginal(t *testing.T) {
	demand := NewDemandState()
	demand.SetTarget("A", "asset-1", 5)

	copied := demand.Copy()
	copied.SetTarget("A", "asset-1", 9)

	assertApprox(t, demand.Target("A", "asset-1"), 5)
	assertApprox(t, copied.Target("A", "asset-1"), 9)
}

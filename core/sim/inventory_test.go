package sim

import "testing"

func TestInventorySetGet(t *testing.T) {
	inv := NewInventoryState()
	inv.Set("A", "asset-1", 3)

	assertApprox(t, inv.Get("A", "asset-1"), 3)
	assertApprox(t, inv.Get("A", "missing"), 0)
	assertApprox(t, inv.Get("missing", "asset-1"), 0)
}

func TestInventoryAddPositive(t *testing.T) {
	inv := NewInventoryState()
	inv.Set("A", "asset-1", 3)

	next := inv.Add("A", "asset-1", 2)

	assertApprox(t, next.Get("A", "asset-1"), 5)
	assertApprox(t, inv.Get("A", "asset-1"), 3)
}

func TestInventoryAddNegativeClampsAtZero(t *testing.T) {
	inv := NewInventoryState()
	inv.Set("A", "asset-1", 3)

	next := inv.Add("A", "asset-1", -5)

	assertApprox(t, next.Get("A", "asset-1"), 0)
	assertApprox(t, inv.Get("A", "asset-1"), 3)
}

func TestInventoryCopyDoesNotMutateOriginal(t *testing.T) {
	inv := NewInventoryState()
	inv.Set("A", "asset-1", 3)

	copied := inv.Copy()
	copied.Set("A", "asset-1", 9)

	assertApprox(t, inv.Get("A", "asset-1"), 3)
	assertApprox(t, copied.Get("A", "asset-1"), 9)
}

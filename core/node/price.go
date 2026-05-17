package node

import (
	"bytes"
	"sort"

	"web4-v3/core/model"
	"web4-v3/core/price"
)

func (n *Node) ComputePrice(unit model.UnitID) price.PriceResult {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.init()
	return n.computePriceLocked(unit)
}

func (n *Node) computePriceLocked(unit model.UnitID) price.PriceResult {
	result := price.ComputePrice(
		n.Features[unit],
		n.TradeHistory[unit],
		n.SettledVolume[unit],
		n.LastTradeUnix[unit],
		n.NowUnix(),
		n.PriceConfig,
	)
	n.PriceState[unit] = result
	return result
}

func (n *Node) Price(unit model.UnitID) float64 {
	n.mu.RLock()
	if result, ok := n.PriceState[unit]; ok {
		n.mu.RUnlock()
		return result.FinalPrice
	}
	n.mu.RUnlock()
	n.mu.Lock()
	defer n.mu.Unlock()
	n.init()
	if result, ok := n.PriceState[unit]; ok {
		return result.FinalPrice
	}
	return n.computePriceLocked(unit).FinalPrice
}

func (n *Node) RefreshPrices() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.init()
	for _, unit := range n.unitsLocked() {
		n.computePriceLocked(unit)
	}
}

func (n *Node) effectivePrice(unit model.UnitID) float64 {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.init()
	return n.effectivePriceLocked(unit)
}

func (n *Node) effectivePriceLocked(unit model.UnitID) float64 {
	result, ok := n.PriceState[unit]
	finalPrice := 0.0
	if ok {
		finalPrice = result.FinalPrice
	} else {
		finalPrice = n.computePriceLocked(unit).FinalPrice
	}
	utility := 1.0
	if n.Preferences != nil {
		if u, ok := n.Preferences[unit]; ok {
			utility = u
		}
	}
	if utility <= 0 {
		return 0
	}
	return finalPrice * utility
}

func (n *Node) units() []model.UnitID {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.unitsLocked()
}

func (n *Node) unitsLocked() []model.UnitID {
	seen := make(map[model.UnitID]struct{})
	if n.Inventory.Holdings[n.ID] != nil {
		for unit := range n.Inventory.Holdings[n.ID] {
			seen[unit] = struct{}{}
		}
	}
	for unit := range n.Features {
		seen[unit] = struct{}{}
	}
	for unit := range n.TradeHistory {
		seen[unit] = struct{}{}
	}
	for unit := range n.PriceState {
		seen[unit] = struct{}{}
	}

	units := make([]model.UnitID, 0, len(seen))
	for unit := range seen {
		units = append(units, unit)
	}
	sort.Slice(units, func(i, j int) bool {
		return bytes.Compare(units[i][:], units[j][:]) < 0
	})
	return units
}

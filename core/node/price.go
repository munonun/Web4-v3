package node

import (
	"bytes"
	"sort"

	"web4-v3/core/model"
	"web4-v3/core/price"
)

func (n *Node) ComputePrice(unit model.UnitID) price.PriceResult {
	n.init()
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
	n.init()
	if result, ok := n.PriceState[unit]; ok {
		return result.FinalPrice
	}
	return n.ComputePrice(unit).FinalPrice
}

func (n *Node) RefreshPrices() {
	n.init()
	for _, unit := range n.units() {
		n.ComputePrice(unit)
	}
}

func (n *Node) effectivePrice(unit model.UnitID) float64 {
	price := n.Price(unit)
	utility := 1.0
	if n.Preferences != nil {
		if u, ok := n.Preferences[unit]; ok {
			utility = u
		}
	}
	if utility <= 0 {
		return 0
	}
	return price * utility
}

func (n *Node) units() []model.UnitID {
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

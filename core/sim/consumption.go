package sim

type ConsumptionState struct {
	Rates map[string]map[string]float64
}

func NewConsumptionState() ConsumptionState {
	return ConsumptionState{Rates: map[string]map[string]float64{}}
}

func (c ConsumptionState) SetRate(nodeID, assetID string, rate float64) {
	if c.Rates == nil {
		c.Rates = map[string]map[string]float64{}
	}
	assets := copyFloatMap(c.Rates[nodeID])
	if assets == nil {
		assets = map[string]float64{}
	}
	assets[assetID] = max0(rate)
	c.Rates[nodeID] = assets
}

func (c ConsumptionState) Rate(nodeID, assetID string) float64 {
	assets, ok := c.Rates[nodeID]
	if !ok {
		return 0
	}

	return assets[assetID]
}

func (c ConsumptionState) Apply(inv InventoryState) InventoryState {
	next, _ := c.ApplyWithVolume(inv)
	return next
}

func (c ConsumptionState) ApplyWithVolume(inv InventoryState) (InventoryState, float64) {
	next := inv.Copy()
	consumed := 0.0
	for nodeID, assets := range c.Rates {
		for assetID, rate := range assets {
			qty := minFloat(max0(rate), next.Get(nodeID, assetID))
			if qty == 0 {
				continue
			}
			next = next.Add(nodeID, assetID, -qty)
			consumed += qty
		}
	}

	return next, consumed
}

func (c ConsumptionState) Copy() ConsumptionState {
	copied := NewConsumptionState()
	for nodeID, assets := range c.Rates {
		copied.Rates[nodeID] = copyFloatMap(assets)
	}

	return copied
}

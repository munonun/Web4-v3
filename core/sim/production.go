package sim

// ProductionState models simulation-only production/issuance flows; it does not
// represent protocol issuance, persistence, or ledger behavior.
type ProductionState struct {
	Rates map[string]map[string]float64
}

func NewProductionState() ProductionState {
	return ProductionState{Rates: map[string]map[string]float64{}}
}

func (p ProductionState) SetRate(nodeID, assetID string, rate float64) {
	if p.Rates == nil {
		p.Rates = map[string]map[string]float64{}
	}
	assets := copyFloatMap(p.Rates[nodeID])
	if assets == nil {
		assets = map[string]float64{}
	}
	assets[assetID] = max0(rate)
	p.Rates[nodeID] = assets
}

func (p ProductionState) Rate(nodeID, assetID string) float64 {
	assets, ok := p.Rates[nodeID]
	if !ok {
		return 0
	}

	return assets[assetID]
}

func (p ProductionState) Apply(inv InventoryState) InventoryState {
	next, _ := p.ApplyWithVolume(inv)
	return next
}

func (p ProductionState) ApplyWithVolume(inv InventoryState) (InventoryState, float64) {
	next := inv.Copy()
	produced := 0.0
	for nodeID, assets := range p.Rates {
		for assetID, rate := range assets {
			qty := max0(rate)
			if qty == 0 {
				continue
			}
			next = next.Add(nodeID, assetID, qty)
			produced += qty
		}
	}

	return next, produced
}

func (p ProductionState) Copy() ProductionState {
	copied := NewProductionState()
	for nodeID, assets := range p.Rates {
		copied.Rates[nodeID] = copyFloatMap(assets)
	}

	return copied
}

package sim

type PortfolioPreference struct {
	Weights map[string]map[string]float64
}

func NewPortfolioPreference() PortfolioPreference {
	return PortfolioPreference{Weights: map[string]map[string]float64{}}
}

func (p PortfolioPreference) SetWeight(nodeID, assetID string, weight float64) {
	if p.Weights == nil {
		p.Weights = map[string]map[string]float64{}
	}
	assets := copyFloatMap(p.Weights[nodeID])
	if assets == nil {
		assets = map[string]float64{}
	}
	assets[assetID] = max0(weight)
	p.Weights[nodeID] = assets
}

func (p PortfolioPreference) Weight(nodeID, assetID string) float64 {
	assets, ok := p.Weights[nodeID]
	if !ok {
		return 1
	}
	weight, ok := assets[assetID]
	if !ok {
		return 1
	}
	return weight
}

func (p PortfolioPreference) Copy() PortfolioPreference {
	copied := NewPortfolioPreference()
	for nodeID, assets := range p.Weights {
		copied.Weights[nodeID] = copyFloatMap(assets)
	}
	return copied
}

func EffectiveValue(pt PriceTable, pref PortfolioPreference, nodeID, assetID string) float64 {
	price, ok := pt.Get(nodeID, assetID)
	if !ok {
		return 0
	}
	return price * pref.Weight(nodeID, assetID)
}

func PreferredAsset(pt PriceTable, pref PortfolioPreference, nodeID string, universe AssetUniverse) string {
	bestAsset := ""
	bestValue := -1.0
	for _, assetID := range universe.IDs() {
		value := EffectiveValue(pt, pref, nodeID, assetID)
		if value > bestValue {
			bestValue = value
			bestAsset = assetID
		}
	}
	return bestAsset
}

type ValueDemand struct {
	Targets map[string]float64
}

func NewValueDemand() ValueDemand {
	return ValueDemand{Targets: map[string]float64{}}
}

func (d ValueDemand) SetTarget(nodeID string, target float64) {
	if d.Targets == nil {
		d.Targets = map[string]float64{}
	}
	d.Targets[nodeID] = max0(target)
}

func (d ValueDemand) Target(nodeID string) float64 {
	return d.Targets[nodeID]
}

func (d ValueDemand) HoldingValue(nodeID string, inv InventoryState, pt PriceTable, pref PortfolioPreference, universe AssetUniverse) float64 {
	total := 0.0
	for _, assetID := range universe.IDs() {
		total += inv.Get(nodeID, assetID) * EffectiveValue(pt, pref, nodeID, assetID)
	}
	return total
}

func (d ValueDemand) Need(nodeID string, inv InventoryState, pt PriceTable, pref PortfolioPreference, universe AssetUniverse) float64 {
	return max0(d.Target(nodeID) - d.HoldingValue(nodeID, inv, pt, pref, universe))
}

func (d ValueDemand) Copy() ValueDemand {
	copied := NewValueDemand()
	for nodeID, target := range d.Targets {
		copied.Targets[nodeID] = target
	}
	return copied
}

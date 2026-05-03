package sim

type DemandState struct {
	Targets map[string]map[string]float64
}

func NewDemandState() DemandState {
	return DemandState{Targets: map[string]map[string]float64{}}
}

func (d DemandState) SetTarget(nodeID, assetID string, qty float64) {
	if d.Targets == nil {
		d.Targets = map[string]map[string]float64{}
	}
	assets := copyFloatMap(d.Targets[nodeID])
	if assets == nil {
		assets = map[string]float64{}
	}
	assets[assetID] = max0(qty)
	d.Targets[nodeID] = assets
}

func (d DemandState) Target(nodeID, assetID string) float64 {
	assets, ok := d.Targets[nodeID]
	if !ok {
		return 0
	}

	return assets[assetID]
}

func (d DemandState) Need(nodeID, assetID string, inv InventoryState) float64 {
	return max0(d.Target(nodeID, assetID) - inv.Get(nodeID, assetID))
}

func (d DemandState) Surplus(nodeID, assetID string, inv InventoryState) float64 {
	return max0(inv.Get(nodeID, assetID) - d.Target(nodeID, assetID))
}

func (d DemandState) Copy() DemandState {
	copied := NewDemandState()
	for nodeID, assets := range d.Targets {
		copied.Targets[nodeID] = copyFloatMap(assets)
	}

	return copied
}

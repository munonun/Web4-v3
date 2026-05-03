package sim

type InventoryState struct {
	Holdings map[string]map[string]float64
}

func NewInventoryState() InventoryState {
	return InventoryState{Holdings: map[string]map[string]float64{}}
}

func (s InventoryState) Set(nodeID, assetID string, qty float64) {
	if s.Holdings == nil {
		s.Holdings = map[string]map[string]float64{}
	}
	assets := copyFloatMap(s.Holdings[nodeID])
	if assets == nil {
		assets = map[string]float64{}
	}
	assets[assetID] = max0(qty)
	s.Holdings[nodeID] = assets
}

func (s InventoryState) Get(nodeID, assetID string) float64 {
	assets, ok := s.Holdings[nodeID]
	if !ok {
		return 0
	}

	return assets[assetID]
}

// Add returns a copied state with delta applied. Holdings are clamped at zero.
func (s InventoryState) Add(nodeID, assetID string, delta float64) InventoryState {
	next := s.Copy()
	next.Set(nodeID, assetID, next.Get(nodeID, assetID)+delta)
	return next
}

func (s InventoryState) Copy() InventoryState {
	copied := NewInventoryState()
	for nodeID, assets := range s.Holdings {
		copied.Holdings[nodeID] = copyFloatMap(assets)
	}

	return copied
}

func copyFloatMap(values map[string]float64) map[string]float64 {
	if values == nil {
		return nil
	}

	copied := make(map[string]float64, len(values))
	for key, value := range values {
		copied[key] = value
	}

	return copied
}

func max0(v float64) float64 {
	if v < 0 {
		return 0
	}

	return v
}

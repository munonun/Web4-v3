package sim

type AssetFlowMetrics struct {
	TradeVolume       map[string]float64 `json:"trade_flow"`
	PaymentVolume     map[string]float64 `json:"payment_flow"`
	ConsumptionVolume map[string]float64 `json:"consumption_flow"`
	DemandFulfilled   map[string]float64 `json:"demand_fulfilled"`
}

func NewAssetFlowMetrics(assetIDs []string) AssetFlowMetrics {
	metrics := AssetFlowMetrics{
		TradeVolume:       map[string]float64{},
		PaymentVolume:     map[string]float64{},
		ConsumptionVolume: map[string]float64{},
		DemandFulfilled:   map[string]float64{},
	}
	for _, assetID := range sortedUnique(assetIDs) {
		metrics.TradeVolume[assetID] = 0
		metrics.PaymentVolume[assetID] = 0
		metrics.ConsumptionVolume[assetID] = 0
		metrics.DemandFulfilled[assetID] = 0
	}

	return metrics
}

// AddTrade mutates the receiver and ignores negative quantities.
func (m AssetFlowMetrics) AddTrade(assetID string, qty float64) {
	addFlow(m.TradeVolume, assetID, qty)
}

// AddPayment mutates the receiver and ignores negative quantities.
func (m AssetFlowMetrics) AddPayment(assetID string, qty float64) {
	addFlow(m.PaymentVolume, assetID, qty)
}

// AddConsumption mutates the receiver and ignores negative quantities.
func (m AssetFlowMetrics) AddConsumption(assetID string, qty float64) {
	addFlow(m.ConsumptionVolume, assetID, qty)
}

// AddDemandFulfilled mutates the receiver and ignores negative quantities.
func (m AssetFlowMetrics) AddDemandFulfilled(assetID string, qty float64) {
	addFlow(m.DemandFulfilled, assetID, qty)
}

func (m AssetFlowMetrics) Copy() AssetFlowMetrics {
	return AssetFlowMetrics{
		TradeVolume:       copyFloatMap(m.TradeVolume),
		PaymentVolume:     copyFloatMap(m.PaymentVolume),
		ConsumptionVolume: copyFloatMap(m.ConsumptionVolume),
		DemandFulfilled:   copyFloatMap(m.DemandFulfilled),
	}
}

func FlowShare(volumes map[string]float64) map[string]float64 {
	shares := map[string]float64{}
	total := sumVolumes(volumes)
	for _, assetID := range sortedVolumeIDs(volumes) {
		if total == 0 {
			shares[assetID] = 0
		} else {
			shares[assetID] = volumes[assetID] / total
		}
	}

	return shares
}

func DominantByFlow(volumes map[string]float64) string {
	if sumVolumes(volumes) == 0 {
		return ""
	}
	bestAsset := ""
	bestVolume := -1.0
	for _, assetID := range sortedVolumeIDs(volumes) {
		volume := volumes[assetID]
		if volume > bestVolume {
			bestAsset = assetID
			bestVolume = volume
		}
	}

	return bestAsset
}

func FlowConcentration(volumes map[string]float64) float64 {
	shares := FlowShare(volumes)
	maxShare := 0.0
	for _, assetID := range sortedVolumeIDs(shares) {
		if shares[assetID] > maxShare {
			maxShare = shares[assetID]
		}
	}

	return maxShare
}

func ApplyConsumptionWithFlow(consumption ConsumptionState, inv InventoryState, universe AssetUniverse) (InventoryState, AssetFlowMetrics) {
	next := inv.Copy()
	flows := NewAssetFlowMetrics(universe.IDs())
	for nodeID, assets := range consumption.Rates {
		for _, assetID := range universe.IDs() {
			qty := minFloat(max0(assets[assetID]), next.Get(nodeID, assetID))
			if qty == 0 {
				continue
			}
			next = next.Add(nodeID, assetID, -qty)
			flows.AddConsumption(assetID, qty)
		}
	}

	return next, flows
}

func addAssetFlows(dst AssetFlowMetrics, src AssetFlowMetrics) {
	for assetID, qty := range src.TradeVolume {
		dst.AddTrade(assetID, qty)
	}
	for assetID, qty := range src.PaymentVolume {
		dst.AddPayment(assetID, qty)
	}
	for assetID, qty := range src.ConsumptionVolume {
		dst.AddConsumption(assetID, qty)
	}
	for assetID, qty := range src.DemandFulfilled {
		dst.AddDemandFulfilled(assetID, qty)
	}
}

func totalFlowVolume(volumes map[string]float64) float64 {
	return sumVolumes(volumes)
}

func addFlow(volumes map[string]float64, assetID string, qty float64) {
	if qty <= 0 {
		return
	}
	if volumes == nil {
		return
	}
	volumes[assetID] += qty
}

func sumVolumes(volumes map[string]float64) float64 {
	total := 0.0
	for _, assetID := range sortedVolumeIDs(volumes) {
		total += volumes[assetID]
	}
	return total
}

func sortedVolumeIDs(volumes map[string]float64) []string {
	ids := make([]string, 0, len(volumes))
	for assetID := range volumes {
		ids = append(ids, assetID)
	}
	return sortedUnique(ids)
}

package sim

func PriceFromAcceptance(a float64) float64 {
	return clamp01(a)
}

func RiskFromAcceptance(a float64) float64 {
	return 1 - clamp01(a)
}

type PriceTable struct {
	Prices map[string]map[string]float64
}

func NewPriceTable() PriceTable {
	return PriceTable{Prices: map[string]map[string]float64{}}
}

func (pt PriceTable) Set(nodeID, assetID string, price float64) {
	if pt.Prices == nil {
		pt.Prices = map[string]map[string]float64{}
	}

	assets := copyPriceMap(pt.Prices[nodeID])
	if assets == nil {
		assets = map[string]float64{}
	}
	assets[assetID] = price
	pt.Prices[nodeID] = assets
}

func (pt PriceTable) Get(nodeID, assetID string) (float64, bool) {
	assets, ok := pt.Prices[nodeID]
	if !ok {
		return 0, false
	}

	price, ok := assets[assetID]
	return price, ok
}

func PriceTableFromAcceptance(assetID string, state AcceptanceState) PriceTable {
	pt := NewPriceTable()
	for nodeID, score := range state.Scores {
		pt.Set(nodeID, assetID, PriceFromAcceptance(score))
	}

	return pt
}

func copyPriceMap(prices map[string]float64) map[string]float64 {
	if prices == nil {
		return nil
	}

	copied := make(map[string]float64, len(prices))
	for assetID, price := range prices {
		copied[assetID] = price
	}

	return copied
}

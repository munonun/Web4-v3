package sim

import "fmt"

type AssetUniverse struct {
	AssetIDs []string
}

func NewAssetUniverse(assetIDs []string) AssetUniverse {
	return AssetUniverse{AssetIDs: sortedUnique(assetIDs)}
}

func (u AssetUniverse) IDs() []string {
	return append([]string(nil), u.AssetIDs...)
}

func (u AssetUniverse) Validate() error {
	if len(u.AssetIDs) == 0 {
		return fmt.Errorf("asset universe must not be empty")
	}

	return nil
}

type MultiAcceptanceState struct {
	Scores map[string]map[string]float64
}

func NewMultiAcceptanceState() MultiAcceptanceState {
	return MultiAcceptanceState{Scores: map[string]map[string]float64{}}
}

func (s MultiAcceptanceState) Set(nodeID, assetID string, score float64) {
	if s.Scores == nil {
		s.Scores = map[string]map[string]float64{}
	}
	assets := copyFloatMap(s.Scores[nodeID])
	if assets == nil {
		assets = map[string]float64{}
	}
	assets[assetID] = clamp01(score)
	s.Scores[nodeID] = assets
}

func (s MultiAcceptanceState) Get(nodeID, assetID string) float64 {
	assets, ok := s.Scores[nodeID]
	if !ok {
		return 0
	}

	return assets[assetID]
}

func (s MultiAcceptanceState) Copy() MultiAcceptanceState {
	copied := NewMultiAcceptanceState()
	for nodeID, assets := range s.Scores {
		copied.Scores[nodeID] = copyFloatMap(assets)
	}

	return copied
}

func (s MultiAcceptanceState) AssetState(assetID string) AcceptanceState {
	state := AcceptanceState{Scores: map[string]float64{}}
	for nodeID, assets := range s.Scores {
		state.Scores[nodeID] = clamp01(assets[assetID])
	}

	return state
}

func (s MultiAcceptanceState) SetAssetState(assetID string, state AcceptanceState) MultiAcceptanceState {
	next := s.Copy()
	for nodeID, score := range state.Scores {
		next.Set(nodeID, assetID, score)
	}

	return next
}

func PriceTableFromMultiAcceptance(state MultiAcceptanceState) PriceTable {
	pt := NewPriceTable()
	for nodeID, assets := range state.Scores {
		for assetID, score := range assets {
			pt.Set(nodeID, assetID, PriceFromAcceptance(score))
		}
	}

	return pt
}

func multiNodeIDs(state MultiAcceptanceState) []string {
	ids := make([]string, 0, len(state.Scores))
	for nodeID := range state.Scores {
		ids = append(ids, nodeID)
	}

	return sortedUnique(ids)
}

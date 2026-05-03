package sim

import "sort"

type ArbitrageOpportunity struct {
	BuyFrom   string
	SellTo    string
	AssetID   string
	BuyPrice  float64
	SellPrice float64
	Profit    float64
}

func FindArbitrage(pt PriceTable, assetID string, minProfit float64) []ArbitrageOpportunity {
	if minProfit < 0 {
		minProfit = 0
	}

	nodeIDs := priceNodeIDs(pt)
	opportunities := []ArbitrageOpportunity{}
	for _, buyFrom := range nodeIDs {
		buyPrice, ok := pt.Get(buyFrom, assetID)
		if !ok {
			continue
		}
		for _, sellTo := range nodeIDs {
			if buyFrom == sellTo {
				continue
			}
			sellPrice, ok := pt.Get(sellTo, assetID)
			if !ok {
				continue
			}
			profit := sellPrice - buyPrice
			if profit >= minProfit {
				opportunities = append(opportunities, ArbitrageOpportunity{
					BuyFrom:   buyFrom,
					SellTo:    sellTo,
					AssetID:   assetID,
					BuyPrice:  buyPrice,
					SellPrice: sellPrice,
					Profit:    profit,
				})
			}
		}
	}

	sort.Slice(opportunities, func(i, j int) bool {
		if opportunities[i].Profit != opportunities[j].Profit {
			return opportunities[i].Profit > opportunities[j].Profit
		}
		if opportunities[i].BuyFrom != opportunities[j].BuyFrom {
			return opportunities[i].BuyFrom < opportunities[j].BuyFrom
		}
		return opportunities[i].SellTo < opportunities[j].SellTo
	})

	return opportunities
}

func priceNodeIDs(pt PriceTable) []string {
	ids := make([]string, 0, len(pt.Prices))
	for nodeID := range pt.Prices {
		ids = append(ids, nodeID)
	}

	return sortedUnique(ids)
}

package node

import (
	"web4-v3/core/model"
	"web4-v3/core/price"
)

func (n *Node) recordTradeFlow(unit model.UnitID, amount model.Amount) {
	n.init()
	record := n.Flow[unit]
	record.Unit = unit
	record.AddTrade(amount)
	n.Flow[unit] = record
}

func (n *Node) recordPaymentFlow(unit model.UnitID, amount model.Amount) {
	n.init()
	record := n.Flow[unit]
	record.Unit = unit
	record.AddPayment(amount)
	n.Flow[unit] = record
}

func (n *Node) recordObservation(unit model.UnitID, executedPrice float64, volume model.Amount, now int64) {
	n.init()
	if executedPrice < 0 || volume <= 0 {
		return
	}
	weight := 1.0
	if result, ok := n.PriceState[unit]; ok && result.FeatureScore > 0 {
		weight = result.FeatureScore
	}
	n.TradeHistory[unit] = append(n.TradeHistory[unit], price.TradeObservation{
		Price:    executedPrice,
		Volume:   volume,
		Weight:   weight,
		TimeUnix: now,
	})
	n.SettledVolume[unit] = model.Add(n.SettledVolume[unit], volume)
	n.LastTradeUnix[unit] = now
	n.ComputePrice(unit)
}

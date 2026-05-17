package node

import (
	"web4-v3/core/model"
	"web4-v3/core/price"
)

func (n *Node) recordTradeFlow(unit model.UnitID, amount model.Amount) error {
	n.init()
	record := n.Flow[unit]
	record.Unit = unit
	if err := record.AddTradeChecked(amount); err != nil {
		return err
	}
	n.Flow[unit] = record
	return nil
}

func (n *Node) recordPaymentFlow(unit model.UnitID, amount model.Amount) error {
	n.init()
	record := n.Flow[unit]
	record.Unit = unit
	if err := record.AddPaymentChecked(amount); err != nil {
		return err
	}
	n.Flow[unit] = record
	return nil
}

func (n *Node) recordObservation(unit model.UnitID, executedPrice float64, volume model.Amount, now int64) error {
	n.init()
	if executedPrice < 0 || volume <= 0 {
		return nil
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
	next, err := model.CheckedAdd(n.SettledVolume[unit], volume)
	if err != nil {
		return err
	}
	n.SettledVolume[unit] = next
	n.LastTradeUnix[unit] = now
	n.computePriceLocked(unit)
	return nil
}

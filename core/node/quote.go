package node

import (
	"fmt"
	"math"

	"web4-v3/core/model"
)

type Quote struct {
	Seller model.NodeID
	Buyer  model.NodeID

	SellUnit model.UnitID
	BuyUnit  model.UnitID

	SellAmount model.Amount
	BuyAmount  model.Amount

	SellerAsk float64
	BuyerBid  float64

	Executable bool
	Reason     string
	Timestamp  int64
}

func (n *Node) QuoteSell(
	buyer *Node,
	sellUnit model.UnitID,
	buyUnit model.UnitID,
	sellAmount model.Amount,
	spread float64,
) Quote {
	n.init()
	if buyer != nil {
		buyer.init()
	}
	q := Quote{
		Seller:     n.ID,
		SellUnit:   sellUnit,
		BuyUnit:    buyUnit,
		SellAmount: sellAmount,
		Timestamp:  n.NowUnix(),
	}
	if buyer == nil {
		q.Reason = "buyer is required"
		return q
	}
	q.Buyer = buyer.ID
	if n.ID == buyer.ID {
		q.Reason = "seller and buyer must differ"
		return q
	}
	if sellAmount <= 0 {
		q.Reason = "sell amount must be greater than zero"
		return q
	}
	if n.Balance(sellUnit) < sellAmount {
		q.Reason = "seller has insufficient inventory"
		return q
	}
	sellerSellPrice := n.effectivePrice(sellUnit)
	sellerBuyPrice := n.effectivePrice(buyUnit)
	buyerSellPrice := buyer.effectivePrice(sellUnit)
	buyerBuyPrice := buyer.effectivePrice(buyUnit)
	if sellerSellPrice <= 0 || sellerBuyPrice <= 0 {
		q.Reason = "seller has no usable local price"
		return q
	}
	if buyerSellPrice <= 0 || buyerBuyPrice <= 0 {
		q.Reason = "buyer has no usable local price"
		return q
	}
	if spread < 0 {
		spread = 0
	}

	sellQty := model.ToFloat(sellAmount)
	q.SellerAsk = sellQty * (sellerSellPrice / sellerBuyPrice) * (1 + spread)
	q.BuyerBid = sellQty * (buyerSellPrice / buyerBuyPrice)
	if q.BuyerBid < q.SellerAsk {
		q.Reason = "buyer valuation below seller ask"
		return q
	}
	buyAmount := model.FromFloat((q.SellerAsk + q.BuyerBid) / 2)
	if buyAmount <= 0 {
		q.Reason = "computed buy amount is zero"
		return q
	}
	q.BuyAmount = buyAmount
	if buyer.Balance(buyUnit) < buyAmount {
		q.Reason = "buyer has insufficient payment inventory"
		return q
	}

	q.Executable = true
	q.Reason = "executable"
	return q
}

func (n *Node) AcceptQuote(q Quote) bool {
	n.init()
	if !q.Executable {
		return false
	}
	switch n.ID {
	case q.Seller:
		if n.Balance(q.SellUnit) < q.SellAmount {
			return false
		}
		sellPrice := n.effectivePrice(q.SellUnit)
		buyPrice := n.effectivePrice(q.BuyUnit)
		if sellPrice <= 0 || buyPrice <= 0 {
			return false
		}
		return model.ToFloat(q.BuyAmount)+roundingSlack() >= model.ToFloat(q.SellAmount)*(sellPrice/buyPrice)
	case q.Buyer:
		if n.Balance(q.BuyUnit) < q.BuyAmount {
			return false
		}
		sellPrice := n.effectivePrice(q.SellUnit)
		buyPrice := n.effectivePrice(q.BuyUnit)
		if sellPrice <= 0 || buyPrice <= 0 {
			return false
		}
		return model.ToFloat(q.BuyAmount) <= model.ToFloat(q.SellAmount)*(sellPrice/buyPrice)+roundingSlack()
	default:
		return false
	}
}

func roundingSlack() float64 {
	return 1.0 / float64(model.AmountScale)
}

func quoteExecutionError(q Quote) error {
	if q.Reason != "" {
		return fmt.Errorf("%s", q.Reason)
	}
	if math.IsNaN(q.SellerAsk) || math.IsNaN(q.BuyerBid) {
		return fmt.Errorf("quote contains invalid price")
	}
	return fmt.Errorf("quote is not executable")
}

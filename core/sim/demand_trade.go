package sim

type DemandTradeQuote struct {
	Seller        string
	Buyer         string
	AssetID       string
	Quantity      float64
	SellerSurplus float64
	BuyerNeed     float64
	SellerAsk     float64
	BuyerBid      float64
	Executable    bool
	ClearingPrice float64
}

func QuoteDemandTrade(
	pt PriceTable,
	inv InventoryState,
	demand DemandState,
	sellerID string,
	buyerID string,
	assetID string,
	maxQty float64,
	spread float64,
) DemandTradeQuote {
	if spread < 0 {
		spread = 0
	}

	quote := DemandTradeQuote{Seller: sellerID, Buyer: buyerID, AssetID: assetID}
	if maxQty <= 0 {
		return quote
	}

	quote.SellerSurplus = demand.Surplus(sellerID, assetID, inv)
	quote.BuyerNeed = demand.Need(buyerID, assetID, inv)
	quote.Quantity = minFloat(quote.SellerSurplus, quote.BuyerNeed, maxQty)
	ask, ok := pt.Get(sellerID, assetID)
	if !ok {
		return quote
	}
	bid, ok := pt.Get(buyerID, assetID)
	if !ok {
		quote.SellerAsk = ask
		return quote
	}

	quote.SellerAsk = ask
	quote.BuyerBid = bid
	quote.Executable = quote.Quantity > 0 && bid >= ask*(1+spread)
	if quote.Executable {
		quote.ClearingPrice = (ask + bid) / 2
	}

	return quote
}

func ApplyDemandTrade(
	inv InventoryState,
	state AcceptanceState,
	quote DemandTradeQuote,
	alpha float64,
) (InventoryState, AcceptanceState) {
	nextInv := inv.Copy()
	nextState := copyState(state)
	if !quote.Executable {
		return nextInv, nextState
	}

	qty := max0(quote.Quantity)
	if qty == 0 {
		return nextInv, nextState
	}

	nextInv = nextInv.Add(quote.Seller, quote.AssetID, -qty)
	nextInv = nextInv.Add(quote.Buyer, quote.AssetID, qty)
	nextState = ApplyTradeFeedback(nextState, TradeQuote{
		Seller:        quote.Seller,
		Buyer:         quote.Buyer,
		AssetID:       quote.AssetID,
		Executable:    true,
		ClearingPrice: quote.ClearingPrice,
	}, alpha)

	return nextInv, nextState
}

func minFloat(values ...float64) float64 {
	if len(values) == 0 {
		return 0
	}

	min := values[0]
	for _, value := range values[1:] {
		if value < min {
			min = value
		}
	}

	return min
}

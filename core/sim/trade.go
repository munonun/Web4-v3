package sim

type TradeQuote struct {
	Seller        string
	Buyer         string
	AssetID       string
	SellerAsk     float64
	BuyerBid      float64
	Executable    bool
	ClearingPrice float64
}

func QuoteTrade(pt PriceTable, sellerID, buyerID, assetID string, spread float64) TradeQuote {
	if spread < 0 {
		spread = 0
	}

	quote := TradeQuote{Seller: sellerID, Buyer: buyerID, AssetID: assetID}
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
	quote.Executable = bid >= ask*(1+spread)
	if quote.Executable {
		quote.ClearingPrice = (ask + bid) / 2
	}

	return quote
}

func ApplyTradeFeedback(state AcceptanceState, quote TradeQuote, alpha float64) AcceptanceState {
	next := copyState(state)
	if !quote.Executable {
		return next
	}
	if alpha < 0 {
		alpha = 0
	}
	if alpha > 1 {
		alpha = 1
	}

	if sellerScore, ok := state.Scores[quote.Seller]; ok {
		next.Scores[quote.Seller] = clamp01(sellerScore + alpha*(quote.ClearingPrice-sellerScore))
	}
	if buyerScore, ok := state.Scores[quote.Buyer]; ok {
		next.Scores[quote.Buyer] = clamp01(buyerScore + alpha*(quote.ClearingPrice-buyerScore))
	}

	return next
}

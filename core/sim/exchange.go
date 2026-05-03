package sim

type ExchangeQuote struct {
	NodeID     string
	BaseAsset  string
	QuoteAsset string
	BasePrice  float64
	QuotePrice float64
	Rate       float64
	Valid      bool
}

func QuoteExchange(pt PriceTable, nodeID, baseAsset, quoteAsset string) ExchangeQuote {
	quote := ExchangeQuote{NodeID: nodeID, BaseAsset: baseAsset, QuoteAsset: quoteAsset}
	basePrice, ok := pt.Get(nodeID, baseAsset)
	if !ok {
		return quote
	}
	quotePrice, ok := pt.Get(nodeID, quoteAsset)
	if !ok {
		quote.BasePrice = basePrice
		return quote
	}
	quote.BasePrice = basePrice
	quote.QuotePrice = quotePrice
	if quotePrice <= 0 {
		return quote
	}

	quote.Rate = basePrice / quotePrice
	quote.Valid = true
	return quote
}

type CrossAssetTradeQuote struct {
	Seller          string
	Buyer           string
	SellAsset       string
	BuyAsset        string
	SellQty         float64
	BuyQty          float64
	SellerSellPrice float64
	SellerBuyPrice  float64
	BuyerSellPrice  float64
	BuyerBuyPrice   float64
	Executable      bool
}

func QuoteCrossAssetTrade(
	pt PriceTable,
	inv InventoryState,
	sellerID string,
	buyerID string,
	sellAsset string,
	buyAsset string,
	sellQty float64,
	spread float64,
) CrossAssetTradeQuote {
	if spread < 0 {
		spread = 0
	}
	quote := CrossAssetTradeQuote{Seller: sellerID, Buyer: buyerID, SellAsset: sellAsset, BuyAsset: buyAsset}
	if sellAsset == buyAsset || sellQty <= 0 || inv.Get(sellerID, sellAsset) < sellQty {
		return quote
	}

	sellerSellPrice, ok := pt.Get(sellerID, sellAsset)
	if !ok || sellerSellPrice <= 0 {
		return quote
	}
	sellerBuyPrice, ok := pt.Get(sellerID, buyAsset)
	if !ok || sellerBuyPrice <= 0 {
		quote.SellerSellPrice = sellerSellPrice
		return quote
	}
	buyerSellPrice, ok := pt.Get(buyerID, sellAsset)
	if !ok || buyerSellPrice <= 0 {
		quote.SellerSellPrice = sellerSellPrice
		quote.SellerBuyPrice = sellerBuyPrice
		return quote
	}
	buyerBuyPrice, ok := pt.Get(buyerID, buyAsset)
	if !ok || buyerBuyPrice <= 0 {
		quote.SellerSellPrice = sellerSellPrice
		quote.SellerBuyPrice = sellerBuyPrice
		quote.BuyerSellPrice = buyerSellPrice
		return quote
	}

	quote.SellQty = sellQty
	quote.SellerSellPrice = sellerSellPrice
	quote.SellerBuyPrice = sellerBuyPrice
	quote.BuyerSellPrice = buyerSellPrice
	quote.BuyerBuyPrice = buyerBuyPrice

	sellerRequiredBuyQty := sellQty * (sellerSellPrice / sellerBuyPrice) * (1 + spread)
	buyerMaxBuyQty := sellQty * (buyerSellPrice / buyerBuyPrice)
	if buyerMaxBuyQty < sellerRequiredBuyQty {
		return quote
	}
	buyQty := (sellerRequiredBuyQty + buyerMaxBuyQty) / 2
	if inv.Get(buyerID, buyAsset) < buyQty {
		return quote
	}

	quote.BuyQty = buyQty
	quote.Executable = true
	return quote
}

func ApplyCrossAssetTrade(
	inv InventoryState,
	state MultiAcceptanceState,
	quote CrossAssetTradeQuote,
	alpha float64,
) (InventoryState, MultiAcceptanceState) {
	nextInv := inv.Copy()
	nextState := state.Copy()
	if !quote.Executable {
		return nextInv, nextState
	}

	nextInv = nextInv.Add(quote.Seller, quote.SellAsset, -quote.SellQty)
	nextInv = nextInv.Add(quote.Buyer, quote.SellAsset, quote.SellQty)
	nextInv = nextInv.Add(quote.Buyer, quote.BuyAsset, -quote.BuyQty)
	nextInv = nextInv.Add(quote.Seller, quote.BuyAsset, quote.BuyQty)

	alpha = clamp01(alpha)
	sellClearing := (quote.SellerSellPrice + quote.BuyerSellPrice) / 2
	buyClearing := (quote.SellerBuyPrice + quote.BuyerBuyPrice) / 2
	for _, nodeID := range []string{quote.Seller, quote.Buyer} {
		sellScore := state.Get(nodeID, quote.SellAsset)
		buyScore := state.Get(nodeID, quote.BuyAsset)
		nextState.Set(nodeID, quote.SellAsset, sellScore+alpha*(sellClearing-sellScore))
		nextState.Set(nodeID, quote.BuyAsset, buyScore+alpha*(buyClearing-buyScore))
	}

	return nextInv, nextState
}

func QuoteSubstitutionSwitch(
	pt PriceTable,
	inv InventoryState,
	sellerID string,
	buyerID string,
	sellAsset string,
	buyAsset string,
	sellQty float64,
	spread float64,
) CrossAssetTradeQuote {
	if spread < 0 {
		spread = 0
	}
	quote := CrossAssetTradeQuote{Seller: sellerID, Buyer: buyerID, SellAsset: sellAsset, BuyAsset: buyAsset}
	if sellAsset == buyAsset || sellQty <= 0 || inv.Get(sellerID, sellAsset) < sellQty {
		return quote
	}
	sellerSellPrice, ok := pt.Get(sellerID, sellAsset)
	if !ok || sellerSellPrice <= 0 {
		return quote
	}
	sellerBuyPrice, ok := pt.Get(sellerID, buyAsset)
	if !ok || sellerBuyPrice <= 0 {
		return quote
	}
	buyerSellPrice, ok := pt.Get(buyerID, sellAsset)
	if !ok || buyerSellPrice <= 0 {
		return quote
	}
	buyerBuyPrice, ok := pt.Get(buyerID, buyAsset)
	if !ok || buyerBuyPrice <= 0 {
		return quote
	}
	buyQty := sellQty * (sellerSellPrice / sellerBuyPrice) * (1 + spread)
	if inv.Get(buyerID, buyAsset) < buyQty {
		return quote
	}
	quote.SellQty = sellQty
	quote.BuyQty = buyQty
	quote.SellerSellPrice = sellerSellPrice
	quote.SellerBuyPrice = sellerBuyPrice
	quote.BuyerSellPrice = buyerSellPrice
	quote.BuyerBuyPrice = buyerBuyPrice
	quote.Executable = true
	return quote
}

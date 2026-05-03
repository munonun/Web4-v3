package sim

import "testing"

func TestQuoteTradeExecutable(t *testing.T) {
	pt := NewPriceTable()
	pt.Set("seller", "asset-1", 0.4)
	pt.Set("buyer", "asset-1", 0.8)

	quote := QuoteTrade(pt, "seller", "buyer", "asset-1", 0)

	if !quote.Executable {
		t.Fatal("expected executable quote")
	}
	assertApprox(t, quote.SellerAsk, 0.4)
	assertApprox(t, quote.BuyerBid, 0.8)
	assertApprox(t, quote.ClearingPrice, 0.6)
}

func TestQuoteTradeNotExecutable(t *testing.T) {
	pt := NewPriceTable()
	pt.Set("seller", "asset-1", 0.8)
	pt.Set("buyer", "asset-1", 0.4)

	quote := QuoteTrade(pt, "seller", "buyer", "asset-1", 0)

	if quote.Executable {
		t.Fatal("expected non-executable quote")
	}
	assertApprox(t, quote.ClearingPrice, 0)
}

func TestQuoteTradeSpreadPreventsMarginalTrade(t *testing.T) {
	pt := NewPriceTable()
	pt.Set("seller", "asset-1", 1.0)
	pt.Set("buyer", "asset-1", 1.05)

	quote := QuoteTrade(pt, "seller", "buyer", "asset-1", 0.1)

	if quote.Executable {
		t.Fatal("expected spread to prevent marginal trade")
	}
}

func TestQuoteTradeMissingPrice(t *testing.T) {
	pt := NewPriceTable()
	pt.Set("seller", "asset-1", 0.4)

	quote := QuoteTrade(pt, "seller", "buyer", "asset-1", 0)

	if quote.Executable {
		t.Fatal("expected missing buyer price to be non-executable")
	}
	assertApprox(t, quote.SellerAsk, 0.4)
	assertApprox(t, quote.BuyerBid, 0)
}

func TestQuoteTradeNegativeSpreadUsesZero(t *testing.T) {
	pt := NewPriceTable()
	pt.Set("seller", "asset-1", 0.5)
	pt.Set("buyer", "asset-1", 0.5)

	quote := QuoteTrade(pt, "seller", "buyer", "asset-1", -0.1)

	if !quote.Executable {
		t.Fatal("expected negative spread to behave like zero")
	}
}

func TestApplyTradeFeedbackUpdatesParticipantsOnly(t *testing.T) {
	state := AcceptanceState{Scores: map[string]float64{"seller": 0.2, "buyer": 0.8, "other": 0.1}}
	quote := TradeQuote{
		Seller:        "seller",
		Buyer:         "buyer",
		AssetID:       "asset-1",
		Executable:    true,
		ClearingPrice: 0.6,
	}

	next := ApplyTradeFeedback(state, quote, 0.5)

	assertApprox(t, next.Scores["seller"], 0.4)
	assertApprox(t, next.Scores["buyer"], 0.7)
	assertApprox(t, next.Scores["other"], 0.1)
	assertApprox(t, state.Scores["seller"], 0.2)
	assertApprox(t, state.Scores["buyer"], 0.8)
}

func TestApplyTradeFeedbackNonExecutableCopiesUnchanged(t *testing.T) {
	state := AcceptanceState{Scores: map[string]float64{"seller": 0.2, "buyer": 0.8}}
	quote := TradeQuote{Seller: "seller", Buyer: "buyer", AssetID: "asset-1"}

	next := ApplyTradeFeedback(state, quote, 0.5)

	assertApprox(t, next.Scores["seller"], 0.2)
	assertApprox(t, next.Scores["buyer"], 0.8)
	next.Scores["seller"] = 1
	assertApprox(t, state.Scores["seller"], 0.2)
}

func TestApplyTradeFeedbackClampsScores(t *testing.T) {
	state := AcceptanceState{Scores: map[string]float64{"seller": 0.9, "buyer": 0.1}}
	quote := TradeQuote{
		Seller:        "seller",
		Buyer:         "buyer",
		Executable:    true,
		ClearingPrice: 1.2,
	}

	next := ApplyTradeFeedback(state, quote, 1)

	assertApprox(t, next.Scores["seller"], 1)
	assertApprox(t, next.Scores["buyer"], 1)
}

package sim

import "testing"

func demandTradeFixture() (PriceTable, InventoryState, DemandState) {
	pt := NewPriceTable()
	pt.Set("seller", "asset-1", 0.3)
	pt.Set("buyer", "asset-1", 0.8)

	inv := NewInventoryState()
	inv.Set("seller", "asset-1", 10)
	inv.Set("buyer", "asset-1", 0)

	demand := NewDemandState()
	demand.SetTarget("seller", "asset-1", 2)
	demand.SetTarget("buyer", "asset-1", 4)

	return pt, inv, demand
}

func TestQuoteDemandTradeExecutable(t *testing.T) {
	pt, inv, demand := demandTradeFixture()

	quote := QuoteDemandTrade(pt, inv, demand, "seller", "buyer", "asset-1", 1.5, 0.01)

	if !quote.Executable {
		t.Fatal("expected executable demand trade")
	}
	assertApprox(t, quote.SellerSurplus, 8)
	assertApprox(t, quote.BuyerNeed, 4)
	assertApprox(t, quote.Quantity, 1.5)
	assertApprox(t, quote.ClearingPrice, 0.55)
}

func TestQuoteDemandTradeRequiresSurplus(t *testing.T) {
	pt, inv, demand := demandTradeFixture()
	inv.Set("seller", "asset-1", 2)

	quote := QuoteDemandTrade(pt, inv, demand, "seller", "buyer", "asset-1", 1, 0)

	if quote.Executable {
		t.Fatal("expected non-executable trade without surplus")
	}
	assertApprox(t, quote.Quantity, 0)
}

func TestQuoteDemandTradeRequiresNeed(t *testing.T) {
	pt, inv, demand := demandTradeFixture()
	inv.Set("buyer", "asset-1", 4)

	quote := QuoteDemandTrade(pt, inv, demand, "seller", "buyer", "asset-1", 1, 0)

	if quote.Executable {
		t.Fatal("expected non-executable trade without need")
	}
	assertApprox(t, quote.Quantity, 0)
}

func TestQuoteDemandTradeRequiresPriceOverlap(t *testing.T) {
	pt, inv, demand := demandTradeFixture()
	pt.Set("buyer", "asset-1", 0.2)

	quote := QuoteDemandTrade(pt, inv, demand, "seller", "buyer", "asset-1", 1, 0)

	if quote.Executable {
		t.Fatal("expected non-executable trade with low buyer bid")
	}
}

func TestApplyDemandTradeTransfersInventoryAndFeedback(t *testing.T) {
	pt, inv, demand := demandTradeFixture()
	state := AcceptanceState{Scores: map[string]float64{"seller": 0.3, "buyer": 0.8, "other": 0.1}}
	quote := QuoteDemandTrade(pt, inv, demand, "seller", "buyer", "asset-1", 2, 0)

	nextInv, nextState := ApplyDemandTrade(inv, state, quote, 0.5)

	assertApprox(t, nextInv.Get("seller", "asset-1"), 8)
	assertApprox(t, nextInv.Get("buyer", "asset-1"), 2)
	assertApprox(t, nextState.Scores["seller"], 0.425)
	assertApprox(t, nextState.Scores["buyer"], 0.675)
	assertApprox(t, nextState.Scores["other"], 0.1)
	assertApprox(t, inv.Get("seller", "asset-1"), 10)
	assertApprox(t, state.Scores["seller"], 0.3)
}

func TestApplyDemandTradeNonExecutableCopiesUnchanged(t *testing.T) {
	_, inv, _ := demandTradeFixture()
	state := AcceptanceState{Scores: map[string]float64{"seller": 0.3, "buyer": 0.8}}
	quote := DemandTradeQuote{Seller: "seller", Buyer: "buyer", AssetID: "asset-1"}

	nextInv, nextState := ApplyDemandTrade(inv, state, quote, 0.5)
	nextInv.Set("seller", "asset-1", 0)
	nextState.Scores["seller"] = 0

	assertApprox(t, inv.Get("seller", "asset-1"), 10)
	assertApprox(t, state.Scores["seller"], 0.3)
}

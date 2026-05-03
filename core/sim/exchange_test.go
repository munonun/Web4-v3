package sim

import "testing"

func TestQuoteExchangeValid(t *testing.T) {
	pt := NewPriceTable()
	pt.Set("A", "SKUG", 0.8)
	pt.Set("A", "WEB4", 0.4)

	quote := QuoteExchange(pt, "A", "SKUG", "WEB4")

	if !quote.Valid {
		t.Fatal("expected valid exchange quote")
	}
	assertApprox(t, quote.Rate, 2)
}

func TestQuoteExchangeMissingPriceInvalid(t *testing.T) {
	pt := NewPriceTable()
	pt.Set("A", "SKUG", 0.8)

	quote := QuoteExchange(pt, "A", "SKUG", "WEB4")

	if quote.Valid {
		t.Fatal("expected invalid exchange quote")
	}
}

func TestQuoteExchangeZeroQuotePriceInvalid(t *testing.T) {
	pt := NewPriceTable()
	pt.Set("A", "SKUG", 0.8)
	pt.Set("A", "WEB4", 0)

	quote := QuoteExchange(pt, "A", "SKUG", "WEB4")

	if quote.Valid {
		t.Fatal("expected invalid exchange quote")
	}
}

func TestQuoteCrossAssetTradeExecutable(t *testing.T) {
	pt, inv := crossAssetFixture()

	quote := QuoteCrossAssetTrade(pt, inv, "seller", "buyer", "SKUG", "WEB4", 1, 0)

	if !quote.Executable {
		t.Fatal("expected executable cross-asset trade")
	}
	assertApprox(t, quote.BuyQty, 1.3)
}

func TestQuoteCrossAssetTradeBuyerCannotPay(t *testing.T) {
	pt, inv := crossAssetFixture()
	inv.Set("buyer", "WEB4", 0.5)

	quote := QuoteCrossAssetTrade(pt, inv, "seller", "buyer", "SKUG", "WEB4", 1, 0)

	if quote.Executable {
		t.Fatal("expected buyer payment limit to prevent trade")
	}
}

func TestQuoteCrossAssetTradeSellerLacksInventory(t *testing.T) {
	pt, inv := crossAssetFixture()
	inv.Set("seller", "SKUG", 0)

	quote := QuoteCrossAssetTrade(pt, inv, "seller", "buyer", "SKUG", "WEB4", 1, 0)

	if quote.Executable {
		t.Fatal("expected seller inventory limit to prevent trade")
	}
}

func TestQuoteCrossAssetTradeMissingPriceInvalid(t *testing.T) {
	pt, inv := crossAssetFixture()
	delete(pt.Prices["buyer"], "SKUG")

	quote := QuoteCrossAssetTrade(pt, inv, "seller", "buyer", "SKUG", "WEB4", 1, 0)

	if quote.Executable {
		t.Fatal("expected missing price to prevent trade")
	}
}

func TestApplyCrossAssetTradeTransfersInventoryAndAcceptance(t *testing.T) {
	pt, inv := crossAssetFixture()
	state := NewMultiAcceptanceState()
	state.Set("seller", "SKUG", 0.4)
	state.Set("seller", "WEB4", 0.5)
	state.Set("buyer", "SKUG", 0.9)
	state.Set("buyer", "WEB4", 0.5)
	quote := QuoteCrossAssetTrade(pt, inv, "seller", "buyer", "SKUG", "WEB4", 1, 0)

	nextInv, nextState := ApplyCrossAssetTrade(inv, state, quote, 0.5)

	assertApprox(t, nextInv.Get("seller", "SKUG"), 1)
	assertApprox(t, nextInv.Get("buyer", "SKUG"), 1)
	assertApprox(t, nextInv.Get("buyer", "WEB4"), 0.7)
	assertApprox(t, nextInv.Get("seller", "WEB4"), 1.3)
	assertApprox(t, nextState.Get("seller", "SKUG"), 0.525)
	assertApprox(t, nextState.Get("buyer", "SKUG"), 0.775)
	assertApprox(t, inv.Get("seller", "SKUG"), 2)
	assertApprox(t, state.Get("seller", "SKUG"), 0.4)
}

func crossAssetFixture() (PriceTable, InventoryState) {
	pt := NewPriceTable()
	pt.Set("seller", "SKUG", 0.4)
	pt.Set("seller", "WEB4", 0.5)
	pt.Set("buyer", "SKUG", 0.9)
	pt.Set("buyer", "WEB4", 0.5)

	inv := NewInventoryState()
	inv.Set("seller", "SKUG", 2)
	inv.Set("seller", "WEB4", 0)
	inv.Set("buyer", "SKUG", 0)
	inv.Set("buyer", "WEB4", 2)
	return pt, inv
}

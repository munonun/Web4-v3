package sim

import "testing"

func TestFindArbitrageDetectsBuyLowSellHigh(t *testing.T) {
	pt := NewPriceTable()
	pt.Set("A", "asset-1", 0.2)
	pt.Set("B", "asset-1", 0.8)

	opportunities := FindArbitrage(pt, "asset-1", 0.1)

	if len(opportunities) != 1 {
		t.Fatalf("opportunities length %d, want 1", len(opportunities))
	}
	opp := opportunities[0]
	if opp.BuyFrom != "A" || opp.SellTo != "B" || opp.AssetID != "asset-1" {
		t.Fatalf("unexpected opportunity: %#v", opp)
	}
	assertApprox(t, opp.BuyPrice, 0.2)
	assertApprox(t, opp.SellPrice, 0.8)
	assertApprox(t, opp.Profit, 0.6)
}

func TestFindArbitrageIgnoresBelowMinProfit(t *testing.T) {
	pt := NewPriceTable()
	pt.Set("A", "asset-1", 0.2)
	pt.Set("B", "asset-1", 0.25)

	opportunities := FindArbitrage(pt, "asset-1", 0.1)

	if len(opportunities) != 0 {
		t.Fatalf("expected no opportunities, got %#v", opportunities)
	}
}

func TestFindArbitrageSortedByProfitDescending(t *testing.T) {
	pt := NewPriceTable()
	pt.Set("A", "asset-1", 0.1)
	pt.Set("B", "asset-1", 0.6)
	pt.Set("C", "asset-1", 0.9)

	opportunities := FindArbitrage(pt, "asset-1", 0.1)

	if len(opportunities) < 2 {
		t.Fatalf("expected multiple opportunities, got %#v", opportunities)
	}
	if opportunities[0].BuyFrom != "A" || opportunities[0].SellTo != "C" {
		t.Fatalf("expected top opportunity A->C, got %#v", opportunities[0])
	}
	if opportunities[0].Profit < opportunities[1].Profit {
		t.Fatalf("opportunities not sorted by profit: %#v", opportunities)
	}
}

func TestFindArbitrageDeterministicTieBreak(t *testing.T) {
	pt := NewPriceTable()
	pt.Set("A", "asset-1", 0.1)
	pt.Set("B", "asset-1", 0.1)
	pt.Set("C", "asset-1", 0.5)
	pt.Set("D", "asset-1", 0.5)

	opportunities := FindArbitrage(pt, "asset-1", 0.1)

	if len(opportunities) != 4 {
		t.Fatalf("opportunities length %d, want 4", len(opportunities))
	}
	want := []ArbitrageOpportunity{
		{BuyFrom: "A", SellTo: "C"},
		{BuyFrom: "A", SellTo: "D"},
		{BuyFrom: "B", SellTo: "C"},
		{BuyFrom: "B", SellTo: "D"},
	}
	for i := range want {
		if opportunities[i].BuyFrom != want[i].BuyFrom || opportunities[i].SellTo != want[i].SellTo {
			t.Fatalf("opportunity %d got %#v, want buy %s sell %s", i, opportunities[i], want[i].BuyFrom, want[i].SellTo)
		}
	}
}

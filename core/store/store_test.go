package store_test

import (
	"testing"

	"web4-v3/core/crypto"
	"web4-v3/core/model"
	"web4-v3/core/node"
	"web4-v3/core/price"
	"web4-v3/core/store"
)

func TestJSONStorePersistsInventoryFlowAndPriceState(t *testing.T) {
	s, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	n := newStoredNode(t, s, 10)
	unit := testUnit(t, n.ID, "SKUG")
	n.AddInventory(unit, model.FromFloat(5))
	n.Flow[unit] = model.FlowRecord{Unit: unit, TradeVolume: model.FromFloat(2)}
	n.PriceState[unit] = price.PriceResult{FinalPrice: 1.25, SeedPrice: 1}

	if err := s.SaveInventory(n.ID, n.Inventory); err != nil {
		t.Fatalf("save inventory: %v", err)
	}
	if err := s.SaveFlow(n.ID, n.Flow); err != nil {
		t.Fatalf("save flow: %v", err)
	}
	if err := s.SavePriceState(n.ID, n.PriceState); err != nil {
		t.Fatalf("save price: %v", err)
	}

	restarted, err := node.NewNodeWithStore(n.PrivateKey, node.DefaultPriceConfig(), s)
	if err != nil {
		t.Fatalf("restart node: %v", err)
	}
	if got := restarted.Balance(unit); got != model.FromFloat(5) {
		t.Fatalf("balance %d, want %d", got, model.FromFloat(5))
	}
	if got := restarted.Flow[unit].TradeVolume; got != model.FromFloat(2) {
		t.Fatalf("flow %d, want %d", got, model.FromFloat(2))
	}
	if got := restarted.PriceState[unit].FinalPrice; got != 1.25 {
		t.Fatalf("price %f, want 1.25", got)
	}
}

func TestJSONStoreReplaySurvivesRestart(t *testing.T) {
	root := t.TempDir()
	s, err := store.NewJSONStore(root)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	seller, buyer, sellUnit, buyUnit := storedTradeNodes(t, s)
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerSig, buyerSig := signBoth(t, seller, buyer, q)

	tx, err := node.ExecuteSignedTrade(seller, buyer, q, sellerSig, buyerSig)
	if err != nil {
		t.Fatalf("execute signed trade: %v", err)
	}
	auth := node.AuthorizedTradeTx{Tx: *tx, SellerAuth: sellerSig, BuyerAuth: buyerSig}
	authID, err := node.AuthorizedTradeID(auth)
	if err != nil {
		t.Fatalf("auth id: %v", err)
	}
	if !s.HasExecutedTrade(authID) {
		t.Fatal("trade was not marked executed")
	}

	reopened, err := store.NewJSONStore(root)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	if !reopened.HasExecutedTrade(authID) {
		t.Fatal("executed trade mark did not survive restart")
	}
	restartedSeller, err := node.NewNodeWithStore(seller.PrivateKey, node.DefaultPriceConfig(), reopened)
	if err != nil {
		t.Fatalf("restart seller: %v", err)
	}
	restartedBuyer, err := node.NewNodeWithStore(buyer.PrivateKey, node.DefaultPriceConfig(), reopened)
	if err != nil {
		t.Fatalf("restart buyer: %v", err)
	}
	if _, err := node.ExecuteSignedTrade(restartedSeller, restartedBuyer, q, sellerSig, buyerSig); err == nil {
		t.Fatal("expected replay rejection after restart")
	}
}

func TestJSONStorePersistsAuthorizedTrade(t *testing.T) {
	s, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	seller, buyer, sellUnit, buyUnit := storedTradeNodes(t, s)
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerSig, buyerSig := signBoth(t, seller, buyer, q)
	tx, err := node.ExecuteSignedTrade(seller, buyer, q, sellerSig, buyerSig)
	if err != nil {
		t.Fatalf("execute signed trade: %v", err)
	}
	auth := node.AuthorizedTradeTx{Tx: *tx, SellerAuth: sellerSig, BuyerAuth: buyerSig}
	authID, err := node.AuthorizedTradeID(auth)
	if err != nil {
		t.Fatalf("auth id: %v", err)
	}

	loaded, ok := s.LoadAuthorizedTrade(authID)
	if !ok {
		t.Fatal("authorized trade not found")
	}
	loadedID, err := node.AuthorizedTradeID(loaded)
	if err != nil {
		t.Fatalf("loaded auth id: %v", err)
	}
	if loadedID != authID {
		t.Fatalf("loaded auth ID %x, want %x", loadedID, authID)
	}
}

func TestRejectReplay(t *testing.T) {
	s, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	id := model.TxID{1, 2, 3}
	if err := store.RejectReplay(s, id); err != nil {
		t.Fatalf("first replay check: %v", err)
	}
	if err := store.RejectReplay(s, id); err == nil {
		t.Fatal("expected replay rejection")
	}
}

func storedTradeNodes(t *testing.T, s *store.JSONStore) (*node.Node, *node.Node, model.UnitID, model.UnitID) {
	t.Helper()
	seller := newStoredNode(t, s, 100)
	buyer := newStoredNode(t, s, 100)
	sellUnit := testUnit(t, seller.ID, "SKUG")
	buyUnit := testUnit(t, buyer.ID, "WEB4")
	configureUnit(t, seller, sellUnit)
	configureUnit(t, seller, buyUnit)
	configureUnit(t, buyer, sellUnit)
	configureUnit(t, buyer, buyUnit)
	seller.AddInventory(sellUnit, model.FromFloat(10))
	buyer.AddInventory(buyUnit, model.FromFloat(10))
	return seller, buyer, sellUnit, buyUnit
}

func newStoredNode(t *testing.T, s *store.JSONStore, now int64) *node.Node {
	t.Helper()
	_, priv, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	n, err := node.NewNodeWithStore(priv, node.DefaultPriceConfig(), s)
	if err != nil {
		t.Fatalf("new node with store: %v", err)
	}
	n.NowUnix = func() int64 { return now }
	return n
}

func configureUnit(t *testing.T, n *node.Node, unit model.UnitID) {
	t.Helper()
	n.Features[unit] = price.AssetFeatures{Cost: 1}
	n.PriceConfig = price.PriceConfig{
		BasePrice:       1,
		Weights:         price.FeatureWeights{Cost: 1},
		VolumeThreshold: model.FromFloat(10),
	}
	n.ComputePrice(unit)
}

func signBoth(t *testing.T, seller *node.Node, buyer *node.Node, q node.Quote) (node.SignedTradeIntent, node.SignedTradeIntent) {
	t.Helper()
	sellerSig, err := seller.SignQuote(q)
	if err != nil {
		t.Fatalf("seller sign: %v", err)
	}
	buyerSig, err := buyer.SignQuote(q)
	if err != nil {
		t.Fatalf("buyer sign: %v", err)
	}
	return sellerSig, buyerSig
}

func testUnit(t *testing.T, issuer model.NodeID, metadata string) model.UnitID {
	t.Helper()
	unit, err := model.NewUnitIDFromMetadata(issuer, []byte(metadata))
	if err != nil {
		t.Fatalf("unit id: %v", err)
	}
	return unit
}

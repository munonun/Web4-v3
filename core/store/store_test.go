package store_test

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
	if got := restartedSeller.Balance(sellUnit); got != model.FromFloat(8) {
		t.Fatalf("restarted seller sell balance %d, want %d", got, model.FromFloat(8))
	}
	if got := restartedSeller.Balance(buyUnit); got != model.FromFloat(2) {
		t.Fatalf("restarted seller buy balance %d, want %d", got, model.FromFloat(2))
	}
	if got := restartedBuyer.Balance(sellUnit); got != model.FromFloat(2) {
		t.Fatalf("restarted buyer sell balance %d, want %d", got, model.FromFloat(2))
	}
	if got := restartedBuyer.Balance(buyUnit); got != model.FromFloat(8) {
		t.Fatalf("restarted buyer buy balance %d, want %d", got, model.FromFloat(8))
	}
	if restartedSeller.Flow[sellUnit].TradeVolume == 0 || restartedBuyer.Flow[buyUnit].PaymentVolume == 0 {
		t.Fatalf("flow state was not persisted seller=%+v buyer=%+v", restartedSeller.Flow, restartedBuyer.Flow)
	}
	if _, ok := reopened.LoadAuthorizedTrade(authID); !ok {
		t.Fatal("authorized trade was not persisted")
	}
	if _, err := node.ExecuteSignedTrade(restartedSeller, restartedBuyer, q, sellerSig, buyerSig); err == nil {
		t.Fatal("expected replay rejection after restart")
	}
}

func TestJSONStoreRecoveryCompletesMarkerAfterStateTradeCrash(t *testing.T) {
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
	if err := os.Remove(executedPath(root, authID)); err != nil {
		t.Fatalf("simulate missing marker: %v", err)
	}
	if err := os.WriteFile(reservationPath(root, authID), []byte("executing"), 0o600); err != nil {
		t.Fatalf("simulate executing record: %v", err)
	}

	recovered, err := store.NewJSONStore(root)
	if err != nil {
		t.Fatalf("recover store: %v", err)
	}
	if !recovered.HasExecutedTrade(authID) {
		t.Fatal("recovery did not mark trade executed")
	}
	if _, err := os.Stat(executedPath(root, authID)); err != nil {
		t.Fatalf("recovery did not recreate marker: %v", err)
	}
	restartedSeller, err := node.NewNodeWithStore(seller.PrivateKey, node.DefaultPriceConfig(), recovered)
	if err != nil {
		t.Fatalf("restart seller: %v", err)
	}
	restartedBuyer, err := node.NewNodeWithStore(buyer.PrivateKey, node.DefaultPriceConfig(), recovered)
	if err != nil {
		t.Fatalf("restart buyer: %v", err)
	}
	if got := restartedSeller.Balance(sellUnit); got != model.FromFloat(8) {
		t.Fatalf("recovered seller sell balance %d, want %d", got, model.FromFloat(8))
	}
	if _, err := node.ExecuteSignedTrade(restartedSeller, restartedBuyer, q, sellerSig, buyerSig); err == nil {
		t.Fatal("expected replay rejection after recovered marker")
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

func TestJSONStoreSaveAuthorizedTradeDoesNotMarkExecuted(t *testing.T) {
	root := t.TempDir()
	s, err := store.NewJSONStore(root)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	id := model.TxID{7, 7, 7}
	if err := s.SaveAuthorizedTrade(id, node.AuthorizedTradeTx{}); err != nil {
		t.Fatalf("save authorized trade: %v", err)
	}
	reopened, err := store.NewJSONStore(root)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	if _, ok := reopened.LoadAuthorizedTrade(id); !ok {
		t.Fatal("authorized trade did not load after restart")
	}
	if reopened.HasExecutedTrade(id) {
		t.Fatal("saved authorized trade was treated as executed")
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

func TestJSONStoreConcurrentPersistExecutedTradeOnlyOneSucceeds(t *testing.T) {
	s, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	id := model.TxID{9, 9, 9}
	const workers = 16
	start := make(chan struct{})
	results := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- s.PersistExecutedTrade(id, node.AuthorizedTradeTx{})
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("successful persists %d, want 1", success)
	}
	if !s.HasExecutedTrade(id) {
		t.Fatal("winner did not write replay marker")
	}
}

func TestJSONStoreExistingMarkerRejects(t *testing.T) {
	s, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	id := model.TxID{4, 5, 6}
	if err := s.MarkExecutedTrade(id); err != nil {
		t.Fatalf("mark executed: %v", err)
	}
	if err := s.MarkExecutedTrade(id); err == nil {
		t.Fatal("expected existing marker rejection")
	}
	if err := s.PersistExecutedTrade(id, node.AuthorizedTradeTx{}); err == nil {
		t.Fatal("expected persist existing marker rejection")
	}
}

func TestJSONStoreFailedTradePathDoesNotLeaveReplayMarker(t *testing.T) {
	root := t.TempDir()
	s, err := store.NewJSONStore(root)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	id := model.TxID{7, 7, 7}
	if err := os.Mkdir(tradePath(root, id), 0o755); err != nil {
		t.Fatalf("make trade path dir: %v", err)
	}
	if err := s.PersistExecutedTrade(id, node.AuthorizedTradeTx{}); err == nil {
		t.Fatal("expected persistence failure")
	}
	if s.HasExecutedTrade(id) {
		t.Fatal("failed persistence left replay marker")
	}
	reopened, err := store.NewJSONStore(root)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	if reopened.HasExecutedTrade(id) {
		t.Fatal("restart treated failed persistence as executed")
	}
}

func TestJSONStoreRejectsSwappedNodeState(t *testing.T) {
	root := t.TempDir()
	s, err := store.NewJSONStore(root)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	n1 := newStoredNode(t, s, 10)
	n2 := newStoredNode(t, s, 10)
	unit := testUnit(t, n1.ID, "SKUG")
	n1.AddInventory(unit, model.FromFloat(5))
	if err := s.SaveInventory(n1.ID, n1.Inventory); err != nil {
		t.Fatalf("save inventory: %v", err)
	}
	data, err := os.ReadFile(inventoryPath(root, n1.ID))
	if err != nil {
		t.Fatalf("read inventory: %v", err)
	}
	if err := os.WriteFile(inventoryPath(root, n2.ID), data, 0o644); err != nil {
		t.Fatalf("write swapped inventory: %v", err)
	}
	if _, err := s.LoadInventory(n2.ID); err == nil {
		t.Fatal("expected swapped inventory rejection")
	}
}

func TestJSONStoreMalformedStateReturnsError(t *testing.T) {
	root := t.TempDir()
	s, err := store.NewJSONStore(root)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	n := newStoredNode(t, s, 10)
	if err := os.WriteFile(inventoryPath(root, n.ID), []byte("{bad"), 0o644); err != nil {
		t.Fatalf("write malformed inventory: %v", err)
	}
	if _, err := s.LoadInventory(n.ID); err == nil {
		t.Fatal("expected malformed inventory error")
	}
	if err := os.WriteFile(inventoryPath(root, n.ID), nil, 0o644); err != nil {
		t.Fatalf("write empty inventory: %v", err)
	}
	if _, err := s.LoadInventory(n.ID); err == nil {
		t.Fatal("expected empty inventory error")
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

func inventoryPath(root string, id model.NodeID) string {
	return filepath.Join(root, "inventory", hex.EncodeToString(id[:])+".json")
}

func tradePath(root string, id model.TxID) string {
	return filepath.Join(root, "trades", fmt.Sprintf("%x", id[:])+".json")
}

func executedPath(root string, id model.TxID) string {
	return filepath.Join(root, "trades", fmt.Sprintf("%x", id[:])+".executed.json")
}

func reservationPath(root string, id model.TxID) string {
	return filepath.Join(root, "trades", fmt.Sprintf("%x", id[:])+".executing")
}

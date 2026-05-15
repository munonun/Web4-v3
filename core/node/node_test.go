package node

import (
	"testing"

	"web4-v3/core/crypto"
	"web4-v3/core/model"
	"web4-v3/core/price"
)

func TestInventoryAddSubtract(t *testing.T) {
	n := testNode(t, 0)
	unit := testUnit(t, n.ID, "SKUG")

	n.AddInventory(unit, model.FromFloat(10))
	if got := n.Balance(unit); got != model.FromFloat(10) {
		t.Fatalf("balance %d, want %d", got, model.FromFloat(10))
	}
	if err := n.SubInventory(unit, model.FromFloat(3)); err != nil {
		t.Fatalf("sub inventory: %v", err)
	}
	if got := n.Balance(unit); got != model.FromFloat(7) {
		t.Fatalf("balance %d, want %d", got, model.FromFloat(7))
	}
	if err := n.SubInventory(unit, model.FromFloat(8)); err == nil {
		t.Fatal("expected negative subtraction to fail")
	}
	if got := n.Balance(unit); got != model.FromFloat(7) {
		t.Fatalf("failed subtraction mutated balance to %d", got)
	}
}

func TestNodeComputesSeedPriceWithoutTrades(t *testing.T) {
	n := testNode(t, 0)
	unit := testUnit(t, n.ID, "SKUG")
	n.PriceConfig = price.PriceConfig{
		BasePrice:       10,
		Weights:         price.FeatureWeights{Cost: 1},
		VolumeThreshold: model.FromFloat(10),
	}
	n.Features[unit] = price.AssetFeatures{Cost: 0.5}

	result := n.ComputePrice(unit)
	if result.FinalPrice != 5 {
		t.Fatalf("price %f, want 5", result.FinalPrice)
	}
}

func TestNodePriceChangesAfterTradeObservation(t *testing.T) {
	n := testNode(t, 0)
	unit := testUnit(t, n.ID, "SKUG")
	n.PriceConfig = price.PriceConfig{
		BasePrice:       10,
		Weights:         price.FeatureWeights{Cost: 1},
		VolumeThreshold: model.FromFloat(10),
	}
	n.Features[unit] = price.AssetFeatures{Cost: 0.5}
	n.TradeHistory[unit] = []price.TradeObservation{{Price: 20, Volume: model.FromFloat(10), Weight: 1, TimeUnix: 0}}
	n.SettledVolume[unit] = model.FromFloat(10)
	n.LastTradeUnix[unit] = 0

	result := n.ComputePrice(unit)
	if result.FinalPrice != 20 {
		t.Fatalf("price %f, want 20", result.FinalPrice)
	}
}

func TestNodeInactivePriceDecays(t *testing.T) {
	n := testNode(t, 10)
	unit := testUnit(t, n.ID, "SKUG")
	n.PriceConfig = price.PriceConfig{
		BasePrice:       10,
		Weights:         price.FeatureWeights{Cost: 1},
		VolumeThreshold: model.FromFloat(10),
		DecayK:          0.1,
	}
	n.Features[unit] = price.AssetFeatures{Cost: 1}
	n.LastTradeUnix[unit] = 0

	result := n.ComputePrice(unit)
	if result.FinalPrice >= 10 || result.FinalPrice <= 0 {
		t.Fatalf("decayed price %f, want in (0,10)", result.FinalPrice)
	}
}

func TestQuoteSellExecutable(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testTradeNodes(t)

	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	if !q.Executable {
		t.Fatalf("expected executable quote: %+v", q)
	}
	if q.BuyAmount != model.FromFloat(2) {
		t.Fatalf("buy amount %d, want %d", q.BuyAmount, model.FromFloat(2))
	}
}

func TestQuoteSellFailsWithUsefulReasons(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testTradeNodes(t)

	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(20), 0)
	if q.Executable || q.Reason == "" {
		t.Fatalf("expected seller inventory failure with reason: %+v", q)
	}

	if err := buyer.SubInventory(buyUnit, model.FromFloat(9)); err != nil {
		t.Fatalf("prepare buyer inventory: %v", err)
	}
	q = seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	if q.Executable || q.Reason == "" {
		t.Fatalf("expected buyer inventory failure with reason: %+v", q)
	}
}

func TestAcceptQuoteRequiresPartyExecutableAndInventory(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testTradeNodes(t)
	outsider := testNode(t, 0)
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)

	if !seller.AcceptQuote(q) || !buyer.AcceptQuote(q) {
		t.Fatalf("expected parties to accept quote: %+v", q)
	}
	if outsider.AcceptQuote(q) {
		t.Fatal("outsider accepted quote")
	}
	q.Executable = false
	if seller.AcceptQuote(q) {
		t.Fatal("accepted non-executable quote")
	}
}

func TestExecuteTradeUpdatesInventoriesAndCreatesTradeTx(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testTradeNodes(t)
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)

	tx, err := ExecuteTrade(seller, buyer, q)
	if err != nil {
		t.Fatalf("execute trade: %v", err)
	}
	if tx == nil || tx.ID == (model.TxID{}) {
		t.Fatalf("missing trade tx: %+v", tx)
	}
	if seller.Balance(sellUnit) != model.FromFloat(8) || seller.Balance(buyUnit) != model.FromFloat(2) {
		t.Fatalf("bad seller balances sell=%d buy=%d", seller.Balance(sellUnit), seller.Balance(buyUnit))
	}
	if buyer.Balance(sellUnit) != model.FromFloat(2) || buyer.Balance(buyUnit) != model.FromFloat(8) {
		t.Fatalf("bad buyer balances sell=%d buy=%d", buyer.Balance(sellUnit), buyer.Balance(buyUnit))
	}
	if tx.InputsA[0].Amount != tx.OutputsB[0].Amount || tx.InputsB[0].Amount != tx.OutputsA[0].Amount {
		t.Fatalf("trade tx does not conserve exact amounts: %+v", tx)
	}
}

func TestExecuteTradeFailureLeavesInventoriesUnchanged(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testTradeNodes(t)
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	buyer.Inventory = model.NewInventoryState()

	_, err := ExecuteTrade(seller, buyer, q)
	if err == nil {
		t.Fatal("expected execution failure")
	}
	if seller.Balance(sellUnit) != model.FromFloat(10) || buyer.Balance(buyUnit) != 0 {
		t.Fatalf("failed trade mutated balances seller=%d buyer=%d", seller.Balance(sellUnit), buyer.Balance(buyUnit))
	}
}

func TestExecuteTradeRecordsFlowAndObservations(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testTradeNodes(t)
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)

	if _, err := ExecuteTrade(seller, buyer, q); err != nil {
		t.Fatalf("execute trade: %v", err)
	}
	if seller.Flow[sellUnit].TradeVolume != model.FromFloat(2) || buyer.Flow[sellUnit].TradeVolume != model.FromFloat(2) {
		t.Fatalf("missing sell-unit trade flow seller=%+v buyer=%+v", seller.Flow[sellUnit], buyer.Flow[sellUnit])
	}
	if seller.Flow[buyUnit].PaymentVolume != model.FromFloat(2) || buyer.Flow[buyUnit].PaymentVolume != model.FromFloat(2) {
		t.Fatalf("missing payment flow seller=%+v buyer=%+v", seller.Flow[buyUnit], buyer.Flow[buyUnit])
	}
	if len(seller.TradeHistory[sellUnit]) != 1 || len(buyer.TradeHistory[buyUnit]) != 1 {
		t.Fatalf("missing observations seller=%d buyer=%d", len(seller.TradeHistory[sellUnit]), len(buyer.TradeHistory[buyUnit]))
	}
	if seller.SettledVolume[sellUnit] != model.FromFloat(2) || buyer.SettledVolume[buyUnit] != model.FromFloat(2) {
		t.Fatalf("missing settled volume")
	}
}

func TestTradeIntentIDDeterministicAndAmountSensitive(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testSignedTradeNodes(t)
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	intentA := IntentFromQuote(q, seller.ID, q.Timestamp)
	intentB := IntentFromQuote(q, seller.ID, q.Timestamp)

	idA, err := TradeIntentID(intentA)
	if err != nil {
		t.Fatalf("intent id A: %v", err)
	}
	idB, err := TradeIntentID(intentB)
	if err != nil {
		t.Fatalf("intent id B: %v", err)
	}
	if idA != idB {
		t.Fatal("same intent produced different IDs")
	}
	intentB.SellAmount = model.FromFloat(3)
	idC, err := TradeIntentID(intentB)
	if err != nil {
		t.Fatalf("intent id C: %v", err)
	}
	if idA == idC {
		t.Fatal("changed amount did not change intent ID")
	}
	buyerIntent := IntentFromQuote(q, buyer.ID, q.Timestamp)
	if !economicTermsMatch(intentA, buyerIntent) {
		t.Fatal("seller and buyer intents should share economic terms")
	}
}

func TestSignVerifyTradeIntent(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testSignedTradeNodes(t)
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerIntent := IntentFromQuote(q, seller.ID, q.Timestamp)
	buyerIntent := IntentFromQuote(q, buyer.ID, q.Timestamp)

	sellerSig, err := SignTradeIntent(seller.PrivateKey, sellerIntent)
	if err != nil {
		t.Fatalf("sign seller intent: %v", err)
	}
	buyerSig, err := SignTradeIntent(buyer.PrivateKey, buyerIntent)
	if err != nil {
		t.Fatalf("sign buyer intent: %v", err)
	}
	if !VerifyTradeIntent(sellerSig) || !VerifyTradeIntent(buyerSig) {
		t.Fatal("valid signatures did not verify")
	}

	tampered := sellerSig
	tampered.Intent.BuyAmount = model.FromFloat(3)
	if VerifyTradeIntent(tampered) {
		t.Fatal("tampered intent verified")
	}
	wrongKey := sellerSig
	wrongKey.PublicKey = buyer.PublicKey
	if VerifyTradeIntent(wrongKey) {
		t.Fatal("wrong public key verified")
	}
	malformed := sellerSig
	malformed.Signature = malformed.Signature[:8]
	if VerifyTradeIntent(malformed) {
		t.Fatal("malformed signature verified")
	}
}

func TestNodeSignQuoteRules(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testSignedTradeNodes(t)
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)

	if _, err := seller.SignQuote(q); err != nil {
		t.Fatalf("seller sign quote: %v", err)
	}
	if _, err := buyer.SignQuote(q); err != nil {
		t.Fatalf("buyer sign quote: %v", err)
	}
	outsider := testSignedNode(t, 0)
	if _, err := outsider.SignQuote(q); err == nil {
		t.Fatal("non-party signed quote")
	}
	if err := seller.SubInventory(sellUnit, model.FromFloat(9)); err != nil {
		t.Fatalf("prepare seller inventory: %v", err)
	}
	if _, err := seller.SignQuote(q); err == nil {
		t.Fatal("seller signed after losing required inventory")
	}
}

func TestExecuteSignedTradeSucceeds(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testSignedTradeNodes(t)
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerSig, buyerSig := signQuoteBoth(t, seller, buyer, q)

	tx, err := ExecuteSignedTrade(seller, buyer, q, sellerSig, buyerSig)
	if err != nil {
		t.Fatalf("execute signed trade: %v", err)
	}
	if tx == nil || tx.ID == (model.TxID{}) {
		t.Fatalf("missing trade tx: %+v", tx)
	}
	if seller.Balance(sellUnit) != model.FromFloat(8) || buyer.Balance(buyUnit) != model.FromFloat(8) {
		t.Fatalf("bad balances seller=%d buyer=%d", seller.Balance(sellUnit), buyer.Balance(buyUnit))
	}
	if seller.Flow[sellUnit].TradeVolume != model.FromFloat(2) || buyer.Flow[buyUnit].PaymentVolume != model.FromFloat(2) {
		t.Fatalf("missing flow seller=%+v buyer=%+v", seller.Flow[sellUnit], buyer.Flow[buyUnit])
	}
}

func TestExecuteSignedTradeRejectsMissingAndMismatchedAuth(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testSignedTradeNodes(t)
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerSig, buyerSig := signQuoteBoth(t, seller, buyer, q)

	if _, err := ExecuteSignedTrade(seller, buyer, q, SignedTradeIntent{}, buyerSig); err == nil {
		t.Fatal("missing seller signature succeeded")
	}
	if _, err := ExecuteSignedTrade(seller, buyer, q, sellerSig, SignedTradeIntent{}); err == nil {
		t.Fatal("missing buyer signature succeeded")
	}

	badBuyer := buyerSig
	badBuyer.Intent.BuyAmount = model.FromFloat(3)
	badBuyer, err := SignTradeIntent(buyer.PrivateKey, badBuyer.Intent)
	if err != nil {
		t.Fatalf("sign mismatched buyer intent: %v", err)
	}
	if _, err := ExecuteSignedTrade(seller, buyer, q, sellerSig, badBuyer); err == nil {
		t.Fatal("mismatched buyer intent succeeded")
	}
}

func TestExecuteSignedTradeTamperedQuoteFailsWithoutMutation(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testSignedTradeNodes(t)
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerSig, buyerSig := signQuoteBoth(t, seller, buyer, q)
	tampered := q
	tampered.BuyAmount = model.FromFloat(3)

	_, err := ExecuteSignedTrade(seller, buyer, tampered, sellerSig, buyerSig)
	if err == nil {
		t.Fatal("tampered quote succeeded")
	}
	if seller.Balance(sellUnit) != model.FromFloat(10) || buyer.Balance(buyUnit) != model.FromFloat(10) {
		t.Fatalf("failed signed trade mutated balances seller=%d buyer=%d", seller.Balance(sellUnit), buyer.Balance(buyUnit))
	}
}

func TestExecuteSignedTradeRejectsReplayFromStore(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testSignedTradeNodes(t)
	replayStore := newFakeStore()
	seller.Store = replayStore
	buyer.Store = replayStore
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerSig, buyerSig := signQuoteBoth(t, seller, buyer, q)

	if _, err := ExecuteSignedTrade(seller, buyer, q, sellerSig, buyerSig); err != nil {
		t.Fatalf("first signed trade: %v", err)
	}
	if _, err := ExecuteSignedTrade(seller, buyer, q, sellerSig, buyerSig); err == nil {
		t.Fatal("expected replay rejection")
	}
	if seller.Balance(sellUnit) != model.FromFloat(8) || buyer.Balance(buyUnit) != model.FromFloat(8) {
		t.Fatalf("replay mutated balances seller=%d buyer=%d", seller.Balance(sellUnit), buyer.Balance(buyUnit))
	}
}

func TestExecuteSignedTradePersistenceFailureDoesNotMutateRuntime(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testSignedTradeNodes(t)
	failing := newFakeStore()
	failing.failSaveInventory = true
	seller.Store = failing
	buyer.Store = failing
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerSig, buyerSig := signQuoteBoth(t, seller, buyer, q)

	if _, err := ExecuteSignedTrade(seller, buyer, q, sellerSig, buyerSig); err == nil {
		t.Fatal("expected persistence failure")
	}
	if seller.Balance(sellUnit) != model.FromFloat(10) || buyer.Balance(buyUnit) != model.FromFloat(10) {
		t.Fatalf("failed persistence mutated balances seller=%d buyer=%d", seller.Balance(sellUnit), buyer.Balance(buyUnit))
	}
	if failing.markedCount != 0 {
		t.Fatalf("replay mark happened despite failed persistence")
	}
}

func TestAuthorizedTradeIDDeterministic(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testSignedTradeNodes(t)
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerSig, buyerSig := signQuoteBoth(t, seller, buyer, q)
	tx, err := buildTradeTx(seller.ID, buyer.ID, q, q.Timestamp)
	if err != nil {
		t.Fatalf("build tx: %v", err)
	}
	auth := AuthorizedTradeTx{Tx: *tx, SellerAuth: sellerSig, BuyerAuth: buyerSig}
	a, err := AuthorizedTradeID(auth)
	if err != nil {
		t.Fatalf("auth id A: %v", err)
	}
	b, err := AuthorizedTradeID(auth)
	if err != nil {
		t.Fatalf("auth id B: %v", err)
	}
	if a != b {
		t.Fatal("authorized trade ID is not deterministic")
	}
}

func testTradeNodes(t *testing.T) (*Node, *Node, model.UnitID, model.UnitID) {
	t.Helper()
	seller := testNode(t, 0)
	buyer := testNode(t, 0)
	sellUnit := testUnit(t, seller.ID, "SKUG")
	buyUnit := testUnit(t, buyer.ID, "WEB4")
	configureUnit(t, seller, sellUnit, 1)
	configureUnit(t, seller, buyUnit, 1)
	configureUnit(t, buyer, sellUnit, 1)
	configureUnit(t, buyer, buyUnit, 1)
	seller.AddInventory(sellUnit, model.FromFloat(10))
	buyer.AddInventory(buyUnit, model.FromFloat(10))
	return seller, buyer, sellUnit, buyUnit
}

func testSignedTradeNodes(t *testing.T) (*Node, *Node, model.UnitID, model.UnitID) {
	t.Helper()
	seller := testSignedNode(t, 100)
	buyer := testSignedNode(t, 100)
	sellUnit := testUnit(t, seller.ID, "SKUG")
	buyUnit := testUnit(t, buyer.ID, "WEB4")
	configureUnit(t, seller, sellUnit, 1)
	configureUnit(t, seller, buyUnit, 1)
	configureUnit(t, buyer, sellUnit, 1)
	configureUnit(t, buyer, buyUnit, 1)
	seller.AddInventory(sellUnit, model.FromFloat(10))
	buyer.AddInventory(buyUnit, model.FromFloat(10))
	return seller, buyer, sellUnit, buyUnit
}

func testSignedNode(t *testing.T, now int64) *Node {
	t.Helper()
	_, priv, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	n, err := NewNode(priv, DefaultPriceConfig())
	if err != nil {
		t.Fatalf("new node: %v", err)
	}
	n.NowUnix = func() int64 { return now }
	return n
}

func signQuoteBoth(t *testing.T, seller *Node, buyer *Node, q Quote) (SignedTradeIntent, SignedTradeIntent) {
	t.Helper()
	sellerSig, err := seller.SignQuote(q)
	if err != nil {
		t.Fatalf("seller sign quote: %v", err)
	}
	buyerSig, err := buyer.SignQuote(q)
	if err != nil {
		t.Fatalf("buyer sign quote: %v", err)
	}
	return sellerSig, buyerSig
}

func configureUnit(t *testing.T, n *Node, unit model.UnitID, score float64) {
	t.Helper()
	n.Features[unit] = price.AssetFeatures{Cost: score}
	n.PriceConfig = price.PriceConfig{
		BasePrice:       1,
		Weights:         price.FeatureWeights{Cost: 1},
		VolumeThreshold: model.FromFloat(10),
	}
	n.ComputePrice(unit)
}

func testNode(t *testing.T, now int64) *Node {
	t.Helper()
	pub, _, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	id, err := model.NodeIDFromPublicKey(pub)
	if err != nil {
		t.Fatalf("node id: %v", err)
	}
	n := New(id)
	n.NowUnix = func() int64 { return now }
	return n
}

func testUnit(t *testing.T, issuer model.NodeID, metadata string) model.UnitID {
	t.Helper()
	unit, err := model.NewUnitIDFromMetadata(issuer, []byte(metadata))
	if err != nil {
		t.Fatalf("unit id: %v", err)
	}
	return unit
}

type fakeStore struct {
	executed          map[model.TxID]bool
	inventory         map[model.NodeID]model.InventoryState
	flow              map[model.NodeID]map[model.UnitID]model.FlowRecord
	prices            map[model.NodeID]map[model.UnitID]price.PriceResult
	trades            map[model.TxID]AuthorizedTradeTx
	failSaveInventory bool
	markedCount       int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		executed:  map[model.TxID]bool{},
		inventory: map[model.NodeID]model.InventoryState{},
		flow:      map[model.NodeID]map[model.UnitID]model.FlowRecord{},
		prices:    map[model.NodeID]map[model.UnitID]price.PriceResult{},
		trades:    map[model.TxID]AuthorizedTradeTx{},
	}
}

func (s *fakeStore) HasExecutedTrade(id model.TxID) bool { return s.executed[id] }
func (s *fakeStore) MarkExecutedTrade(id model.TxID) error {
	s.executed[id] = true
	s.markedCount++
	return nil
}
func (s *fakeStore) SaveInventory(id model.NodeID, inv model.InventoryState) error {
	if s.failSaveInventory {
		return errFakeStore
	}
	s.inventory[id] = inv.Copy()
	return nil
}
func (s *fakeStore) LoadInventory(id model.NodeID) (model.InventoryState, error) {
	if inv, ok := s.inventory[id]; ok {
		return inv.Copy(), nil
	}
	return model.NewInventoryState(), nil
}
func (s *fakeStore) SaveFlow(id model.NodeID, flow map[model.UnitID]model.FlowRecord) error {
	s.flow[id] = copyFlow(flow)
	return nil
}
func (s *fakeStore) LoadFlow(id model.NodeID) (map[model.UnitID]model.FlowRecord, error) {
	return copyFlow(s.flow[id]), nil
}
func (s *fakeStore) SavePriceState(id model.NodeID, state map[model.UnitID]price.PriceResult) error {
	s.prices[id] = copyPriceState(state)
	return nil
}
func (s *fakeStore) LoadPriceState(id model.NodeID) (map[model.UnitID]price.PriceResult, error) {
	return copyPriceState(s.prices[id]), nil
}
func (s *fakeStore) SaveAuthorizedTrade(id model.TxID, tx AuthorizedTradeTx) error {
	s.trades[id] = tx
	return nil
}
func (s *fakeStore) LoadAuthorizedTrade(id model.TxID) (AuthorizedTradeTx, bool) {
	tx, ok := s.trades[id]
	return tx, ok
}
func (s *fakeStore) Close() error { return nil }

var errFakeStore = &fakeStoreError{}

type fakeStoreError struct{}

func (*fakeStoreError) Error() string { return "fake store failure" }

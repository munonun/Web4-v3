package node

import (
	"bytes"
	"fmt"
	"sync"

	"web4-v3/core/model"
	"web4-v3/core/price"
)

var signedTradeExecutionMu sync.Mutex

func ExecuteTrade(seller *Node, buyer *Node, q Quote) (*model.TradeTx, error) {
	if seller == nil || buyer == nil {
		return nil, fmt.Errorf("seller and buyer are required")
	}
	signedTradeExecutionMu.Lock()
	defer signedTradeExecutionMu.Unlock()
	unlock := lockTradeNodes(seller, buyer)
	defer unlock()
	seller.init()
	buyer.init()
	if !q.Executable {
		return nil, quoteExecutionError(q)
	}
	if seller.ID != q.Seller || buyer.ID != q.Buyer {
		return nil, fmt.Errorf("quote parties do not match nodes")
	}
	if !seller.AcceptQuote(q) {
		return nil, fmt.Errorf("seller rejected quote")
	}
	if !buyer.AcceptQuote(q) {
		return nil, fmt.Errorf("buyer rejected quote")
	}

	now := maxInt64(seller.NowUnix(), buyer.NowUnix())
	tx, err := buildTradeTx(seller.ID, buyer.ID, q, now)
	if err != nil {
		return nil, err
	}
	combined := combinedInventory(seller, buyer)
	if err := model.ValidateTradeTx(tx, combined); err != nil {
		return nil, err
	}

	nextSeller := seller.Inventory.Copy()
	nextBuyer := buyer.Inventory.Copy()
	if err := nextSeller.Sub(seller.ID, q.SellUnit, q.SellAmount); err != nil {
		return nil, err
	}
	nextSeller.Add(seller.ID, q.BuyUnit, q.BuyAmount)
	if err := nextBuyer.Sub(buyer.ID, q.BuyUnit, q.BuyAmount); err != nil {
		return nil, err
	}
	nextBuyer.Add(buyer.ID, q.SellUnit, q.SellAmount)

	seller.Inventory = nextSeller
	buyer.Inventory = nextBuyer
	recordSuccessfulTrade(seller, buyer, q, now)
	return tx, nil
}

func ExecuteSignedTrade(
	seller *Node,
	buyer *Node,
	q Quote,
	sellerSig SignedTradeIntent,
	buyerSig SignedTradeIntent,
) (*model.TradeTx, error) {
	if seller == nil || buyer == nil {
		return nil, fmt.Errorf("seller and buyer are required")
	}
	signedTradeExecutionMu.Lock()
	defer signedTradeExecutionMu.Unlock()
	unlock := lockTradeNodes(seller, buyer)
	defer unlock()
	seller.init()
	buyer.init()
	if !q.Executable {
		return nil, quoteExecutionError(q)
	}
	if seller.ID != q.Seller || buyer.ID != q.Buyer {
		return nil, fmt.Errorf("quote parties do not match nodes")
	}
	if !VerifyTradeIntent(sellerSig) {
		return nil, fmt.Errorf("invalid seller authorization")
	}
	if !VerifyTradeIntent(buyerSig) {
		return nil, fmt.Errorf("invalid buyer authorization")
	}
	if sellerSig.Intent.Timestamp <= 0 || buyerSig.Intent.Timestamp <= 0 {
		return nil, fmt.Errorf("authorization timestamp must be greater than zero")
	}
	if sellerSig.Intent.Party != seller.ID || buyerSig.Intent.Party != buyer.ID {
		return nil, fmt.Errorf("authorization party mismatch")
	}
	if !publicKeyMatchesNode(sellerSig.PublicKey, seller.ID) || !publicKeyMatchesNode(buyerSig.PublicKey, buyer.ID) {
		return nil, fmt.Errorf("authorization key mismatch")
	}
	if len(seller.PublicKey) > 0 && !samePublicKey(seller.PublicKey, sellerSig.PublicKey) {
		return nil, fmt.Errorf("seller public key mismatch")
	}
	if len(buyer.PublicKey) > 0 && !samePublicKey(buyer.PublicKey, buyerSig.PublicKey) {
		return nil, fmt.Errorf("buyer public key mismatch")
	}
	if !intentMatchesQuote(sellerSig.Intent, q, seller.ID) {
		return nil, fmt.Errorf("seller intent does not match quote")
	}
	if !intentMatchesQuote(buyerSig.Intent, q, buyer.ID) {
		return nil, fmt.Errorf("buyer intent does not match quote")
	}
	if !economicTermsMatch(sellerSig.Intent, buyerSig.Intent) {
		return nil, fmt.Errorf("authorizations do not sign the same terms")
	}

	if err := requireDurableReplayGuard(seller, buyer); err != nil {
		return nil, err
	}
	if !seller.AcceptQuote(q) {
		return nil, fmt.Errorf("seller rejected quote")
	}
	if !buyer.AcceptQuote(q) {
		return nil, fmt.Errorf("buyer rejected quote")
	}

	now := sellerSig.Intent.Timestamp
	tx, err := buildTradeTx(seller.ID, buyer.ID, q, now)
	if err != nil {
		return nil, err
	}
	auth := AuthorizedTradeTx{Tx: *tx, SellerAuth: sellerSig, BuyerAuth: buyerSig}
	authID, err := AuthorizedTradeID(auth)
	if err != nil {
		return nil, err
	}
	if seller.Store != nil && buyer.Store != nil && seller.Store != buyer.Store {
		return nil, fmt.Errorf("split-store signed execution is not supported")
	}
	if err := rejectInMemoryReplay(seller, buyer, authID); err != nil {
		return nil, err
	}
	if err := rejectReplayCheck(seller.Store, authID); err != nil {
		return nil, err
	}
	if buyer.Store != seller.Store {
		if err := rejectReplayCheck(buyer.Store, authID); err != nil {
			return nil, err
		}
	}
	combined := combinedInventory(seller, buyer)
	if err := model.ValidateTradeTx(tx, combined); err != nil {
		return nil, err
	}

	nextSeller, nextBuyer, err := preparedTradeState(seller, buyer, q, now)
	if err != nil {
		return nil, err
	}
	reserved, err := reserveInMemoryReplay(seller, buyer, authID)
	if err != nil {
		return nil, err
	}
	if err := persistSignedTrade(seller, buyer, authID, auth, nextSeller, nextBuyer); err != nil {
		rollbackInMemoryReplay(authID, reserved...)
		return nil, err
	}
	commitPreparedState(seller, nextSeller)
	commitPreparedState(buyer, nextBuyer)
	return tx, nil
}

func buildTradeTx(seller model.NodeID, buyer model.NodeID, q Quote, now int64) (*model.TradeTx, error) {
	inputSeller, err := tradeValue(q.SellUnit, q.SellAmount, seller, now)
	if err != nil {
		return nil, err
	}
	inputBuyer, err := tradeValue(q.BuyUnit, q.BuyAmount, buyer, now+1)
	if err != nil {
		return nil, err
	}
	outputSeller, err := tradeValue(q.BuyUnit, q.BuyAmount, seller, now+2)
	if err != nil {
		return nil, err
	}
	outputBuyer, err := tradeValue(q.SellUnit, q.SellAmount, buyer, now+3)
	if err != nil {
		return nil, err
	}
	tx := &model.TradeTx{
		InputsA:   []model.Value{inputSeller},
		InputsB:   []model.Value{inputBuyer},
		OutputsA:  []model.Value{outputSeller},
		OutputsB:  []model.Value{outputBuyer},
		PartyA:    seller,
		PartyB:    buyer,
		Timestamp: now,
	}
	id, err := model.TradeTxID(*tx)
	if err != nil {
		return nil, err
	}
	tx.ID = id
	return tx, nil
}

func tradeValue(unit model.UnitID, amount model.Amount, owner model.NodeID, createdAt int64) (model.Value, error) {
	value := model.Value{Unit: unit, Amount: amount, Owner: owner, CreatedAt: createdAt}
	id, err := model.ValueIDFor(value)
	if err != nil {
		return model.Value{}, err
	}
	value.ID = id
	return value, nil
}

func combinedInventory(a *Node, b *Node) model.InventoryState {
	out := model.NewInventoryState()
	for unit, amount := range a.Inventory.Holdings[a.ID] {
		out.Add(a.ID, unit, amount)
	}
	for unit, amount := range b.Inventory.Holdings[b.ID] {
		out.Add(b.ID, unit, amount)
	}
	return out
}

func recordSuccessfulTrade(seller *Node, buyer *Node, q Quote, now int64) {
	seller.recordTradeFlow(q.SellUnit, q.SellAmount)
	buyer.recordTradeFlow(q.SellUnit, q.SellAmount)
	seller.recordTradeFlow(q.BuyUnit, q.BuyAmount)
	buyer.recordTradeFlow(q.BuyUnit, q.BuyAmount)
	seller.recordPaymentFlow(q.BuyUnit, q.BuyAmount)
	buyer.recordPaymentFlow(q.BuyUnit, q.BuyAmount)

	sellUnitPrice := model.ToFloat(q.BuyAmount) / model.ToFloat(q.SellAmount)
	buyUnitPrice := model.ToFloat(q.SellAmount) / model.ToFloat(q.BuyAmount)
	seller.recordObservation(q.SellUnit, sellUnitPrice, q.SellAmount, now)
	buyer.recordObservation(q.SellUnit, sellUnitPrice, q.SellAmount, now)
	seller.recordObservation(q.BuyUnit, buyUnitPrice, q.BuyAmount, now)
	buyer.recordObservation(q.BuyUnit, buyUnitPrice, q.BuyAmount, now)
}

func preparedTradeState(seller *Node, buyer *Node, q Quote, now int64) (*Node, *Node, error) {
	nextSeller := cloneRuntimeState(seller)
	nextBuyer := cloneRuntimeState(buyer)
	if err := nextSeller.Inventory.Sub(seller.ID, q.SellUnit, q.SellAmount); err != nil {
		return nil, nil, err
	}
	nextSeller.Inventory.Add(seller.ID, q.BuyUnit, q.BuyAmount)
	if err := nextBuyer.Inventory.Sub(buyer.ID, q.BuyUnit, q.BuyAmount); err != nil {
		return nil, nil, err
	}
	nextBuyer.Inventory.Add(buyer.ID, q.SellUnit, q.SellAmount)
	recordSuccessfulTrade(nextSeller, nextBuyer, q, now)
	return nextSeller, nextBuyer, nil
}

func persistSignedTrade(seller *Node, buyer *Node, id model.TxID, auth AuthorizedTradeTx, nextSeller *Node, nextBuyer *Node) error {
	if seller.Store != nil && buyer.Store == seller.Store {
		return seller.Store.PersistExecutedTrade(
			id,
			auth,
			persistedNodeState(seller.ID, nextSeller),
			persistedNodeState(buyer.ID, nextBuyer),
		)
	}
	if seller.Store != nil {
		if err := seller.Store.PersistExecutedTrade(id, auth, persistedNodeState(seller.ID, nextSeller)); err != nil {
			return err
		}
	}
	if buyer.Store != nil && buyer.Store != seller.Store {
		if err := buyer.Store.PersistExecutedTrade(id, auth, persistedNodeState(buyer.ID, nextBuyer)); err != nil {
			return err
		}
	}
	return nil
}

func persistedNodeState(id model.NodeID, next *Node) PersistedNodeState {
	return PersistedNodeState{
		ID:         id,
		Inventory:  next.Inventory,
		Flow:       next.Flow,
		PriceState: next.PriceState,
	}
}

func cloneRuntimeState(n *Node) *Node {
	return &Node{
		ID:                         n.ID,
		PublicKey:                  append(n.PublicKey[:0:0], n.PublicKey...),
		PrivateKey:                 append(n.PrivateKey[:0:0], n.PrivateKey...),
		Inventory:                  n.Inventory.Copy(),
		Preferences:                copyFloatMap(n.Preferences),
		PriceState:                 copyPriceState(n.PriceState),
		PriceConfig:                n.PriceConfig,
		Features:                   copyFeatures(n.Features),
		TradeHistory:               copyTradeHistory(n.TradeHistory),
		SettledVolume:              copyAmountMap(n.SettledVolume),
		LastTradeUnix:              copyInt64Map(n.LastTradeUnix),
		Flow:                       copyFlow(n.Flow),
		Store:                      n.Store,
		ExecutedTrades:             copyExecutedTrades(n.ExecutedTrades),
		AllowEphemeralReplayUnsafe: n.AllowEphemeralReplayUnsafe,
		NowUnix:                    n.NowUnix,
	}
}

func commitPreparedState(dst *Node, prepared *Node) {
	dst.Inventory = prepared.Inventory
	dst.PriceState = prepared.PriceState
	dst.Flow = prepared.Flow
	dst.TradeHistory = prepared.TradeHistory
	dst.SettledVolume = prepared.SettledVolume
	dst.LastTradeUnix = prepared.LastTradeUnix
}

func rejectInMemoryReplay(seller *Node, buyer *Node, id model.TxID) error {
	if seller.ExecutedTrades[id] {
		return fmt.Errorf("trade replay rejected")
	}
	if buyer != seller && buyer.ExecutedTrades[id] {
		return fmt.Errorf("trade replay rejected")
	}
	return nil
}

func reserveInMemoryReplay(seller *Node, buyer *Node, id model.TxID) ([]*Node, error) {
	if err := rejectInMemoryReplay(seller, buyer, id); err != nil {
		return nil, err
	}
	seller.ExecutedTrades[id] = true
	reserved := []*Node{seller}
	if buyer != seller {
		buyer.ExecutedTrades[id] = true
		reserved = append(reserved, buyer)
	}
	return reserved, nil
}

func rollbackInMemoryReplay(id model.TxID, nodes ...*Node) {
	for _, n := range nodes {
		delete(n.ExecutedTrades, id)
	}
}

func requireDurableReplayGuard(seller *Node, buyer *Node) error {
	if seller.Store != nil || buyer.Store != nil {
		return nil
	}
	if seller.AllowEphemeralReplayUnsafe && buyer.AllowEphemeralReplayUnsafe {
		return nil
	}
	return fmt.Errorf("signed trade execution requires durable replay store")
}

func lockTradeNodes(a *Node, b *Node) func() {
	if a == b {
		a.mu.Lock()
		return func() { a.mu.Unlock() }
	}
	first, second := orderedNodes(a, b)
	first.mu.Lock()
	second.mu.Lock()
	return func() {
		second.mu.Unlock()
		first.mu.Unlock()
	}
}

func orderedNodes(a *Node, b *Node) (*Node, *Node) {
	if cmp := bytes.Compare(a.ID[:], b.ID[:]); cmp < 0 {
		return a, b
	} else if cmp > 0 {
		return b, a
	}
	if fmt.Sprintf("%p", a) < fmt.Sprintf("%p", b) {
		return a, b
	}
	return b, a
}

func copyPriceState(in map[model.UnitID]price.PriceResult) map[model.UnitID]price.PriceResult {
	out := make(map[model.UnitID]price.PriceResult, len(in))
	for unit, result := range in {
		out[unit] = result
	}
	return out
}

func copyFlow(in map[model.UnitID]model.FlowRecord) map[model.UnitID]model.FlowRecord {
	out := make(map[model.UnitID]model.FlowRecord, len(in))
	for unit, record := range in {
		out[unit] = record
	}
	return out
}

func copyTradeHistory(in map[model.UnitID][]price.TradeObservation) map[model.UnitID][]price.TradeObservation {
	out := make(map[model.UnitID][]price.TradeObservation, len(in))
	for unit, observations := range in {
		out[unit] = append([]price.TradeObservation(nil), observations...)
	}
	return out
}

func copyFloatMap(in map[model.UnitID]float64) map[model.UnitID]float64 {
	out := make(map[model.UnitID]float64, len(in))
	for unit, value := range in {
		out[unit] = value
	}
	return out
}

func copyFeatures(in map[model.UnitID]price.AssetFeatures) map[model.UnitID]price.AssetFeatures {
	out := make(map[model.UnitID]price.AssetFeatures, len(in))
	for unit, value := range in {
		out[unit] = value
	}
	return out
}

func copyAmountMap(in map[model.UnitID]model.Amount) map[model.UnitID]model.Amount {
	out := make(map[model.UnitID]model.Amount, len(in))
	for unit, amount := range in {
		out[unit] = amount
	}
	return out
}

func copyInt64Map(in map[model.UnitID]int64) map[model.UnitID]int64 {
	out := make(map[model.UnitID]int64, len(in))
	for unit, value := range in {
		out[unit] = value
	}
	return out
}

func copyExecutedTrades(in map[model.TxID]bool) map[model.TxID]bool {
	out := make(map[model.TxID]bool, len(in))
	for id, executed := range in {
		out[id] = executed
	}
	return out
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

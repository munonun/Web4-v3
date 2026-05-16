package node

import (
	"fmt"

	"web4-v3/core/model"
	"web4-v3/core/price"
)

func ExecuteTrade(seller *Node, buyer *Node, q Quote) (*model.TradeTx, error) {
	if seller == nil || buyer == nil {
		return nil, fmt.Errorf("seller and buyer are required")
	}
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
	if err := persistSignedTrade(seller, buyer, authID, auth, nextSeller, nextBuyer); err != nil {
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
	clone := *n
	clone.Inventory = n.Inventory.Copy()
	clone.PriceState = copyPriceState(n.PriceState)
	clone.Flow = copyFlow(n.Flow)
	clone.TradeHistory = copyTradeHistory(n.TradeHistory)
	clone.SettledVolume = copyAmountMap(n.SettledVolume)
	clone.LastTradeUnix = copyInt64Map(n.LastTradeUnix)
	return &clone
}

func commitPreparedState(dst *Node, prepared *Node) {
	dst.Inventory = prepared.Inventory
	dst.PriceState = prepared.PriceState
	dst.Flow = prepared.Flow
	dst.TradeHistory = prepared.TradeHistory
	dst.SettledVolume = prepared.SettledVolume
	dst.LastTradeUnix = prepared.LastTradeUnix
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

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

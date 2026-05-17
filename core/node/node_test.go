package node

import (
	"crypto/ed25519"
	"math"
	"sync"
	"testing"
	"time"

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

func TestZeroTimestampTradeIntentRejected(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testSignedTradeNodes(t)
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	intent := IntentFromQuote(q, seller.ID, 0)

	if _, err := SignTradeIntent(seller.PrivateKey, intent); err == nil {
		t.Fatal("zero timestamp intent signed")
	}

	sig := signedIntentUnsafe(t, seller.PrivateKey, intent)
	if VerifyTradeIntent(sig) {
		t.Fatal("zero timestamp intent verified")
	}
	_, buyerSig := signQuoteBoth(t, seller, buyer, q)
	if _, err := ExecuteSignedTrade(seller, buyer, q, sig, buyerSig); err == nil {
		t.Fatal("zero timestamp intent executed")
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

func TestExecuteSignedTradeReceiveOverflowReturnsError(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testSignedTradeNodes(t)
	replayStore := newFakeStore()
	seller.Store = replayStore
	buyer.Store = replayStore
	seller.AddInventory(buyUnit, model.Amount(math.MaxInt64)-model.FromFloat(2)+1)
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerSig, buyerSig := signQuoteBoth(t, seller, buyer, q)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ExecuteSignedTrade panicked on receive overflow: %v", r)
		}
	}()
	if _, err := ExecuteSignedTrade(seller, buyer, q, sellerSig, buyerSig); err == nil {
		t.Fatal("expected receive overflow error")
	}
	if got := seller.Balance(buyUnit); got != model.Amount(math.MaxInt64)-model.FromFloat(2)+1 {
		t.Fatalf("overflow failure mutated receive balance to %d", got)
	}
}

func TestExecuteSignedTradeLargeSafeReceiveSucceeds(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testSignedTradeNodes(t)
	replayStore := newFakeStore()
	seller.Store = replayStore
	buyer.Store = replayStore
	seller.AddInventory(buyUnit, model.Amount(math.MaxInt64)-model.FromFloat(2))
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerSig, buyerSig := signQuoteBoth(t, seller, buyer, q)

	if _, err := ExecuteSignedTrade(seller, buyer, q, sellerSig, buyerSig); err != nil {
		t.Fatalf("safe receive should execute: %v", err)
	}
	if got := seller.Balance(buyUnit); got != model.Amount(math.MaxInt64) {
		t.Fatalf("receive balance %d, want %d", got, model.Amount(math.MaxInt64))
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

func TestExecuteSignedTradeRejectsNilStoreByDefault(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testSignedTradeNodes(t)
	seller.AllowEphemeralReplayUnsafe = false
	buyer.AllowEphemeralReplayUnsafe = false
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerSig, buyerSig := signQuoteBoth(t, seller, buyer, q)

	if _, err := ExecuteSignedTrade(seller, buyer, q, sellerSig, buyerSig); err == nil {
		t.Fatal("expected nil-store signed trade rejection")
	}
	if seller.Balance(sellUnit) != model.FromFloat(10) || buyer.Balance(buyUnit) != model.FromFloat(10) {
		t.Fatalf("failed nil-store execution mutated balances seller=%d buyer=%d", seller.Balance(sellUnit), buyer.Balance(buyUnit))
	}
}

func TestExecuteSignedTradeRejectsStorelessFreshRuntimeReplayByDefault(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testSignedTradeNodes(t)
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerSig, buyerSig := signQuoteBoth(t, seller, buyer, q)
	if _, err := ExecuteSignedTrade(seller, buyer, q, sellerSig, buyerSig); err != nil {
		t.Fatalf("unsafe setup execution: %v", err)
	}
	freshSeller, freshBuyer := cloneSignedTradeParties(t, seller, buyer, sellUnit, buyUnit)
	freshSeller.AllowEphemeralReplayUnsafe = false
	freshBuyer.AllowEphemeralReplayUnsafe = false

	if _, err := ExecuteSignedTrade(freshSeller, freshBuyer, q, sellerSig, buyerSig); err == nil {
		t.Fatal("expected storeless fresh-runtime replay rejection")
	}
}

func TestExecuteSignedTradeEphemeralUnsafeRejectsReplay(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testSignedTradeNodes(t)
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerSig, buyerSig := signQuoteBoth(t, seller, buyer, q)

	if _, err := ExecuteSignedTrade(seller, buyer, q, sellerSig, buyerSig); err != nil {
		t.Fatalf("first signed trade: %v", err)
	}
	if _, err := ExecuteSignedTrade(seller, buyer, q, sellerSig, buyerSig); err == nil {
		t.Fatal("expected nil-store replay rejection")
	}
	if seller.Balance(sellUnit) != model.FromFloat(8) || buyer.Balance(buyUnit) != model.FromFloat(8) {
		t.Fatalf("replay mutated balances seller=%d buyer=%d", seller.Balance(sellUnit), buyer.Balance(buyUnit))
	}
}

func TestExecuteSignedTradeConcurrentSameTradeOnlyOnce(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testSignedTradeNodes(t)
	replayStore := newFakeStore()
	seller.Store = replayStore
	buyer.Store = replayStore
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerSig, buyerSig := signQuoteBoth(t, seller, buyer, q)

	const workers = 8
	start := make(chan struct{})
	results := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := ExecuteSignedTrade(seller, buyer, q, sellerSig, buyerSig)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful concurrent executions %d, want 1", successes)
	}
	if seller.Balance(sellUnit) != model.FromFloat(8) || buyer.Balance(buyUnit) != model.FromFloat(8) {
		t.Fatalf("bad balances seller=%d buyer=%d", seller.Balance(sellUnit), buyer.Balance(buyUnit))
	}
}

func TestExecuteSignedTradeConcurrentDifferentTradesNoCorruption(t *testing.T) {
	seller, buyer1, sellUnit, buyUnit := testSignedTradeNodes(t)
	buyer2 := testSignedNode(t, 100)
	configureUnit(t, buyer2, sellUnit, 1)
	configureUnit(t, buyer2, buyUnit, 1)
	buyer2.AddInventory(buyUnit, model.FromFloat(10))
	replayStore := newFakeStore()
	seller.Store = replayStore
	buyer1.Store = replayStore
	buyer2.Store = replayStore
	q1 := seller.QuoteSell(buyer1, sellUnit, buyUnit, model.FromFloat(2), 0)
	q2 := seller.QuoteSell(buyer2, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerSig1, buyerSig1 := signQuoteBoth(t, seller, buyer1, q1)
	sellerSig2, buyerSig2 := signQuoteBoth(t, seller, buyer2, q2)

	errCh := make(chan error, 2)
	done := make(chan struct{})
	go func() {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, err := ExecuteSignedTrade(seller, buyer1, q1, sellerSig1, buyerSig1)
			errCh <- err
		}()
		go func() {
			defer wg.Done()
			_, err := ExecuteSignedTrade(seller, buyer2, q2, sellerSig2, buyerSig2)
			errCh <- err
		}()
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("concurrent signed trades deadlocked")
	}
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent different trade failed: %v", err)
		}
	}
	if seller.Balance(sellUnit) != model.FromFloat(6) {
		t.Fatalf("seller sell balance %d, want %d", seller.Balance(sellUnit), model.FromFloat(6))
	}
	if buyer1.Balance(sellUnit) != model.FromFloat(2) || buyer2.Balance(sellUnit) != model.FromFloat(2) {
		t.Fatalf("buyer balances buyer1=%d buyer2=%d", buyer1.Balance(sellUnit), buyer2.Balance(sellUnit))
	}
}

func TestSignQuotePriceBalanceConcurrentWithSignedExecutionNoRace(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testSignedTradeNodes(t)
	replayStore := newFakeStore()
	seller.Store = replayStore
	buyer.Store = replayStore
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerSig, buyerSig := signQuoteBoth(t, seller, buyer, q)

	start := make(chan struct{})
	done := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		<-start
		for {
			select {
			case <-done:
				return
			default:
				_, _ = seller.SignQuote(q)
				_ = seller.Price(sellUnit)
				_ = seller.Balance(sellUnit)
			}
		}
	}()
	go func() {
		<-start
		_, err := ExecuteSignedTrade(seller, buyer, q, sellerSig, buyerSig)
		errCh <- err
	}()
	close(start)

	select {
	case err := <-errCh:
		close(done)
		if err != nil {
			t.Fatalf("execute signed trade: %v", err)
		}
	case <-time.After(time.Second):
		close(done)
		t.Fatal("concurrent quote/sign/read test deadlocked")
	}
}

func TestSignedTradeReplayUsesStableAuthorizedTradeID(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testSignedTradeNodes(t)
	replayStore := newFakeStore()
	seller.Store = replayStore
	buyer.Store = replayStore
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerSig, buyerSig := signQuoteBoth(t, seller, buyer, q)

	tx, err := ExecuteSignedTrade(seller, buyer, q, sellerSig, buyerSig)
	if err != nil {
		t.Fatalf("first signed trade: %v", err)
	}
	auth := AuthorizedTradeTx{Tx: *tx, SellerAuth: sellerSig, BuyerAuth: buyerSig}
	authID, err := AuthorizedTradeID(auth)
	if err != nil {
		t.Fatalf("auth id: %v", err)
	}
	if !replayStore.HasExecutedTrade(authID) {
		t.Fatal("stable authorized trade ID was not marked")
	}

	seller.NowUnix = func() int64 { return 999 }
	buyer.NowUnix = func() int64 { return 1000 }
	if _, err := ExecuteSignedTrade(seller, buyer, q, sellerSig, buyerSig); err == nil {
		t.Fatal("expected replay rejection")
	}

	freshSeller, freshBuyer := cloneSignedTradeParties(t, seller, buyer, sellUnit, buyUnit)
	freshSeller.NowUnix = func() int64 { return 5000 }
	freshBuyer.NowUnix = func() int64 { return 6000 }
	replayedTx, err := ExecuteSignedTrade(freshSeller, freshBuyer, q, sellerSig, buyerSig)
	if err != nil {
		t.Fatalf("same signed terms should execute on fresh runtime: %v", err)
	}
	replayedAuthID, err := AuthorizedTradeID(AuthorizedTradeTx{Tx: *replayedTx, SellerAuth: sellerSig, BuyerAuth: buyerSig})
	if err != nil {
		t.Fatalf("replayed auth id: %v", err)
	}
	if replayedAuthID != authID {
		t.Fatal("same signed trade produced different AuthorizedTradeID")
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
	failing.failSaveInventory = false
	if _, err := ExecuteSignedTrade(seller, buyer, q, sellerSig, buyerSig); err != nil {
		t.Fatalf("retry after persistence failure: %v", err)
	}
	if seller.Balance(sellUnit) != model.FromFloat(8) || buyer.Balance(buyUnit) != model.FromFloat(8) {
		t.Fatalf("retry did not commit expected balances seller=%d buyer=%d", seller.Balance(sellUnit), buyer.Balance(buyUnit))
	}
}

func TestExecuteSignedTradePartialPersistenceRejectsReplay(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testSignedTradeNodes(t)
	failing := newFakeStore()
	failing.failAfterMark = true
	seller.Store = failing
	buyer.Store = failing
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerSig, buyerSig := signQuoteBoth(t, seller, buyer, q)

	tx, err := buildTradeTx(seller.ID, buyer.ID, q, q.Timestamp)
	if err != nil {
		t.Fatalf("build trade: %v", err)
	}
	authID, err := AuthorizedTradeID(AuthorizedTradeTx{Tx: *tx, SellerAuth: sellerSig, BuyerAuth: buyerSig})
	if err != nil {
		t.Fatalf("auth id: %v", err)
	}

	if _, err := ExecuteSignedTrade(seller, buyer, q, sellerSig, buyerSig); err == nil {
		t.Fatal("expected partial persistence failure")
	}
	if seller.Balance(sellUnit) != model.FromFloat(10) || buyer.Balance(buyUnit) != model.FromFloat(10) {
		t.Fatalf("failed partial persistence mutated balances seller=%d buyer=%d", seller.Balance(sellUnit), buyer.Balance(buyUnit))
	}
	if !failing.HasExecutedTrade(authID) {
		t.Fatal("partial failure did not leave replay marker")
	}
	if _, err := ExecuteSignedTrade(seller, buyer, q, sellerSig, buyerSig); err == nil {
		t.Fatal("expected replay rejection after partial failure")
	}
}

func TestExecuteSignedTradeRejectsSplitStoresBeforePersistence(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testSignedTradeNodes(t)
	sellerStore := newFakeStore()
	buyerStore := newFakeStore()
	seller.Store = sellerStore
	buyer.Store = buyerStore
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerSig, buyerSig := signQuoteBoth(t, seller, buyer, q)

	if _, err := ExecuteSignedTrade(seller, buyer, q, sellerSig, buyerSig); err == nil {
		t.Fatal("expected split-store rejection")
	}
	if sellerStore.markedCount != 0 || buyerStore.markedCount != 0 {
		t.Fatalf("split-store rejection persisted seller=%d buyer=%d", sellerStore.markedCount, buyerStore.markedCount)
	}
	if seller.Balance(sellUnit) != model.FromFloat(10) || buyer.Balance(buyUnit) != model.FromFloat(10) {
		t.Fatalf("split-store rejection mutated balances seller=%d buyer=%d", seller.Balance(sellUnit), buyer.Balance(buyUnit))
	}
}

func TestExecuteSignedTradeRejectsOneSidedAuthoritativeStore(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testSignedTradeNodes(t)
	seller.Store = newFakeStore()
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerSig, buyerSig := signQuoteBoth(t, seller, buyer, q)

	if _, err := ExecuteSignedTrade(seller, buyer, q, sellerSig, buyerSig); err == nil {
		t.Fatal("expected one-sided authoritative store rejection")
	}
	if seller.Balance(sellUnit) != model.FromFloat(10) || buyer.Balance(buyUnit) != model.FromFloat(10) {
		t.Fatalf("one-sided rejection mutated balances seller=%d buyer=%d", seller.Balance(sellUnit), buyer.Balance(buyUnit))
	}
}

func TestExecuteSignedTradeWithPeerShadowAllowsOneSidedStore(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testSignedTradeNodes(t)
	seller.Store = newFakeStore()
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerSig, buyerSig := signQuoteBoth(t, seller, buyer, q)

	if _, err := ExecuteSignedTradeWithPeerShadow(seller, buyer, q, sellerSig, buyerSig); err != nil {
		t.Fatalf("peer-shadow signed execution: %v", err)
	}
	if seller.Balance(sellUnit) != model.FromFloat(8) || buyer.Balance(buyUnit) != model.FromFloat(8) {
		t.Fatalf("peer-shadow execution balances seller=%d buyer=%d", seller.Balance(sellUnit), buyer.Balance(buyUnit))
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
	n.AllowEphemeralReplayUnsafe = true
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

func signedIntentUnsafe(t *testing.T, priv crypto.PrivateKey, intent TradeIntent) SignedTradeIntent {
	t.Helper()
	pub, ok := ed25519.PrivateKey(priv).Public().(ed25519.PublicKey)
	if !ok {
		t.Fatalf("private key public component has unexpected type")
	}
	preimage, err := tradeIntentPreimage(intent)
	if err != nil {
		t.Fatalf("intent preimage: %v", err)
	}
	sig, err := crypto.Sign(priv, preimage)
	if err != nil {
		t.Fatalf("sign unsafe intent: %v", err)
	}
	return SignedTradeIntent{Intent: intent, PublicKey: append(crypto.PublicKey(nil), pub...), Signature: sig}
}

func cloneSignedTradeParties(t *testing.T, seller *Node, buyer *Node, sellUnit model.UnitID, buyUnit model.UnitID) (*Node, *Node) {
	t.Helper()
	freshSeller, err := NewNode(seller.PrivateKey, seller.PriceConfig)
	if err != nil {
		t.Fatalf("new fresh seller: %v", err)
	}
	freshBuyer, err := NewNode(buyer.PrivateKey, buyer.PriceConfig)
	if err != nil {
		t.Fatalf("new fresh buyer: %v", err)
	}
	freshSeller.AllowEphemeralReplayUnsafe = seller.AllowEphemeralReplayUnsafe
	freshBuyer.AllowEphemeralReplayUnsafe = buyer.AllowEphemeralReplayUnsafe
	configureUnit(t, freshSeller, sellUnit, 1)
	configureUnit(t, freshSeller, buyUnit, 1)
	configureUnit(t, freshBuyer, sellUnit, 1)
	configureUnit(t, freshBuyer, buyUnit, 1)
	freshSeller.AddInventory(sellUnit, model.FromFloat(10))
	freshBuyer.AddInventory(buyUnit, model.FromFloat(10))
	return freshSeller, freshBuyer
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
	mu                sync.Mutex
	executed          map[model.TxID]bool
	inventory         map[model.NodeID]model.InventoryState
	flow              map[model.NodeID]map[model.UnitID]model.FlowRecord
	prices            map[model.NodeID]map[model.UnitID]price.PriceResult
	trades            map[model.TxID]AuthorizedTradeTx
	failSaveInventory bool
	failAfterMark     bool
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

func (s *fakeStore) HasExecutedTrade(id model.TxID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.executed[id]
}
func (s *fakeStore) MarkExecutedTrade(id model.TxID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executed[id] = true
	s.markedCount++
	return nil
}
func (s *fakeStore) SaveInventory(id model.NodeID, inv model.InventoryState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failSaveInventory {
		return errFakeStore
	}
	s.inventory[id] = inv.Copy()
	return nil
}
func (s *fakeStore) LoadInventory(id model.NodeID) (model.InventoryState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if inv, ok := s.inventory[id]; ok {
		return inv.Copy(), nil
	}
	return model.NewInventoryState(), nil
}
func (s *fakeStore) SaveFlow(id model.NodeID, flow map[model.UnitID]model.FlowRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flow[id] = copyFlow(flow)
	return nil
}
func (s *fakeStore) LoadFlow(id model.NodeID) (map[model.UnitID]model.FlowRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyFlow(s.flow[id]), nil
}
func (s *fakeStore) SavePriceState(id model.NodeID, state map[model.UnitID]price.PriceResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prices[id] = copyPriceState(state)
	return nil
}
func (s *fakeStore) LoadPriceState(id model.NodeID) (map[model.UnitID]price.PriceResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyPriceState(s.prices[id]), nil
}
func (s *fakeStore) SaveAuthorizedTrade(id model.TxID, tx AuthorizedTradeTx) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trades[id] = tx
	return nil
}
func (s *fakeStore) LoadAuthorizedTrade(id model.TxID) (AuthorizedTradeTx, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, ok := s.trades[id]
	return tx, ok
}
func (s *fakeStore) PersistExecutedTrade(id model.TxID, tx AuthorizedTradeTx, states ...PersistedNodeState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.executed[id] {
		return errReplay
	}
	if s.failSaveInventory {
		return errFakeStore
	}
	s.trades[id] = tx
	for _, state := range states {
		s.inventory[state.ID] = state.Inventory.Copy()
		s.flow[state.ID] = copyFlow(state.Flow)
		s.prices[state.ID] = copyPriceState(state.PriceState)
	}
	s.executed[id] = true
	s.markedCount++
	if s.failAfterMark {
		return errFakeStore
	}
	return nil
}
func (s *fakeStore) Close() error { return nil }

var errFakeStore = &fakeStoreError{}
var errReplay = &replayError{}

type fakeStoreError struct{}

func (*fakeStoreError) Error() string { return "fake store failure" }

type replayError struct{}

func (*replayError) Error() string { return "trade replay rejected" }

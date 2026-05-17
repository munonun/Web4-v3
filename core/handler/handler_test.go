package handler

import (
	"errors"
	"testing"

	"web4-v3/core/crypto"
	"web4-v3/core/message"
	"web4-v3/core/model"
	"web4-v3/core/node"
	"web4-v3/core/price"
)

func TestSessionSeenAndPendingStorage(t *testing.T) {
	_, _, localID := testKey(t)
	_, _, peerID := testKey(t)
	s := NewSession(localID, peerID)
	id := testTxID(1)

	if s.HasSeen(id) {
		t.Fatal("message unexpectedly seen")
	}
	if err := s.MarkSeen(id); err != nil {
		t.Fatalf("mark seen: %v", err)
	}
	if !s.HasSeen(id) {
		t.Fatal("message not marked seen")
	}
	if err := s.MarkSeen(id); !errors.Is(err, ErrDuplicateMessage) {
		t.Fatalf("duplicate error %v, want ErrDuplicateMessage", err)
	}

	req := message.QuoteRequestPayload{RequestID: testTxID(2)}
	s.PendingRequests[req.RequestID] = req
	if s.PendingRequests[req.RequestID].RequestID != req.RequestID {
		t.Fatal("pending request not stored")
	}
}

func TestNewHandlerValidation(t *testing.T) {
	n := testNode(t, 100)
	peer := testNode(t, 100)
	if _, err := NewHandler(nil, NewSession(n.ID, peer.ID)); err == nil {
		t.Fatal("nil node accepted")
	}
	if _, err := NewHandler(n, nil); err == nil {
		t.Fatal("nil session accepted")
	}
	if _, err := NewHandler(n, NewSession(peer.ID, peer.ID)); err == nil {
		t.Fatal("session local mismatch accepted")
	}
}

func TestHandleMessageRejectsInvalidEnvelopeAndDuplicate(t *testing.T) {
	local := testNode(t, 100)
	peer := testNode(t, 100)
	h := testHandler(t, local, peer)
	msg := signedMessage(t, peer.PrivateKey, message.TypePing, message.PingPayload{TimeUnix: 10}, 10, testNonce(1))
	bad := msg
	bad.Envelope.Timestamp = 0
	if _, err := h.HandleMessage(bad, peer.PublicKey); !errors.Is(err, ErrInvalidPeer) {
		t.Fatalf("invalid envelope error %v, want ErrInvalidPeer", err)
	}
	if _, err := h.HandleMessage(msg, peer.PublicKey); err != nil {
		t.Fatalf("first ping: %v", err)
	}
	if _, err := h.HandleMessage(msg, peer.PublicKey); !errors.Is(err, ErrDuplicateMessage) {
		t.Fatalf("duplicate error %v, want ErrDuplicateMessage", err)
	}
}

func TestAuthenticatedInvalidMessageReplayMarkedSeen(t *testing.T) {
	local := testNode(t, 100)
	peer := testNode(t, 100)
	h := testHandler(t, local, peer)
	payload := message.SignedIntentPayload{QuoteID: testTxID(44), Intent: node.SignedTradeIntent{}}
	msg := signedMessage(t, peer.PrivateKey, message.TypeSignedIntent, payload, 10, testNonce(44))

	if _, err := h.HandleMessage(msg, peer.PublicKey); !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("first invalid message error %v, want ErrInvalidPayload", err)
	}
	if _, err := h.HandleMessage(msg, peer.PublicKey); !errors.Is(err, ErrDuplicateMessage) {
		t.Fatalf("replayed invalid message error %v, want ErrDuplicateMessage", err)
	}
}

func TestUnauthenticatedInvalidMessageNotMarkedSeen(t *testing.T) {
	local := testNode(t, 100)
	peer := testNode(t, 100)
	h := testHandler(t, local, peer)
	msg := signedMessage(t, peer.PrivateKey, message.TypePing, message.PingPayload{TimeUnix: 10}, 10, testNonce(45))
	msg.Envelope.Timestamp = 0

	if _, err := h.HandleMessage(msg, peer.PublicKey); !errors.Is(err, ErrInvalidPeer) {
		t.Fatalf("invalid envelope error %v, want ErrInvalidPeer", err)
	}
	if h.Session.HasSeen(msg.Envelope.MessageID) {
		t.Fatal("unauthenticated invalid message was marked seen")
	}
}

func TestQuoteRequestReturnsSignedResponseWithoutExecution(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testTradeNodes(t)
	h := testHandler(t, seller, buyer)
	beforeSeller := seller.Balance(sellUnit)
	req := quoteRequest(seller, buyer, sellUnit, buyUnit)
	msg := signedMessage(t, buyer.PrivateKey, message.TypeQuoteRequest, req, 10, testNonce(2))

	resp, err := h.HandleMessage(msg, buyer.PublicKey)
	if err != nil {
		t.Fatalf("handle quote request: %v", err)
	}
	if resp == nil || resp.Envelope.Type != message.TypeQuoteResponse {
		t.Fatalf("response %+v, want quote response", resp)
	}
	if err := message.VerifyEnvelope(resp.Envelope, resp.PayloadBytes, seller.PublicKey); err != nil {
		t.Fatalf("verify response: %v", err)
	}
	payload := decodePayload[message.QuoteResponsePayload](t, message.TypeQuoteResponse, resp.PayloadBytes)
	if payload.RequestID != req.RequestID {
		t.Fatal("quote response did not reference request")
	}
	if seller.Balance(sellUnit) != beforeSeller {
		t.Fatal("quote request executed trade")
	}
}

func TestQuoteRequestInvalidRoleReturnsReject(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testTradeNodes(t)
	h := testHandler(t, seller, buyer)
	req := quoteRequest(seller, buyer, sellUnit, buyUnit)
	outsider := testNode(t, 100)
	req.Seller = outsider.ID
	msg := signedMessage(t, buyer.PrivateKey, message.TypeQuoteRequest, req, 10, testNonce(3))

	resp, err := h.HandleMessage(msg, buyer.PublicKey)
	if err != nil {
		t.Fatalf("handle invalid role: %v", err)
	}
	if resp == nil || resp.Envelope.Type != message.TypeReject {
		t.Fatalf("response %+v, want reject", resp)
	}
}

func TestQuoteResponseUnexpectedRejected(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testTradeNodes(t)
	h := testHandler(t, buyer, seller)
	q := executableQuoteResponse(t, seller, buyer, sellUnit, buyUnit, testTxID(9))
	msg := signedMessage(t, seller.PrivateKey, message.TypeQuoteResponse, q, 10, testNonce(4))

	resp, err := h.HandleMessage(msg, seller.PublicKey)
	if err != nil {
		t.Fatalf("handle unexpected quote response: %v", err)
	}
	if resp == nil || resp.Envelope.Type != message.TypeReject {
		t.Fatalf("response %+v, want reject", resp)
	}
}

func TestQuoteResponseStoredAndExecutableReturnsSignedIntent(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testTradeNodes(t)
	h := testHandler(t, buyer, seller)
	req := quoteRequest(seller, buyer, sellUnit, buyUnit)
	h.Session.PendingRequests[req.RequestID] = req
	q := executableQuoteResponse(t, seller, buyer, sellUnit, buyUnit, req.RequestID)
	msg := signedMessage(t, seller.PrivateKey, message.TypeQuoteResponse, q, 10, testNonce(5))

	resp, err := h.HandleMessage(msg, seller.PublicKey)
	if err != nil {
		t.Fatalf("handle quote response: %v", err)
	}
	if _, ok := h.Session.PendingQuotes[q.QuoteID]; !ok {
		t.Fatal("quote response not stored")
	}
	if resp == nil || resp.Envelope.Type != message.TypeSignedIntent {
		t.Fatalf("response %+v, want signed intent", resp)
	}
	payload := decodePayload[message.SignedIntentPayload](t, message.TypeSignedIntent, resp.PayloadBytes)
	if payload.QuoteID != q.QuoteID || payload.Intent.Intent.Party != buyer.ID {
		t.Fatalf("bad signed intent payload: %+v", payload)
	}
}

func TestQuoteResponseConstraintViolationsRejected(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(req *message.QuoteRequestPayload, q *message.QuoteResponsePayload)
	}{
		{
			name: "expiry extension",
			mutate: func(req *message.QuoteRequestPayload, q *message.QuoteResponsePayload) {
				req.ExpiryUnix = 100
				q.ExpiryUnix = 101
			},
		},
		{
			name: "request expired",
			mutate: func(req *message.QuoteRequestPayload, q *message.QuoteResponsePayload) {
				req.ExpiryUnix = 99
				q.ExpiryUnix = 99
			},
		},
		{
			name: "spread limit bypass",
			mutate: func(req *message.QuoteRequestPayload, q *message.QuoteResponsePayload) {
				req.SpreadLimit = 0
				q.SellerAsk = 2
				q.BuyerBid = 3
			},
		},
		{
			name: "mismatched quote id",
			mutate: func(req *message.QuoteRequestPayload, q *message.QuoteResponsePayload) {
				q.BuyAmount = model.FromFloat(3)
			},
		},
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			seller, buyer, sellUnit, buyUnit := testTradeNodes(t)
			h := testHandler(t, buyer, seller)
			req := quoteRequest(seller, buyer, sellUnit, buyUnit)
			req.RequestID = testTxID(byte(60 + i))
			q := executableQuoteResponse(t, seller, buyer, sellUnit, buyUnit, req.RequestID)
			tc.mutate(&req, &q)
			if tc.name != "mismatched quote id" {
				setQuoteID(t, &q)
			}
			h.Session.PendingRequests[req.RequestID] = req
			msg := signedMessage(t, seller.PrivateKey, message.TypeQuoteResponse, q, 10, testNonce(byte(60+i)))

			resp, err := h.HandleMessage(msg, seller.PublicKey)
			if err != nil {
				t.Fatalf("handle quote response: %v", err)
			}
			if resp == nil || resp.Envelope.Type != message.TypeReject {
				t.Fatalf("response %+v, want reject", resp)
			}
			if _, ok := h.Session.PendingQuotes[q.QuoteID]; ok {
				t.Fatal("invalid quote response was stored")
			}
		})
	}
}

func TestManuallyConstructedSessionInitializesMaps(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testTradeNodes(t)
	s := &Session{LocalID: seller.ID, PeerID: buyer.ID}
	h, err := NewHandler(seller, s)
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	req := quoteRequest(seller, buyer, sellUnit, buyUnit)
	msg := signedMessage(t, buyer.PrivateKey, message.TypeQuoteRequest, req, 10, testNonce(73))
	resp, err := h.HandleMessage(msg, buyer.PublicKey)
	if err != nil {
		t.Fatalf("handle quote request: %v", err)
	}
	if resp == nil || resp.Envelope.Type != message.TypeQuoteResponse {
		t.Fatalf("response %+v, want quote response", resp)
	}
}

func TestSignedIntentValidationAndAuthorizedTrade(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testTradeNodes(t)
	h := testHandler(t, seller, buyer)
	req := quoteRequest(seller, buyer, sellUnit, buyUnit)
	q := executableQuoteResponse(t, seller, buyer, sellUnit, buyUnit, req.RequestID)
	h.Session.PendingQuotes[q.QuoteID] = q
	quote := quoteFromResponse(q)
	buyerIntent, err := buyer.SignQuote(quote)
	if err != nil {
		t.Fatalf("buyer sign quote: %v", err)
	}
	msg := signedMessage(t, buyer.PrivateKey, message.TypeSignedIntent, message.SignedIntentPayload{QuoteID: q.QuoteID, Intent: buyerIntent}, 10, testNonce(6))

	resp, err := h.HandleMessage(msg, buyer.PublicKey)
	if err != nil {
		t.Fatalf("handle signed intent: %v", err)
	}
	if resp == nil || resp.Envelope.Type != message.TypeAuthorizedTrade {
		t.Fatalf("response %+v, want authorized trade", resp)
	}
	payload := decodePayload[message.AuthorizedTradePayload](t, message.TypeAuthorizedTrade, resp.PayloadBytes)
	if payload.AuthorizedTradeID == (model.TxID{}) {
		t.Fatal("missing authorized trade id")
	}

	dup := signedMessage(t, buyer.PrivateKey, message.TypeSignedIntent, message.SignedIntentPayload{QuoteID: q.QuoteID, Intent: buyerIntent}, 11, testNonce(7))
	reject, err := h.HandleMessage(dup, buyer.PublicKey)
	if err != nil {
		t.Fatalf("duplicate intent handle error: %v", err)
	}
	if reject == nil || reject.Envelope.Type != message.TypeReject {
		t.Fatalf("duplicate intent response %+v, want reject", reject)
	}
}

func TestSignedIntentMismatchedQuoteRejected(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testTradeNodes(t)
	h := testHandler(t, seller, buyer)
	req := quoteRequest(seller, buyer, sellUnit, buyUnit)
	q := executableQuoteResponse(t, seller, buyer, sellUnit, buyUnit, req.RequestID)
	h.Session.PendingQuotes[q.QuoteID] = q
	badQuote := quoteFromResponse(q)
	badQuote.BuyAmount++
	intent, err := buyer.SignQuote(badQuote)
	if err != nil {
		t.Fatalf("buyer sign bad quote: %v", err)
	}
	msg := signedMessage(t, buyer.PrivateKey, message.TypeSignedIntent, message.SignedIntentPayload{QuoteID: q.QuoteID, Intent: intent}, 10, testNonce(8))

	resp, err := h.HandleMessage(msg, buyer.PublicKey)
	if err != nil {
		t.Fatalf("handle mismatched intent: %v", err)
	}
	if resp == nil || resp.Envelope.Type != message.TypeReject {
		t.Fatalf("response %+v, want reject", resp)
	}
}

func TestAuthorizedTradeExecutesAndReplayRejectedByStore(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testTradeNodes(t)
	seller.Store = newFakeStore()
	h := testHandler(t, seller, buyer)
	quote, authPayload := negotiatedAuthorizedTradePayload(t, h, seller, buyer, sellUnit, buyUnit)
	msg := signedMessage(t, buyer.PrivateKey, message.TypeAuthorizedTrade, authPayload, 10, testNonce(9))
	beforeSeller := seller.Balance(sellUnit)

	resp, err := h.HandleMessage(msg, buyer.PublicKey)
	if err != nil {
		t.Fatalf("handle authorized trade: %v", err)
	}
	if resp == nil || resp.Envelope.Type != message.TypeTradeResult {
		t.Fatalf("response %+v, want trade result", resp)
	}
	result := decodePayload[message.TradeResultPayload](t, message.TypeTradeResult, resp.PayloadBytes)
	if result.Status != message.TradeResultStatusAccepted {
		t.Fatalf("trade result %+v, want accepted", result)
	}
	if seller.Balance(sellUnit) != beforeSeller-model.FromFloat(2) {
		t.Fatalf("seller balance %d, want decremented", seller.Balance(sellUnit))
	}
	if _, ok := h.Session.PendingQuotes[quote.QuoteID]; ok {
		t.Fatal("pending quote was not cleared")
	}
	if _, ok := h.Session.PendingIntents[quote.QuoteID]; ok {
		t.Fatal("pending intents were not cleared")
	}
	if _, ok := h.Session.PendingTrades[authPayload.AuthorizedTradeID]; ok {
		t.Fatal("pending trade was not cleared")
	}

	replay := signedMessage(t, buyer.PrivateKey, message.TypeAuthorizedTrade, authPayload, 11, testNonce(10))
	replayResp, err := h.HandleMessage(replay, buyer.PublicKey)
	if !errors.Is(err, ErrUnexpectedMessage) {
		t.Fatalf("replay error %v, want ErrUnexpectedMessage", err)
	}
	if replayResp != nil {
		t.Fatalf("replay response %+v, want nil", replayResp)
	}
	if seller.Balance(sellUnit) != beforeSeller-model.FromFloat(2) {
		t.Fatal("replay changed inventory")
	}
}

func TestAuthorizedTradeTamperedRejectedAndFailedExecutionReturnsResult(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testTradeNodes(t)
	h := testHandler(t, seller, buyer)
	_, authPayload := negotiatedAuthorizedTradePayload(t, h, seller, buyer, sellUnit, buyUnit)
	tampered := authPayload
	tampered.AuthorizedTradeID = testTxID(99)
	tamperedMsg := signedMessage(t, buyer.PrivateKey, message.TypeAuthorizedTrade, tampered, 10, testNonce(11))
	if _, err := h.HandleMessage(tamperedMsg, buyer.PublicKey); !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("tampered authorized trade error %v, want ErrInvalidPayload", err)
	}

	if err := seller.SubInventory(sellUnit, model.FromFloat(9)); err != nil {
		t.Fatalf("prepare insufficient inventory: %v", err)
	}
	msg := signedMessage(t, buyer.PrivateKey, message.TypeAuthorizedTrade, authPayload, 10, testNonce(12))
	resp, err := h.HandleMessage(msg, buyer.PublicKey)
	if !errors.Is(err, ErrTradeExecutionFailed) {
		t.Fatalf("execution error %v, want ErrTradeExecutionFailed", err)
	}
	if resp == nil {
		t.Fatal("expected failure trade result")
	}
	result := decodePayload[message.TradeResultPayload](t, message.TypeTradeResult, resp.PayloadBytes)
	if result.Status != message.TradeResultStatusFailed {
		t.Fatalf("result %+v, want failed", result)
	}
	if seller.Balance(sellUnit) != model.FromFloat(1) {
		t.Fatal("failed execution mutated inventory")
	}
}

func TestAuthorizedTradeRelayerRejectedWithoutInventoryMutation(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testTradeNodes(t)
	relayer := testNode(t, 100)
	h := testHandler(t, seller, relayer)
	authPayload := authorizedTradePayload(t, seller, buyer, sellUnit, buyUnit)
	msg := signedMessage(t, relayer.PrivateKey, message.TypeAuthorizedTrade, authPayload, 10, testNonce(15))
	beforeSeller := seller.Balance(sellUnit)

	resp, err := h.HandleMessage(msg, relayer.PublicKey)
	if !errors.Is(err, ErrInvalidPeer) {
		t.Fatalf("relay error %v, want ErrInvalidPeer", err)
	}
	if resp != nil {
		t.Fatalf("relay response %+v, want nil", resp)
	}
	if seller.Balance(sellUnit) != beforeSeller {
		t.Fatal("relay changed inventory")
	}
}

func TestAuthorizedTradeWithoutNegotiatedStateRejected(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testTradeNodes(t)
	h := testHandler(t, seller, buyer)
	authPayload := authorizedTradePayload(t, seller, buyer, sellUnit, buyUnit)
	msg := signedMessage(t, buyer.PrivateKey, message.TypeAuthorizedTrade, authPayload, 10, testNonce(16))
	beforeSeller := seller.Balance(sellUnit)

	resp, err := h.HandleMessage(msg, buyer.PublicKey)
	if !errors.Is(err, ErrUnexpectedMessage) {
		t.Fatalf("error %v, want ErrUnexpectedMessage", err)
	}
	if resp != nil {
		t.Fatalf("response %+v, want nil", resp)
	}
	if seller.Balance(sellUnit) != beforeSeller {
		t.Fatal("unexpected authorized trade changed inventory")
	}
}

func TestAuthorizedTradeWrongSessionPeerRejectedEvenWithInjectedState(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testTradeNodes(t)
	outsider := testNode(t, 100)
	h := testHandler(t, seller, outsider)
	_, authPayload := negotiatedAuthorizedTradePayload(t, h, seller, buyer, sellUnit, buyUnit)
	msg := signedMessage(t, outsider.PrivateKey, message.TypeAuthorizedTrade, authPayload, 10, testNonce(17))
	beforeSeller := seller.Balance(sellUnit)

	resp, err := h.HandleMessage(msg, outsider.PublicKey)
	if !errors.Is(err, ErrInvalidPeer) {
		t.Fatalf("error %v, want ErrInvalidPeer", err)
	}
	if resp != nil {
		t.Fatalf("response %+v, want nil", resp)
	}
	if seller.Balance(sellUnit) != beforeSeller {
		t.Fatal("wrong peer trade changed inventory")
	}
}

func TestAuthorizedTradeMismatchedQuoteTermsRejected(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testTradeNodes(t)
	h := testHandler(t, seller, buyer)
	q := executableQuoteResponse(t, seller, buyer, sellUnit, buyUnit, testTxID(41))
	h.Session.PendingQuotes[q.QuoteID] = q
	quote := quoteFromResponse(q)
	localIntent, err := seller.SignQuote(quote)
	if err != nil {
		t.Fatalf("seller sign quote: %v", err)
	}
	h.Session.PendingIntents[q.QuoteID] = []node.SignedTradeIntent{localIntent}

	badQuote := quote
	badQuote.BuyAmount++
	sellerSig, err := seller.SignQuote(badQuote)
	if err != nil {
		t.Fatalf("seller sign bad quote: %v", err)
	}
	buyerSig, err := buyer.SignQuote(badQuote)
	if err != nil {
		t.Fatalf("buyer sign bad quote: %v", err)
	}
	auth, authID, err := buildAuthorizedTrade(badQuote, sellerSig, buyerSig)
	if err != nil {
		t.Fatalf("build bad authorized trade: %v", err)
	}
	payload := message.AuthorizedTradePayload{AuthorizedTrade: auth, AuthorizedTradeID: authID}
	msg := signedMessage(t, buyer.PrivateKey, message.TypeAuthorizedTrade, payload, 10, testNonce(18))
	beforeSeller := seller.Balance(sellUnit)

	resp, err := h.HandleMessage(msg, buyer.PublicKey)
	if !errors.Is(err, ErrUnexpectedMessage) {
		t.Fatalf("error %v, want ErrUnexpectedMessage", err)
	}
	if resp != nil {
		t.Fatalf("response %+v, want nil", resp)
	}
	if seller.Balance(sellUnit) != beforeSeller {
		t.Fatal("mismatched trade changed inventory")
	}
}

func TestAuthorizedTradeCrossSessionNegotiationInjectionRejected(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testTradeNodes(t)
	outsider := testNode(t, 100)
	h := testHandler(t, seller, buyer)
	crossQuote, crossPayload := authorizedTradePayloadForParties(t, seller, outsider, sellUnit, buyUnit)
	h.Session.PendingQuotes[crossQuote.QuoteID] = crossQuote
	h.Session.PendingIntents[crossQuote.QuoteID] = []node.SignedTradeIntent{
		crossPayload.AuthorizedTrade.SellerAuth,
		crossPayload.AuthorizedTrade.BuyerAuth,
	}
	h.Session.PendingTrades[crossPayload.AuthorizedTradeID] = crossPayload.AuthorizedTrade
	msg := signedMessage(t, buyer.PrivateKey, message.TypeAuthorizedTrade, crossPayload, 10, testNonce(19))
	beforeSeller := seller.Balance(sellUnit)

	resp, err := h.HandleMessage(msg, buyer.PublicKey)
	if !errors.Is(err, ErrInvalidPeer) {
		t.Fatalf("error %v, want ErrInvalidPeer", err)
	}
	if resp != nil {
		t.Fatalf("response %+v, want nil", resp)
	}
	if seller.Balance(sellUnit) != beforeSeller {
		t.Fatal("cross-session trade changed inventory")
	}
}

func TestPingPong(t *testing.T) {
	local := testNode(t, 100)
	peer := testNode(t, 200)
	h := testHandler(t, local, peer)
	msg := signedMessage(t, peer.PrivateKey, message.TypePing, message.PingPayload{TimeUnix: 200}, 200, testNonce(13))

	resp, err := h.HandleMessage(msg, peer.PublicKey)
	if err != nil {
		t.Fatalf("handle ping: %v", err)
	}
	if resp == nil || resp.Envelope.Type != message.TypePong {
		t.Fatalf("response %+v, want pong", resp)
	}
	pong := decodePayload[message.PongPayload](t, message.TypePong, resp.PayloadBytes)
	if pong.PingTimeUnix != 200 || pong.TimeUnix != 100 {
		t.Fatalf("bad pong: %+v", pong)
	}

	pongMsg := signedMessage(t, peer.PrivateKey, message.TypePong, message.PongPayload{PingTimeUnix: 100, TimeUnix: 250}, 250, testNonce(14))
	if _, err := h.HandleMessage(pongMsg, peer.PublicKey); err != nil {
		t.Fatalf("handle pong: %v", err)
	}
	if h.Session.LastSeenUnix != 250 {
		t.Fatalf("last seen %d, want 250", h.Session.LastSeenUnix)
	}
}

func TestNoNetworkingPackageSurface(t *testing.T) {
	// This package intentionally has no transport API. The compile-time test
	// surface only constructs signed messages and local node state.
}

func testHandler(t *testing.T, local *node.Node, peer *node.Node) *Handler {
	t.Helper()
	h, err := NewHandler(local, NewSession(local.ID, peer.ID))
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	return h
}

func signedMessage(t *testing.T, priv crypto.PrivateKey, typ message.MessageType, payload any, timestamp int64, nonce [24]byte) message.Message {
	t.Helper()
	env, payloadBytes, err := message.SignEnvelope(priv, typ, payload, timestamp, nonce)
	if err != nil {
		t.Fatalf("sign message: %v", err)
	}
	return message.Message{Envelope: env, PayloadBytes: payloadBytes}
}

func decodePayload[T any](t *testing.T, typ message.MessageType, data []byte) T {
	t.Helper()
	payload, err := message.DecodePayload(typ, data)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	typed, ok := payload.(T)
	if !ok {
		t.Fatalf("payload type %T", payload)
	}
	return typed
}

func quoteRequest(seller *node.Node, buyer *node.Node, sellUnit model.UnitID, buyUnit model.UnitID) message.QuoteRequestPayload {
	return message.QuoteRequestPayload{
		RequestID:   testTxID(21),
		Seller:      seller.ID,
		Buyer:       buyer.ID,
		SellUnit:    sellUnit,
		BuyUnit:     buyUnit,
		SellAmount:  model.FromFloat(2),
		SpreadLimit: 0,
		ExpiryUnix:  seller.NowUnix(),
	}
}

func executableQuoteResponse(t *testing.T, seller *node.Node, buyer *node.Node, sellUnit model.UnitID, buyUnit model.UnitID, requestID model.TxID) message.QuoteResponsePayload {
	t.Helper()
	q := node.Quote{
		Seller:     seller.ID,
		Buyer:      buyer.ID,
		SellUnit:   sellUnit,
		BuyUnit:    buyUnit,
		SellAmount: model.FromFloat(2),
		BuyAmount:  model.FromFloat(2),
		SellerAsk:  2,
		BuyerBid:   2,
		Executable: true,
		Reason:     "executable",
		Timestamp:  seller.NowUnix(),
	}
	p, err := quoteResponseFromQuote(requestID, q)
	if err != nil {
		t.Fatalf("quote response: %v", err)
	}
	return p
}

func setQuoteID(t *testing.T, p *message.QuoteResponsePayload) {
	t.Helper()
	id, err := quoteID(*p)
	if err != nil {
		t.Fatalf("quote id: %v", err)
	}
	p.QuoteID = id
}

func authorizedTradePayload(t *testing.T, seller *node.Node, buyer *node.Node, sellUnit model.UnitID, buyUnit model.UnitID) message.AuthorizedTradePayload {
	t.Helper()
	_, payload := authorizedTradePayloadForParties(t, seller, buyer, sellUnit, buyUnit)
	return payload
}

func negotiatedAuthorizedTradePayload(t *testing.T, h *Handler, seller *node.Node, buyer *node.Node, sellUnit model.UnitID, buyUnit model.UnitID) (message.QuoteResponsePayload, message.AuthorizedTradePayload) {
	t.Helper()
	q, payload := authorizedTradePayloadForParties(t, seller, buyer, sellUnit, buyUnit)
	h.Session.PendingQuotes[q.QuoteID] = q
	h.Session.PendingIntents[q.QuoteID] = []node.SignedTradeIntent{
		payload.AuthorizedTrade.SellerAuth,
		payload.AuthorizedTrade.BuyerAuth,
	}
	h.Session.PendingTrades[payload.AuthorizedTradeID] = payload.AuthorizedTrade
	return q, payload
}

func authorizedTradePayloadForParties(t *testing.T, seller *node.Node, buyer *node.Node, sellUnit model.UnitID, buyUnit model.UnitID) (message.QuoteResponsePayload, message.AuthorizedTradePayload) {
	t.Helper()
	configureTestUnit(t, seller, sellUnit)
	configureTestUnit(t, seller, buyUnit)
	configureTestUnit(t, buyer, sellUnit)
	configureTestUnit(t, buyer, buyUnit)
	if seller.Balance(sellUnit) < model.FromFloat(2) {
		seller.AddInventory(sellUnit, model.FromFloat(2))
	}
	if buyer.Balance(buyUnit) < model.FromFloat(2) {
		buyer.AddInventory(buyUnit, model.FromFloat(2))
	}
	q := executableQuoteResponse(t, seller, buyer, sellUnit, buyUnit, testTxID(31))
	quote := quoteFromResponse(q)
	sellerSig, err := seller.SignQuote(quote)
	if err != nil {
		t.Fatalf("seller sign quote: %v", err)
	}
	buyerSig, err := buyer.SignQuote(quote)
	if err != nil {
		t.Fatalf("buyer sign quote: %v", err)
	}
	auth, id, err := buildAuthorizedTrade(quote, sellerSig, buyerSig)
	if err != nil {
		t.Fatalf("build authorized trade: %v", err)
	}
	return q, message.AuthorizedTradePayload{AuthorizedTrade: auth, AuthorizedTradeID: id}
}

func testTradeNodes(t *testing.T) (*node.Node, *node.Node, model.UnitID, model.UnitID) {
	t.Helper()
	seller := testNode(t, 100)
	buyer := testNode(t, 100)
	sellUnit := testUnit(t, seller.ID, "SKUG")
	buyUnit := testUnit(t, buyer.ID, "WEB4")
	configureTestUnit(t, seller, sellUnit)
	configureTestUnit(t, seller, buyUnit)
	configureTestUnit(t, buyer, sellUnit)
	configureTestUnit(t, buyer, buyUnit)
	seller.AddInventory(sellUnit, model.FromFloat(10))
	buyer.AddInventory(buyUnit, model.FromFloat(10))
	return seller, buyer, sellUnit, buyUnit
}

func testNode(t *testing.T, now int64) *node.Node {
	t.Helper()
	_, priv, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	n, err := node.NewNode(priv, node.DefaultPriceConfig())
	if err != nil {
		t.Fatalf("new node: %v", err)
	}
	n.NowUnix = func() int64 { return now }
	return n
}

func testKey(t *testing.T) (crypto.PublicKey, crypto.PrivateKey, model.NodeID) {
	t.Helper()
	pub, priv, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	id, err := model.NodeIDFromPublicKey(pub)
	if err != nil {
		t.Fatalf("node id: %v", err)
	}
	return pub, priv, id
}

func testUnit(t *testing.T, issuer model.NodeID, metadata string) model.UnitID {
	t.Helper()
	unit, err := model.NewUnitIDFromMetadata(issuer, []byte(metadata))
	if err != nil {
		t.Fatalf("unit id: %v", err)
	}
	return unit
}

func configureTestUnit(t *testing.T, n *node.Node, unit model.UnitID) {
	t.Helper()
	n.Features[unit] = price.AssetFeatures{Cost: 1}
	n.PriceConfig = price.PriceConfig{
		BasePrice:       1,
		Weights:         price.FeatureWeights{Cost: 1},
		VolumeThreshold: model.FromFloat(10),
	}
	n.ComputePrice(unit)
}

func testNonce(seed byte) [24]byte {
	var nonce [24]byte
	for i := range nonce {
		nonce[i] = seed + byte(i)
	}
	return nonce
}

func testTxID(seed byte) model.TxID {
	var id model.TxID
	for i := range id {
		id[i] = seed + byte(i)
	}
	return id
}

type fakeStore struct {
	executed  map[model.TxID]bool
	inventory map[model.NodeID]model.InventoryState
	flow      map[model.NodeID]map[model.UnitID]model.FlowRecord
	prices    map[model.NodeID]map[model.UnitID]price.PriceResult
	trades    map[model.TxID]node.AuthorizedTradeTx
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		executed:  map[model.TxID]bool{},
		inventory: map[model.NodeID]model.InventoryState{},
		flow:      map[model.NodeID]map[model.UnitID]model.FlowRecord{},
		prices:    map[model.NodeID]map[model.UnitID]price.PriceResult{},
		trades:    map[model.TxID]node.AuthorizedTradeTx{},
	}
}

func (s *fakeStore) HasExecutedTrade(id model.TxID) bool { return s.executed[id] }
func (s *fakeStore) MarkExecutedTrade(id model.TxID) error {
	s.executed[id] = true
	return nil
}
func (s *fakeStore) SaveInventory(id model.NodeID, inv model.InventoryState) error {
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
func (s *fakeStore) SaveAuthorizedTrade(id model.TxID, tx node.AuthorizedTradeTx) error {
	s.trades[id] = tx
	return nil
}
func (s *fakeStore) LoadAuthorizedTrade(id model.TxID) (node.AuthorizedTradeTx, bool) {
	tx, ok := s.trades[id]
	return tx, ok
}
func (s *fakeStore) PersistExecutedTrade(id model.TxID, tx node.AuthorizedTradeTx, states ...node.PersistedNodeState) error {
	if s.HasExecutedTrade(id) {
		return errReplay
	}
	s.trades[id] = tx
	for _, state := range states {
		s.inventory[state.ID] = state.Inventory.Copy()
		s.flow[state.ID] = copyFlow(state.Flow)
		s.prices[state.ID] = copyPriceState(state.PriceState)
	}
	return s.MarkExecutedTrade(id)
}
func (s *fakeStore) Close() error { return nil }

var errReplay = errors.New("trade replay rejected")

func copyFlow(in map[model.UnitID]model.FlowRecord) map[model.UnitID]model.FlowRecord {
	out := make(map[model.UnitID]model.FlowRecord, len(in))
	for unit, record := range in {
		out[unit] = record
	}
	return out
}

func copyPriceState(in map[model.UnitID]price.PriceResult) map[model.UnitID]price.PriceResult {
	out := make(map[model.UnitID]price.PriceResult, len(in))
	for unit, result := range in {
		out[unit] = result
	}
	return out
}

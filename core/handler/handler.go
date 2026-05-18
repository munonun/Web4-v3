package handler

import (
	"crypto/rand"
	"fmt"
	"math"

	"web4-v3/core/crypto"
	"web4-v3/core/message"
	"web4-v3/core/model"
	"web4-v3/core/node"
	"web4-v3/core/price"
)

type Handler struct {
	Node    *node.Node
	Session *Session
}

func NewHandler(n *node.Node, s *Session) (*Handler, error) {
	if n == nil {
		return nil, fmt.Errorf("%w: node is nil", ErrInvalidState)
	}
	if s == nil {
		return nil, fmt.Errorf("%w: session is nil", ErrInvalidState)
	}
	if s.LocalID != n.ID {
		return nil, fmt.Errorf("%w: session local id mismatch", ErrInvalidState)
	}
	s.ensureMaps()
	return &Handler{Node: n, Session: s}, nil
}

func (h *Handler) HandleMessage(msg message.Message, peerPub crypto.PublicKey) (*message.Message, error) {
	if h == nil || h.Node == nil || h.Session == nil {
		return nil, fmt.Errorf("%w: handler is not initialized", ErrInvalidState)
	}
	if err := message.VerifyEnvelope(msg.Envelope, msg.PayloadBytes, peerPub); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPeer, err)
	}
	if msg.Envelope.Sender != h.Session.PeerID {
		return nil, fmt.Errorf("%w: envelope sender is not session peer", ErrInvalidPeer)
	}
	if h.Session.HasSeen(msg.Envelope.MessageID) {
		return nil, ErrDuplicateMessage
	}
	if err := h.Session.MarkSeen(msg.Envelope.MessageID); err != nil {
		return nil, err
	}

	payload, err := message.DecodePayload(msg.Envelope.Type, msg.PayloadBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPayload, err)
	}
	if err := message.ValidatePayloadSemantics(msg.Envelope.Type, payload); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPayload, err)
	}
	h.Session.LastSeenUnix = msg.Envelope.Timestamp

	switch msg.Envelope.Type {
	case message.TypeHello:
		return h.handleHello(payload.(message.HelloPayload), peerPub)
	case message.TypeQuoteRequest:
		return h.handleQuoteRequest(msg.Envelope.MessageID, payload.(message.QuoteRequestPayload))
	case message.TypeQuoteResponse:
		return h.handleQuoteResponse(msg.Envelope.MessageID, payload.(message.QuoteResponsePayload))
	case message.TypeSignedIntent:
		return h.handleSignedIntent(msg.Envelope.MessageID, payload.(message.SignedIntentPayload))
	case message.TypeAuthorizedTrade:
		return h.handleAuthorizedTrade(payload.(message.AuthorizedTradePayload))
	case message.TypeTradeResult:
		return h.handleTradeResult(payload.(message.TradeResultPayload))
	case message.TypeReject:
		return h.handleReject(payload.(message.RejectPayload))
	case message.TypePing:
		return h.handlePing(payload.(message.PingPayload))
	case message.TypePong:
		return h.handlePong(payload.(message.PongPayload))
	default:
		return nil, fmt.Errorf("%w: unsupported message type %q", ErrUnexpectedMessage, msg.Envelope.Type)
	}
}

func (h *Handler) SignResponse(msgType message.MessageType, payload any) (*message.Message, error) {
	if h.Node == nil || len(h.Node.PrivateKey) == 0 {
		return nil, fmt.Errorf("%w: node has no private key", ErrInvalidState)
	}
	if h.Node.NowUnix == nil {
		return nil, fmt.Errorf("%w: node clock is nil", ErrInvalidState)
	}
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	now := h.Node.NowUnix()
	env, payloadBytes, err := message.SignEnvelope(h.Node.PrivateKey, msgType, payload, now, nonce)
	if err != nil {
		return nil, err
	}
	return &message.Message{Envelope: env, PayloadBytes: payloadBytes}, nil
}

func (h *Handler) handleHello(payload message.HelloPayload, peerPub crypto.PublicKey) (*message.Message, error) {
	peerID, err := model.NodeIDFromPublicKey(peerPub)
	if err != nil {
		return nil, err
	}
	if payload.NodeID != h.Session.PeerID || payload.NodeID != peerID {
		return nil, fmt.Errorf("%w: hello peer mismatch", ErrInvalidPeer)
	}
	return nil, nil
}

func (h *Handler) handleQuoteRequest(ref model.TxID, payload message.QuoteRequestPayload) (*message.Message, error) {
	if payload.Seller != h.Node.ID || payload.Buyer != h.Session.PeerID {
		return h.reject(ref, "INVALID_ROLE", "quote request roles do not match session")
	}
	if isExpired(payload.ExpiryUnix, h.Node.NowUnix()) {
		h.Session.ClearRequest(payload.RequestID)
		return h.reject(ref, "QUOTE_EXPIRED", "quote request is expired")
	}
	h.Session.PendingRequests[payload.RequestID] = payload

	peer := h.peerShadow(payload.Buyer, payload.BuyUnit, payload.SellUnit, payload.SellAmount)
	quote := h.Node.QuoteSell(peer, payload.SellUnit, payload.BuyUnit, payload.SellAmount, payload.SpreadLimit)
	response, err := quoteResponseFromQuote(payload.RequestID, quote)
	if err != nil {
		return nil, err
	}
	if err := message.ValidatePayloadSemantics(message.TypeQuoteResponse, response); err != nil {
		return nil, err
	}
	h.Session.PendingQuotes[response.QuoteID] = response
	return h.SignResponse(message.TypeQuoteResponse, response)
}

func (h *Handler) handleQuoteResponse(ref model.TxID, payload message.QuoteResponsePayload) (*message.Message, error) {
	if !h.sessionOwnsParties(payload.Seller, payload.Buyer) {
		return h.reject(ref, "INVALID_ROLE", "quote response roles do not match session")
	}
	request, ok := h.Session.PendingRequests[payload.RequestID]
	if !ok {
		return h.reject(ref, "UNEXPECTED_QUOTE_RESPONSE", "quote response does not match a pending request")
	}
	now := h.Node.NowUnix()
	if err := validateQuoteMatchesRequest(payload, request, now); err != nil {
		if freshnessErr := validateQuoteFreshness(&request, payload, now); freshnessErr != nil {
			h.Session.ClearRequest(payload.RequestID)
		}
		return h.reject(ref, "QUOTE_MISMATCH", err.Error())
	}
	expectedQuoteID, err := quoteID(payload)
	if err != nil {
		return nil, err
	}
	if expectedQuoteID != payload.QuoteID {
		return h.reject(ref, "QUOTE_ID_MISMATCH", "quote id does not match quote response")
	}
	h.Session.PendingQuotes[payload.QuoteID] = payload
	if !payload.Executable {
		return nil, nil
	}
	if h.Node.ID != payload.Seller && h.Node.ID != payload.Buyer {
		return nil, fmt.Errorf("%w: local node is not quote party", ErrInvalidState)
	}
	quote := quoteFromResponse(payload)
	intent, err := h.Node.SignQuote(quote)
	if err != nil {
		return h.reject(ref, "SIGN_INTENT_FAILED", err.Error())
	}
	h.Session.PendingIntents[payload.QuoteID] = append(h.Session.PendingIntents[payload.QuoteID], intent)
	return h.SignResponse(message.TypeSignedIntent, message.SignedIntentPayload{QuoteID: payload.QuoteID, Intent: intent})
}

func (h *Handler) handleSignedIntent(ref model.TxID, payload message.SignedIntentPayload) (*message.Message, error) {
	quote, ok := h.Session.PendingQuotes[payload.QuoteID]
	if !ok {
		return h.reject(ref, "UNEXPECTED_SIGNED_INTENT", "signed intent does not match a pending quote")
	}
	if err := h.validatePendingQuoteFreshness(quote); err != nil {
		h.Session.ClearNegotiation(payload.QuoteID, quote, model.TxID{})
		return h.reject(ref, "QUOTE_EXPIRED", err.Error())
	}
	if !h.sessionOwnsParties(quote.Seller, quote.Buyer) {
		return h.reject(ref, "INVALID_ROLE", "quote roles do not match session")
	}
	if payload.Intent.Intent.Party != h.Session.PeerID {
		return h.reject(ref, "INVALID_ROLE", "signed intent is not from session peer")
	}
	if !intentMatchesQuote(payload.Intent.Intent, quote) {
		return h.reject(ref, "INTENT_MISMATCH", "signed intent does not match quote")
	}
	if hasPartyIntent(h.Session.PendingIntents[payload.QuoteID], payload.Intent.Intent.Party) {
		return h.reject(ref, "DUPLICATE_INTENT", "duplicate signed intent for quote party")
	}

	intents := append(h.Session.PendingIntents[payload.QuoteID], payload.Intent)
	h.Session.PendingIntents[payload.QuoteID] = intents
	if !hasPartyIntent(intents, h.Node.ID) && (h.Node.ID == quote.Seller || h.Node.ID == quote.Buyer) {
		localIntent, err := h.Node.SignQuote(quoteFromResponse(quote))
		if err != nil {
			return h.reject(ref, "SIGN_INTENT_FAILED", err.Error())
		}
		intents = append(intents, localIntent)
		h.Session.PendingIntents[payload.QuoteID] = intents
	}
	sellerSig, buyerSig, ok := selectIntents(intents, quote.Seller, quote.Buyer)
	if !ok {
		return nil, nil
	}
	auth, authID, err := buildAuthorizedTrade(quoteFromResponse(quote), sellerSig, buyerSig)
	if err != nil {
		return nil, err
	}
	h.Session.PendingTrades[authID] = auth
	return h.SignResponse(message.TypeAuthorizedTrade, message.AuthorizedTradePayload{AuthorizedTrade: auth, AuthorizedTradeID: authID})
}

func (h *Handler) handleAuthorizedTrade(payload message.AuthorizedTradePayload) (*message.Message, error) {
	auth := payload.AuthorizedTrade
	if !node.VerifyTradeIntent(auth.SellerAuth) || !node.VerifyTradeIntent(auth.BuyerAuth) {
		return nil, fmt.Errorf("%w: invalid trade intent", ErrInvalidPayload)
	}
	if h.Node.ID != auth.SellerAuth.Intent.Seller && h.Node.ID != auth.SellerAuth.Intent.Buyer {
		return nil, fmt.Errorf("%w: local node is not trade party", ErrInvalidState)
	}
	if err := h.requireSessionCounterparty(auth.SellerAuth.Intent.Seller, auth.SellerAuth.Intent.Buyer); err != nil {
		return nil, err
	}

	quoteID, quotePayload, err := h.authorizedNegotiation(payload)
	if err != nil {
		return nil, err
	}
	quote := quoteFromResponse(quotePayload)
	seller, buyer := h.executionParties(quote, auth)
	_, err = node.ExecuteSignedTradeWithPeerShadow(seller, buyer, quote, auth.SellerAuth, auth.BuyerAuth)
	status := message.TradeResultStatusAccepted
	reason := "executed"
	if err != nil {
		status = message.TradeResultStatusFailed
		reason = err.Error()
	}
	h.Session.PendingTrades[payload.AuthorizedTradeID] = auth
	result := message.TradeResultPayload{AuthorizedTradeID: payload.AuthorizedTradeID, Status: status, Reason: reason}
	response, signErr := h.SignResponse(message.TypeTradeResult, result)
	if signErr != nil {
		return nil, signErr
	}
	if err != nil {
		return response, fmt.Errorf("%w: %v", ErrTradeExecutionFailed, err)
	}
	h.clearNegotiation(quoteID, quotePayload, payload.AuthorizedTradeID)
	return response, nil
}

func (h *Handler) handleTradeResult(payload message.TradeResultPayload) (*message.Message, error) {
	if _, ok := h.Session.PendingTrades[payload.AuthorizedTradeID]; !ok {
		return nil, fmt.Errorf("%w: unknown trade result", ErrUnexpectedMessage)
	}
	return nil, nil
}

func (h *Handler) handleReject(payload message.RejectPayload) (*message.Message, error) {
	return nil, nil
}

func (h *Handler) handlePing(payload message.PingPayload) (*message.Message, error) {
	return h.SignResponse(message.TypePong, message.PongPayload{PingTimeUnix: payload.TimeUnix, TimeUnix: h.Node.NowUnix()})
}

func (h *Handler) handlePong(payload message.PongPayload) (*message.Message, error) {
	h.Session.LastSeenUnix = payload.TimeUnix
	return nil, nil
}

func (h *Handler) reject(ref model.TxID, code string, reason string) (*message.Message, error) {
	return h.SignResponse(message.TypeReject, message.RejectPayload{RefMessageID: ref, Code: code, Reason: reason})
}

func (h *Handler) peerShadow(id model.NodeID, inventoryUnit model.UnitID, priceUnit model.UnitID, amount model.Amount) *node.Node {
	peer := node.New(id)
	peer.NowUnix = h.Node.NowUnix
	peer.PriceConfig = node.DefaultPriceConfig()
	configureUnit(peer, inventoryUnit)
	configureUnit(peer, priceUnit)
	inventoryAmount := amount
	if amount <= model.Amount(math.MaxInt64/10) {
		inventoryAmount = amount * 10
	}
	peer.AddInventory(inventoryUnit, inventoryAmount)
	return peer
}

func (h *Handler) executionParties(q node.Quote, auth node.AuthorizedTradeTx) (*node.Node, *node.Node) {
	if h.Node.ID == q.Seller {
		buyer := h.peerShadow(q.Buyer, q.BuyUnit, q.SellUnit, q.BuyAmount)
		buyer.PublicKey = append(crypto.PublicKey(nil), auth.BuyerAuth.PublicKey...)
		return h.Node, buyer
	}
	seller := h.peerShadow(q.Seller, q.SellUnit, q.BuyUnit, q.SellAmount)
	seller.PublicKey = append(crypto.PublicKey(nil), auth.SellerAuth.PublicKey...)
	return seller, h.Node
}

func configureUnit(n *node.Node, unit model.UnitID) {
	n.Features[unit] = price.AssetFeatures{Cost: 1}
	n.PriceConfig = node.DefaultPriceConfig()
	n.ComputePrice(unit)
}

func (h *Handler) sessionOwnsParties(seller model.NodeID, buyer model.NodeID) bool {
	return (seller == h.Session.LocalID && buyer == h.Session.PeerID) ||
		(seller == h.Session.PeerID && buyer == h.Session.LocalID)
}

func (h *Handler) requireSessionCounterparty(seller model.NodeID, buyer model.NodeID) error {
	switch h.Node.ID {
	case seller:
		if h.Session.PeerID != buyer {
			return fmt.Errorf("%w: session peer is not buyer counterparty", ErrInvalidPeer)
		}
	case buyer:
		if h.Session.PeerID != seller {
			return fmt.Errorf("%w: session peer is not seller counterparty", ErrInvalidPeer)
		}
	default:
		return fmt.Errorf("%w: local node is not trade party", ErrInvalidState)
	}
	return nil
}

func (h *Handler) authorizedNegotiation(payload message.AuthorizedTradePayload) (model.TxID, message.QuoteResponsePayload, error) {
	auth := payload.AuthorizedTrade
	for quoteID, quote := range h.Session.PendingQuotes {
		if !h.sessionOwnsParties(quote.Seller, quote.Buyer) {
			continue
		}
		if !intentMatchesQuote(auth.SellerAuth.Intent, quote) || !intentMatchesQuote(auth.BuyerAuth.Intent, quote) {
			continue
		}
		if err := h.validatePendingQuoteFreshness(quote); err != nil {
			h.Session.ClearNegotiation(quoteID, quote, payload.AuthorizedTradeID)
			return model.TxID{}, message.QuoteResponsePayload{}, fmt.Errorf("%w: %v", ErrUnexpectedMessage, err)
		}
		if auth.SellerAuth.Intent.Party != quote.Seller || auth.BuyerAuth.Intent.Party != quote.Buyer {
			return model.TxID{}, message.QuoteResponsePayload{}, fmt.Errorf("%w: authorization party mismatch", ErrInvalidPayload)
		}
		if err := h.bindAuthorizedIntents(quoteID, auth); err != nil {
			return model.TxID{}, message.QuoteResponsePayload{}, err
		}
		rebuilt, rebuiltID, err := buildAuthorizedTrade(quoteFromResponse(quote), auth.SellerAuth, auth.BuyerAuth)
		if err != nil {
			return model.TxID{}, message.QuoteResponsePayload{}, err
		}
		if rebuiltID != payload.AuthorizedTradeID {
			return model.TxID{}, message.QuoteResponsePayload{}, fmt.Errorf("%w: authorized trade id does not match negotiated state", ErrInvalidPayload)
		}
		if pending, ok := h.Session.PendingTrades[payload.AuthorizedTradeID]; ok && !authorizedTradeEqual(pending, rebuilt) {
			return model.TxID{}, message.QuoteResponsePayload{}, fmt.Errorf("%w: pending authorized trade mismatch", ErrInvalidPayload)
		}
		h.Session.PendingTrades[payload.AuthorizedTradeID] = rebuilt
		return quoteID, quote, nil
	}
	return model.TxID{}, message.QuoteResponsePayload{}, fmt.Errorf("%w: authorized trade does not match negotiated session state", ErrUnexpectedMessage)
}

func (h *Handler) bindAuthorizedIntents(quoteID model.TxID, auth node.AuthorizedTradeTx) error {
	intents := h.Session.PendingIntents[quoteID]
	local, peer := auth.SellerAuth, auth.BuyerAuth
	if h.Node.ID == auth.BuyerAuth.Intent.Party {
		local, peer = auth.BuyerAuth, auth.SellerAuth
	}
	if local.Intent.Party != h.Node.ID {
		return fmt.Errorf("%w: missing local authorization", ErrInvalidPayload)
	}
	if peer.Intent.Party != h.Session.PeerID {
		return fmt.Errorf("%w: missing peer authorization", ErrInvalidPayload)
	}
	if !hasSignedIntent(intents, local) {
		return fmt.Errorf("%w: local authorization was not negotiated in session", ErrUnexpectedMessage)
	}
	if !hasSignedIntent(intents, peer) {
		if hasPartyIntent(intents, peer.Intent.Party) {
			return fmt.Errorf("%w: peer authorization differs from negotiated intent", ErrInvalidPayload)
		}
		return fmt.Errorf("%w: peer authorization was not negotiated in session", ErrUnexpectedMessage)
	}
	return nil
}

func (h *Handler) clearNegotiation(quoteID model.TxID, quote message.QuoteResponsePayload, authID model.TxID) {
	h.Session.ClearNegotiation(quoteID, quote, authID)
}

func (h *Handler) validatePendingQuoteFreshness(quote message.QuoteResponsePayload) error {
	if h == nil || h.Node == nil || h.Session == nil {
		return fmt.Errorf("%w: handler is not initialized", ErrInvalidState)
	}
	now := h.Node.NowUnix()
	if request, ok := h.Session.PendingRequests[quote.RequestID]; ok {
		return validateQuoteFreshness(&request, quote, now)
	}
	return validateQuoteFreshness(nil, quote, now)
}

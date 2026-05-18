package handler

import (
	"fmt"

	"web4-v3/core/message"
	"web4-v3/core/model"
	"web4-v3/core/node"
)

type Session struct {
	LocalID model.NodeID
	PeerID  model.NodeID

	PendingRequests map[model.TxID]message.QuoteRequestPayload
	PendingQuotes   map[model.TxID]message.QuoteResponsePayload
	PendingIntents  map[model.TxID][]node.SignedTradeIntent
	PendingTrades   map[model.TxID]node.AuthorizedTradeTx

	SeenMessages map[model.TxID]bool

	LastSeenUnix int64
}

func NewSession(localID, peerID model.NodeID) *Session {
	s := &Session{
		LocalID:         localID,
		PeerID:          peerID,
		PendingRequests: map[model.TxID]message.QuoteRequestPayload{},
		PendingQuotes:   map[model.TxID]message.QuoteResponsePayload{},
		PendingIntents:  map[model.TxID][]node.SignedTradeIntent{},
		PendingTrades:   map[model.TxID]node.AuthorizedTradeTx{},
		SeenMessages:    map[model.TxID]bool{},
	}
	s.ensureMaps()
	return s
}

func (s *Session) MarkSeen(id model.TxID) error {
	if s == nil {
		return fmt.Errorf("%w: session is nil", ErrInvalidState)
	}
	s.ensureMaps()
	if s.SeenMessages[id] {
		return ErrDuplicateMessage
	}
	s.SeenMessages[id] = true
	return nil
}

func (s *Session) HasSeen(id model.TxID) bool {
	return s != nil && s.SeenMessages != nil && s.SeenMessages[id]
}

func (s *Session) PruneExpired(nowUnix int64) {
	if s == nil {
		return
	}
	s.ensureMaps()
	for requestID, req := range s.PendingRequests {
		if isExpired(req.ExpiryUnix, nowUnix) {
			s.ClearRequest(requestID)
		}
	}
	for quoteID, quote := range s.PendingQuotes {
		if isExpired(quote.ExpiryUnix, nowUnix) {
			s.ClearNegotiation(quoteID, quote, model.TxID{})
		}
	}
}

func (s *Session) ClearRequest(requestID model.TxID) {
	if s == nil {
		return
	}
	s.ensureMaps()
	for quoteID, quote := range s.PendingQuotes {
		if quote.RequestID == requestID {
			s.ClearNegotiation(quoteID, quote, model.TxID{})
		}
	}
	delete(s.PendingRequests, requestID)
}

func (s *Session) ClearNegotiation(quoteID model.TxID, quote message.QuoteResponsePayload, authID model.TxID) {
	if s == nil {
		return
	}
	s.ensureMaps()
	delete(s.PendingQuotes, quoteID)
	delete(s.PendingIntents, quoteID)
	delete(s.PendingRequests, quote.RequestID)
	if authID != (model.TxID{}) {
		delete(s.PendingTrades, authID)
	}
	for pendingID, pending := range s.PendingTrades {
		if intentMatchesQuote(pending.SellerAuth.Intent, quote) && intentMatchesQuote(pending.BuyerAuth.Intent, quote) {
			delete(s.PendingTrades, pendingID)
		}
	}
}

func (s *Session) ensureMaps() {
	if s.PendingRequests == nil {
		s.PendingRequests = map[model.TxID]message.QuoteRequestPayload{}
	}
	if s.PendingQuotes == nil {
		s.PendingQuotes = map[model.TxID]message.QuoteResponsePayload{}
	}
	if s.PendingIntents == nil {
		s.PendingIntents = map[model.TxID][]node.SignedTradeIntent{}
	}
	if s.PendingTrades == nil {
		s.PendingTrades = map[model.TxID]node.AuthorizedTradeTx{}
	}
	if s.SeenMessages == nil {
		s.SeenMessages = map[model.TxID]bool{}
	}
}

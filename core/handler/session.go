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

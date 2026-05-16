package message

import (
	"web4-v3/core/crypto"
	"web4-v3/core/model"
	"web4-v3/core/node"
)

type HelloPayload struct {
	NodeID            model.NodeID
	PublicKey         crypto.PublicKey
	SupportedVersions []uint16
	Features          []string
}

type QuoteRequestPayload struct {
	RequestID   model.TxID
	Seller      model.NodeID
	Buyer       model.NodeID
	SellUnit    model.UnitID
	BuyUnit     model.UnitID
	SellAmount  model.Amount
	SpreadLimit float64
	ExpiryUnix  int64
}

type QuoteResponsePayload struct {
	RequestID  model.TxID
	QuoteID    model.TxID
	Seller     model.NodeID
	Buyer      model.NodeID
	SellUnit   model.UnitID
	BuyUnit    model.UnitID
	SellAmount model.Amount
	BuyAmount  model.Amount
	SellerAsk  float64
	BuyerBid   float64
	Executable bool
	Reason     string
	ExpiryUnix int64
}

type SignedIntentPayload struct {
	QuoteID model.TxID
	Intent  node.SignedTradeIntent
}

type AuthorizedTradePayload struct {
	AuthorizedTrade   node.AuthorizedTradeTx
	AuthorizedTradeID model.TxID
}

type TradeResultPayload struct {
	AuthorizedTradeID model.TxID
	Status            string
	Reason            string
}

type RejectPayload struct {
	RefMessageID model.TxID
	Code         string
	Reason       string
}

type PingPayload struct {
	TimeUnix int64
}

type PongPayload struct {
	PingTimeUnix int64
	TimeUnix     int64
}

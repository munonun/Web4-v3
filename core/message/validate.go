package message

import (
	"fmt"
	"math"

	"web4-v3/core/model"
	"web4-v3/core/node"
)

const (
	TradeResultStatusAccepted = "ACCEPTED"
	TradeResultStatusRejected = "REJECTED"
	TradeResultStatusFailed   = "FAILED"
)

func ValidatePayloadSemantics(t MessageType, payload any) error {
	if !IsValidMessageType(t) {
		return fmt.Errorf("invalid message type %q", t)
	}
	normalized, err := normalizePayload(t, payload)
	if err != nil {
		return err
	}

	switch p := normalized.(type) {
	case HelloPayload:
		nodeID, err := model.NodeIDFromPublicKey(p.PublicKey)
		if err != nil {
			return err
		}
		if p.NodeID != nodeID {
			return fmt.Errorf("hello node id does not match public key")
		}
		if len(p.SupportedVersions) == 0 {
			return fmt.Errorf("hello supported versions are required")
		}
		for _, version := range p.SupportedVersions {
			if version == 0 {
				return fmt.Errorf("hello supported version must be greater than zero")
			}
		}
	case QuoteRequestPayload:
		if p.RequestID == (model.TxID{}) {
			return fmt.Errorf("quote request id is required")
		}
		if p.Seller == (model.NodeID{}) || p.Buyer == (model.NodeID{}) {
			return fmt.Errorf("quote request parties are required")
		}
		if p.Seller == p.Buyer {
			return fmt.Errorf("quote request parties must differ")
		}
		if p.SellAmount <= 0 {
			return fmt.Errorf("quote request sell amount must be greater than zero")
		}
		if p.SpreadLimit < 0 || math.IsNaN(p.SpreadLimit) || math.IsInf(p.SpreadLimit, 0) {
			return fmt.Errorf("quote request spread limit must be finite and non-negative")
		}
		if p.ExpiryUnix < 0 {
			return fmt.Errorf("quote request expiry must be non-negative")
		}
	case QuoteResponsePayload:
		if p.RequestID == (model.TxID{}) {
			return fmt.Errorf("quote response request id is required")
		}
		if p.QuoteID == (model.TxID{}) {
			return fmt.Errorf("quote response quote id is required")
		}
		if p.Seller == (model.NodeID{}) || p.Buyer == (model.NodeID{}) {
			return fmt.Errorf("quote response parties are required")
		}
		if p.SellAmount <= 0 {
			return fmt.Errorf("quote response sell amount must be greater than zero")
		}
		if p.Executable && p.BuyAmount <= 0 {
			return fmt.Errorf("executable quote response buy amount must be greater than zero")
		}
		if invalidPrice(p.SellerAsk) || invalidPrice(p.BuyerBid) {
			return fmt.Errorf("quote response prices must be finite and non-negative")
		}
		if p.ExpiryUnix < 0 {
			return fmt.Errorf("quote response expiry must be non-negative")
		}
	case SignedIntentPayload:
		if p.QuoteID == (model.TxID{}) {
			return fmt.Errorf("signed intent quote id is required")
		}
		if !node.VerifyTradeIntent(p.Intent) {
			return fmt.Errorf("signed trade intent is invalid")
		}
	case AuthorizedTradePayload:
		if p.AuthorizedTradeID == (model.TxID{}) {
			return fmt.Errorf("authorized trade id is required")
		}
		id, err := node.AuthorizedTradeID(p.AuthorizedTrade)
		if err != nil {
			return err
		}
		if id != p.AuthorizedTradeID {
			return fmt.Errorf("authorized trade id mismatch")
		}
	case TradeResultPayload:
		if p.AuthorizedTradeID == (model.TxID{}) {
			return fmt.Errorf("trade result authorized trade id is required")
		}
		switch p.Status {
		case TradeResultStatusAccepted, TradeResultStatusRejected, TradeResultStatusFailed:
		default:
			return fmt.Errorf("unknown trade result status %q", p.Status)
		}
	case RejectPayload:
		if p.RefMessageID == (model.TxID{}) {
			return fmt.Errorf("reject reference message id is required")
		}
		if p.Code == "" {
			return fmt.Errorf("reject code is required")
		}
	case PingPayload:
		if p.TimeUnix <= 0 {
			return fmt.Errorf("ping time must be greater than zero")
		}
	case PongPayload:
		if p.PingTimeUnix <= 0 || p.TimeUnix <= 0 {
			return fmt.Errorf("pong times must be greater than zero")
		}
	default:
		return fmt.Errorf("unsupported payload type %T", payload)
	}
	return nil
}

func invalidPrice(v float64) bool {
	return v < 0 || math.IsNaN(v) || math.IsInf(v, 0)
}

package node

import (
	"bytes"
	"crypto/ed25519"
	"fmt"

	"web4-v3/core/canonical"
	"web4-v3/core/crypto"
	"web4-v3/core/model"
)

type TradeIntent struct {
	Party      model.NodeID
	Seller     model.NodeID
	Buyer      model.NodeID
	SellUnit   model.UnitID
	BuyUnit    model.UnitID
	SellAmount model.Amount
	BuyAmount  model.Amount
	Timestamp  int64
}

type SignedTradeIntent struct {
	Intent    TradeIntent
	PublicKey crypto.PublicKey
	Signature crypto.Signature
}

type AuthorizedTradeTx struct {
	Tx         model.TradeTx
	SellerAuth SignedTradeIntent
	BuyerAuth  SignedTradeIntent
}

func IntentFromQuote(q Quote, party model.NodeID, timestamp int64) TradeIntent {
	return TradeIntent{
		Party:      party,
		Seller:     q.Seller,
		Buyer:      q.Buyer,
		SellUnit:   q.SellUnit,
		BuyUnit:    q.BuyUnit,
		SellAmount: q.SellAmount,
		BuyAmount:  q.BuyAmount,
		Timestamp:  timestamp,
	}
}

func TradeIntentID(intent TradeIntent) (model.TxID, error) {
	preimage, err := tradeIntentPreimage(intent)
	if err != nil {
		return model.TxID{}, err
	}
	return model.TxID(crypto.HashBytes(preimage)), nil
}

func SignTradeIntent(priv crypto.PrivateKey, intent TradeIntent) (SignedTradeIntent, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return SignedTradeIntent{}, fmt.Errorf("invalid private key length: got %d, want %d", len(priv), ed25519.PrivateKeySize)
	}
	pub, ok := ed25519.PrivateKey(priv).Public().(ed25519.PublicKey)
	if !ok || len(pub) != ed25519.PublicKeySize {
		return SignedTradeIntent{}, fmt.Errorf("invalid private key public component")
	}
	nodeID, err := model.NodeIDFromPublicKey(crypto.PublicKey(pub))
	if err != nil {
		return SignedTradeIntent{}, err
	}
	if nodeID != intent.Party {
		return SignedTradeIntent{}, fmt.Errorf("intent party does not match private key")
	}
	preimage, err := tradeIntentPreimage(intent)
	if err != nil {
		return SignedTradeIntent{}, err
	}
	sig, err := crypto.Sign(priv, preimage)
	if err != nil {
		return SignedTradeIntent{}, err
	}
	return SignedTradeIntent{
		Intent:    intent,
		PublicKey: append(crypto.PublicKey(nil), pub...),
		Signature: sig,
	}, nil
}

func VerifyTradeIntent(sig SignedTradeIntent) bool {
	nodeID, err := model.NodeIDFromPublicKey(sig.PublicKey)
	if err != nil || nodeID != sig.Intent.Party {
		return false
	}
	preimage, err := tradeIntentPreimage(sig.Intent)
	if err != nil {
		return false
	}
	return crypto.Verify(sig.PublicKey, preimage, sig.Signature)
}

func (n *Node) SignQuote(q Quote) (SignedTradeIntent, error) {
	n.init()
	if !q.Executable {
		return SignedTradeIntent{}, quoteExecutionError(q)
	}
	if n.ID != q.Seller && n.ID != q.Buyer {
		return SignedTradeIntent{}, fmt.Errorf("node is not a quote party")
	}
	if len(n.PrivateKey) == 0 {
		return SignedTradeIntent{}, fmt.Errorf("node has no private key")
	}
	if !n.AcceptQuote(q) {
		return SignedTradeIntent{}, fmt.Errorf("node no longer accepts quote")
	}
	timestamp := q.Timestamp
	if timestamp == 0 {
		timestamp = n.NowUnix()
	}
	return SignTradeIntent(n.PrivateKey, IntentFromQuote(q, n.ID, timestamp))
}

func (n *Node) VerifySignedIntent(sig SignedTradeIntent) bool {
	return VerifyTradeIntent(sig)
}

func AuthorizedTradeID(auth AuthorizedTradeTx) (model.TxID, error) {
	txID, err := model.TradeTxID(auth.Tx)
	if err != nil {
		return model.TxID{}, err
	}
	sellerID, err := TradeIntentID(auth.SellerAuth.Intent)
	if err != nil {
		return model.TxID{}, err
	}
	buyerID, err := TradeIntentID(auth.BuyerAuth.Intent)
	if err != nil {
		return model.TxID{}, err
	}
	preimage, err := canonical.EncodeFields(
		canonical.Field{Name: "kind", Value: "authorized_trade_tx"},
		canonical.Field{Name: "tx_id", Value: hashBytes(txID)},
		canonical.Field{Name: "seller_intent_id", Value: hashBytes(sellerID)},
		canonical.Field{Name: "buyer_intent_id", Value: hashBytes(buyerID)},
		canonical.Field{Name: "seller_public_key", Value: []byte(auth.SellerAuth.PublicKey)},
		canonical.Field{Name: "buyer_public_key", Value: []byte(auth.BuyerAuth.PublicKey)},
		canonical.Field{Name: "seller_signature", Value: []byte(auth.SellerAuth.Signature)},
		canonical.Field{Name: "buyer_signature", Value: []byte(auth.BuyerAuth.Signature)},
	)
	if err != nil {
		return model.TxID{}, err
	}
	return model.TxID(crypto.HashBytes(preimage)), nil
}

func tradeIntentPreimage(intent TradeIntent) ([]byte, error) {
	return canonical.EncodeFields(
		canonical.Field{Name: "kind", Value: "trade_intent"},
		canonical.Field{Name: "party", Value: intent.Party.Bytes()},
		canonical.Field{Name: "seller", Value: intent.Seller.Bytes()},
		canonical.Field{Name: "buyer", Value: intent.Buyer.Bytes()},
		canonical.Field{Name: "sell_unit", Value: hashBytes(intent.SellUnit)},
		canonical.Field{Name: "buy_unit", Value: hashBytes(intent.BuyUnit)},
		canonical.Field{Name: "sell_amount", Value: intent.SellAmount},
		canonical.Field{Name: "buy_amount", Value: intent.BuyAmount},
		canonical.Field{Name: "timestamp", Value: intent.Timestamp},
	)
}

func intentMatchesQuote(intent TradeIntent, q Quote, party model.NodeID) bool {
	return intent.Party == party &&
		intent.Seller == q.Seller &&
		intent.Buyer == q.Buyer &&
		intent.SellUnit == q.SellUnit &&
		intent.BuyUnit == q.BuyUnit &&
		intent.SellAmount == q.SellAmount &&
		intent.BuyAmount == q.BuyAmount &&
		(q.Timestamp == 0 || intent.Timestamp == q.Timestamp)
}

func economicTermsMatch(a TradeIntent, b TradeIntent) bool {
	return a.Seller == b.Seller &&
		a.Buyer == b.Buyer &&
		a.SellUnit == b.SellUnit &&
		a.BuyUnit == b.BuyUnit &&
		a.SellAmount == b.SellAmount &&
		a.BuyAmount == b.BuyAmount &&
		a.Timestamp == b.Timestamp
}

func publicKeyMatchesNode(pub crypto.PublicKey, id model.NodeID) bool {
	nodeID, err := model.NodeIDFromPublicKey(pub)
	return err == nil && nodeID == id
}

func hashBytes[T ~[32]byte](h T) []byte {
	b := make([]byte, 32)
	copy(b, h[:])
	return b
}

func samePublicKey(a crypto.PublicKey, b crypto.PublicKey) bool {
	return bytes.Equal(a, b)
}

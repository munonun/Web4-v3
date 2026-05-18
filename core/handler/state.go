package handler

import (
	"bytes"
	"fmt"

	"web4-v3/core/canonical"
	"web4-v3/core/crypto"
	"web4-v3/core/message"
	"web4-v3/core/model"
	"web4-v3/core/node"
)

func quoteFromResponse(p message.QuoteResponsePayload) node.Quote {
	return node.Quote{
		Seller:     p.Seller,
		Buyer:      p.Buyer,
		SellUnit:   p.SellUnit,
		BuyUnit:    p.BuyUnit,
		SellAmount: p.SellAmount,
		BuyAmount:  p.BuyAmount,
		SellerAsk:  p.SellerAsk,
		BuyerBid:   p.BuyerBid,
		Executable: p.Executable,
		Reason:     p.Reason,
		Timestamp:  p.ExpiryUnix,
	}
}

func quoteFromIntent(intent node.TradeIntent) node.Quote {
	return node.Quote{
		Seller:     intent.Seller,
		Buyer:      intent.Buyer,
		SellUnit:   intent.SellUnit,
		BuyUnit:    intent.BuyUnit,
		SellAmount: intent.SellAmount,
		BuyAmount:  intent.BuyAmount,
		SellerAsk:  model.ToFloat(intent.BuyAmount),
		BuyerBid:   model.ToFloat(intent.BuyAmount),
		Executable: true,
		Reason:     "authorized",
		Timestamp:  intent.Timestamp,
	}
}

func quoteResponseFromQuote(requestID model.TxID, q node.Quote) (message.QuoteResponsePayload, error) {
	p := message.QuoteResponsePayload{
		RequestID:  requestID,
		Seller:     q.Seller,
		Buyer:      q.Buyer,
		SellUnit:   q.SellUnit,
		BuyUnit:    q.BuyUnit,
		SellAmount: q.SellAmount,
		BuyAmount:  q.BuyAmount,
		SellerAsk:  q.SellerAsk,
		BuyerBid:   q.BuyerBid,
		Executable: q.Executable,
		Reason:     q.Reason,
		ExpiryUnix: q.Timestamp,
	}
	id, err := quoteID(p)
	if err != nil {
		return message.QuoteResponsePayload{}, err
	}
	p.QuoteID = id
	return p, nil
}

func quoteID(p message.QuoteResponsePayload) (model.TxID, error) {
	preimage, err := canonical.EncodeFields(
		canonical.Field{Name: "kind", Value: "quote_response"},
		canonical.Field{Name: "request_id", Value: hashBytes(p.RequestID)},
		canonical.Field{Name: "seller", Value: p.Seller.Bytes()},
		canonical.Field{Name: "buyer", Value: p.Buyer.Bytes()},
		canonical.Field{Name: "sell_unit", Value: hashBytes(p.SellUnit)},
		canonical.Field{Name: "buy_unit", Value: hashBytes(p.BuyUnit)},
		canonical.Field{Name: "sell_amount", Value: p.SellAmount},
		canonical.Field{Name: "buy_amount", Value: p.BuyAmount},
		canonical.Field{Name: "seller_ask", Value: fmt.Sprintf("%.17g", p.SellerAsk)},
		canonical.Field{Name: "buyer_bid", Value: fmt.Sprintf("%.17g", p.BuyerBid)},
		canonical.Field{Name: "executable", Value: boolString(p.Executable)},
		canonical.Field{Name: "reason", Value: p.Reason},
		canonical.Field{Name: "expiry_unix", Value: p.ExpiryUnix},
	)
	if err != nil {
		return model.TxID{}, err
	}
	return model.TxID(crypto.HashBytes(preimage)), nil
}

func buildAuthorizedTrade(q node.Quote, sellerSig, buyerSig node.SignedTradeIntent) (node.AuthorizedTradeTx, model.TxID, error) {
	tx, err := buildTradeTx(q)
	if err != nil {
		return node.AuthorizedTradeTx{}, model.TxID{}, err
	}
	auth := node.AuthorizedTradeTx{Tx: tx, SellerAuth: sellerSig, BuyerAuth: buyerSig}
	id, err := node.AuthorizedTradeID(auth)
	if err != nil {
		return node.AuthorizedTradeTx{}, model.TxID{}, err
	}
	return auth, id, nil
}

func buildTradeTx(q node.Quote) (model.TradeTx, error) {
	inputSeller, err := tradeValue(q.SellUnit, q.SellAmount, q.Seller, q.Timestamp)
	if err != nil {
		return model.TradeTx{}, err
	}
	inputBuyer, err := tradeValue(q.BuyUnit, q.BuyAmount, q.Buyer, q.Timestamp+1)
	if err != nil {
		return model.TradeTx{}, err
	}
	outputSeller, err := tradeValue(q.BuyUnit, q.BuyAmount, q.Seller, q.Timestamp+2)
	if err != nil {
		return model.TradeTx{}, err
	}
	outputBuyer, err := tradeValue(q.SellUnit, q.SellAmount, q.Buyer, q.Timestamp+3)
	if err != nil {
		return model.TradeTx{}, err
	}
	tx := model.TradeTx{
		InputsA:   []model.Value{inputSeller},
		InputsB:   []model.Value{inputBuyer},
		OutputsA:  []model.Value{outputSeller},
		OutputsB:  []model.Value{outputBuyer},
		PartyA:    q.Seller,
		PartyB:    q.Buyer,
		Timestamp: q.Timestamp,
	}
	id, err := model.TradeTxID(tx)
	if err != nil {
		return model.TradeTx{}, err
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

func intentMatchesQuote(intent node.TradeIntent, quote message.QuoteResponsePayload) bool {
	return intent.Seller == quote.Seller &&
		intent.Buyer == quote.Buyer &&
		intent.SellUnit == quote.SellUnit &&
		intent.BuyUnit == quote.BuyUnit &&
		intent.SellAmount == quote.SellAmount &&
		intent.BuyAmount == quote.BuyAmount &&
		intent.Timestamp == quote.ExpiryUnix
}

func isExpired(expiryUnix, nowUnix int64) bool {
	return expiryUnix > 0 && nowUnix > expiryUnix
}

func validateQuoteFreshness(req *message.QuoteRequestPayload, q message.QuoteResponsePayload, nowUnix int64) error {
	if req != nil {
		if isExpired(req.ExpiryUnix, nowUnix) {
			return fmt.Errorf("quote request is expired")
		}
		if req.ExpiryUnix > 0 {
			if q.ExpiryUnix == 0 {
				return fmt.Errorf("quote response removes request expiry")
			}
			if q.ExpiryUnix > req.ExpiryUnix {
				return fmt.Errorf("quote response extends request expiry")
			}
		}
	}
	if isExpired(q.ExpiryUnix, nowUnix) {
		return fmt.Errorf("quote response is expired")
	}
	return nil
}

func validateQuoteMatchesRequest(q message.QuoteResponsePayload, req message.QuoteRequestPayload, nowUnix int64) error {
	if q.RequestID != req.RequestID ||
		q.Seller != req.Seller ||
		q.Buyer != req.Buyer ||
		q.SellUnit != req.SellUnit ||
		q.BuyUnit != req.BuyUnit ||
		q.SellAmount != req.SellAmount {
		return fmt.Errorf("quote response does not match request")
	}
	if err := validateQuoteFreshness(&req, q, nowUnix); err != nil {
		return err
	}
	if req.SpreadLimit < 0 {
		return fmt.Errorf("quote request spread limit is invalid")
	}
	if q.Executable {
		if q.SellerAsk <= 0 || q.BuyerBid <= 0 {
			return fmt.Errorf("executable quote prices must be greater than zero")
		}
		if q.BuyerBid < q.SellerAsk {
			return fmt.Errorf("quote response bid is below ask")
		}
		spread := (q.BuyerBid - q.SellerAsk) / q.SellerAsk
		if spread > req.SpreadLimit+quoteSpreadSlack() {
			return fmt.Errorf("quote response exceeds request spread limit")
		}
	}
	return nil
}

func selectIntents(intents []node.SignedTradeIntent, seller, buyer model.NodeID) (node.SignedTradeIntent, node.SignedTradeIntent, bool) {
	var sellerSig node.SignedTradeIntent
	var buyerSig node.SignedTradeIntent
	for _, intent := range intents {
		switch intent.Intent.Party {
		case seller:
			sellerSig = intent
		case buyer:
			buyerSig = intent
		}
	}
	return sellerSig, buyerSig, sellerSig.Signature != nil && buyerSig.Signature != nil
}

func hasPartyIntent(intents []node.SignedTradeIntent, party model.NodeID) bool {
	for _, intent := range intents {
		if intent.Intent.Party == party {
			return true
		}
	}
	return false
}

func hasSignedIntent(intents []node.SignedTradeIntent, target node.SignedTradeIntent) bool {
	for _, intent := range intents {
		if signedIntentEqual(intent, target) {
			return true
		}
	}
	return false
}

func signedIntentEqual(a node.SignedTradeIntent, b node.SignedTradeIntent) bool {
	return a.Intent == b.Intent &&
		bytes.Equal(a.PublicKey, b.PublicKey) &&
		bytes.Equal(a.Signature, b.Signature)
}

func authorizedTradeEqual(a node.AuthorizedTradeTx, b node.AuthorizedTradeTx) bool {
	aID, errA := node.AuthorizedTradeID(a)
	bID, errB := node.AuthorizedTradeID(b)
	return errA == nil && errB == nil && aID == bID
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func quoteSpreadSlack() float64 {
	return 1.0 / float64(model.AmountScale)
}

func hashBytes[T ~[32]byte](h T) []byte {
	b := make([]byte, 32)
	copy(b, h[:])
	return b
}

package message

import (
	"bytes"
	"testing"

	"web4-v3/core/crypto"
	"web4-v3/core/model"
	"web4-v3/core/node"
	"web4-v3/core/price"
)

func TestMessageTypes(t *testing.T) {
	for _, typ := range []MessageType{
		TypeHello,
		TypeQuoteRequest,
		TypeQuoteResponse,
		TypeSignedIntent,
		TypeAuthorizedTrade,
		TypeTradeResult,
		TypeReject,
		TypePing,
		TypePong,
	} {
		if !IsValidMessageType(typ) {
			t.Fatalf("%s should be valid", typ)
		}
	}
	if IsValidMessageType(MessageType("MISSING")) {
		t.Fatal("unknown type was valid")
	}
}

func TestPayloadEncodingDeterministicAndHashSensitive(t *testing.T) {
	payload := testHelloPayload(t)
	payload.Features = []string{"beta", "alpha"}

	a, err := EncodePayload(TypeHello, payload)
	if err != nil {
		t.Fatalf("encode A: %v", err)
	}
	b, err := EncodePayload(TypeHello, payload)
	if err != nil {
		t.Fatalf("encode B: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("same payload encoded differently")
	}

	hashA, err := PayloadHash(TypePing, PingPayload{TimeUnix: 10})
	if err != nil {
		t.Fatalf("hash A: %v", err)
	}
	hashB, err := PayloadHash(TypePing, PingPayload{TimeUnix: 11})
	if err != nil {
		t.Fatalf("hash B: %v", err)
	}
	if hashA == hashB {
		t.Fatal("changed payload did not change hash")
	}
}

func TestPayloadEncodingRejectsWrongTypeAndMalformedDecode(t *testing.T) {
	if _, err := EncodePayload(TypeHello, PingPayload{TimeUnix: 1}); err == nil {
		t.Fatal("wrong payload type encoded")
	}
	ping, err := EncodePayload(TypePing, PingPayload{TimeUnix: 1})
	if err != nil {
		t.Fatalf("encode ping: %v", err)
	}
	if _, err := DecodePayload(TypePong, ping); err == nil {
		t.Fatal("decoded payload under wrong message type")
	}
	if _, err := DecodePayload(TypePing, []byte("{bad")); err == nil {
		t.Fatal("malformed payload decoded")
	}
}

func TestSignAndVerifyEnvelope(t *testing.T) {
	pub, priv, _ := testKey(t)
	nonce := testNonce(7)
	env, payloadBytes, err := SignEnvelope(priv, TypePing, PingPayload{TimeUnix: 10}, 100, nonce)
	if err != nil {
		t.Fatalf("sign envelope: %v", err)
	}

	if err := VerifyEnvelope(env, payloadBytes, pub); err != nil {
		t.Fatalf("verify envelope: %v", err)
	}
}

func TestVerifyEnvelopeRejectsUnsupportedVersion(t *testing.T) {
	pub, priv, _ := testKey(t)
	env, payloadBytes, err := SignEnvelope(priv, TypePing, PingPayload{TimeUnix: 10}, 100, testNonce(7))
	if err != nil {
		t.Fatalf("sign envelope: %v", err)
	}
	if err := VerifyEnvelope(env, payloadBytes, pub); err != nil {
		t.Fatalf("current version rejected: %v", err)
	}

	versionZero := env
	versionZero.Version = 0
	if err := VerifyEnvelope(versionZero, payloadBytes, pub); err == nil {
		t.Fatal("version 0 verified")
	}

	versionTwo := env
	versionTwo.Version = CurrentVersion + 1
	versionTwo.MessageID, err = MessageID(versionTwo.Version, versionTwo.Type, versionTwo.Sender, versionTwo.Timestamp, versionTwo.Nonce, versionTwo.PayloadHash)
	if err != nil {
		t.Fatalf("message id v2: %v", err)
	}
	versionTwo.Signature = nil
	preimage, err := EnvelopePreimage(versionTwo)
	if err != nil {
		t.Fatalf("preimage v2: %v", err)
	}
	versionTwo.Signature, err = crypto.Sign(priv, preimage)
	if err != nil {
		t.Fatalf("sign v2: %v", err)
	}
	if err := VerifyEnvelope(versionTwo, payloadBytes, pub); err == nil {
		t.Fatal("unsupported version 2 verified")
	}
}

func TestVerifyEnvelopeRejectsTampering(t *testing.T) {
	pub, priv, _ := testKey(t)
	env, payloadBytes, err := SignEnvelope(priv, TypePing, PingPayload{TimeUnix: 10}, 100, testNonce(8))
	if err != nil {
		t.Fatalf("sign envelope: %v", err)
	}

	tamperedPayload, err := EncodePayload(TypePing, PingPayload{TimeUnix: 11})
	if err != nil {
		t.Fatalf("encode tampered payload: %v", err)
	}
	if err := VerifyEnvelope(env, tamperedPayload, pub); err == nil {
		t.Fatal("tampered payload verified")
	}

	tamperedType := env
	tamperedType.Type = TypePong
	if err := VerifyEnvelope(tamperedType, payloadBytes, pub); err == nil {
		t.Fatal("tampered type verified")
	}

	tamperedTimestamp := env
	tamperedTimestamp.Timestamp = 0
	if err := VerifyEnvelope(tamperedTimestamp, payloadBytes, pub); err == nil {
		t.Fatal("tampered timestamp verified")
	}
}

func TestSignEnvelopeRejectsZeroTimestampAndNonce(t *testing.T) {
	_, priv, _ := testKey(t)
	if _, _, err := SignEnvelope(priv, TypePing, PingPayload{TimeUnix: 10}, 0, testNonce(1)); err == nil {
		t.Fatal("zero timestamp signed")
	}
	if _, _, err := SignEnvelope(priv, TypePing, PingPayload{TimeUnix: 10}, 10, [24]byte{}); err == nil {
		t.Fatal("zero nonce signed")
	}
}

func TestVerifyEnvelopeRejectsWrongPublicKey(t *testing.T) {
	_, priv, _ := testKey(t)
	wrongPub, _, _ := testKey(t)
	env, payloadBytes, err := SignEnvelope(priv, TypePing, PingPayload{TimeUnix: 10}, 100, testNonce(9))
	if err != nil {
		t.Fatalf("sign envelope: %v", err)
	}
	if err := VerifyEnvelope(env, payloadBytes, wrongPub); err == nil {
		t.Fatal("wrong public key verified")
	}
}

func TestMessageIDStableAndNonceSensitive(t *testing.T) {
	_, _, sender := testKey(t)
	payloadHash, err := PayloadHash(TypePing, PingPayload{TimeUnix: 10})
	if err != nil {
		t.Fatalf("payload hash: %v", err)
	}
	a, err := MessageID(CurrentVersion, TypePing, sender, 100, testNonce(1), payloadHash)
	if err != nil {
		t.Fatalf("message id A: %v", err)
	}
	b, err := MessageID(CurrentVersion, TypePing, sender, 100, testNonce(1), payloadHash)
	if err != nil {
		t.Fatalf("message id B: %v", err)
	}
	if a != b {
		t.Fatal("same fields produced different message IDs")
	}
	c, err := MessageID(CurrentVersion, TypePing, sender, 100, testNonce(2), payloadHash)
	if err != nil {
		t.Fatalf("message id C: %v", err)
	}
	if a == c {
		t.Fatal("nonce change did not change message ID")
	}
}

func TestPayloadSemanticsHelloMismatchRejected(t *testing.T) {
	payload := testHelloPayload(t)
	_, _, other := testKey(t)
	payload.NodeID = other

	if err := ValidatePayloadSemantics(TypeHello, payload); err == nil {
		t.Fatal("hello node/public key mismatch accepted")
	}
}

func TestPayloadSemanticsSignedIntentAndAuthorizedTrade(t *testing.T) {
	seller, buyer, sellUnit, buyUnit := testTradeNodes(t)
	q := seller.QuoteSell(buyer, sellUnit, buyUnit, model.FromFloat(2), 0)
	sellerSig, err := seller.SignQuote(q)
	if err != nil {
		t.Fatalf("seller sign quote: %v", err)
	}
	if err := ValidatePayloadSemantics(TypeSignedIntent, SignedIntentPayload{QuoteID: testTxID(1), Intent: sellerSig}); err != nil {
		t.Fatalf("signed intent semantics: %v", err)
	}

	buyerSig, err := buyer.SignQuote(q)
	if err != nil {
		t.Fatalf("buyer sign quote: %v", err)
	}
	tx, err := node.ExecuteSignedTrade(seller, buyer, q, sellerSig, buyerSig)
	if err != nil {
		t.Fatalf("execute signed trade: %v", err)
	}
	auth := node.AuthorizedTradeTx{Tx: *tx, SellerAuth: sellerSig, BuyerAuth: buyerSig}
	authID, err := node.AuthorizedTradeID(auth)
	if err != nil {
		t.Fatalf("authorized trade id: %v", err)
	}
	if err := ValidatePayloadSemantics(TypeAuthorizedTrade, AuthorizedTradePayload{AuthorizedTrade: auth, AuthorizedTradeID: authID}); err != nil {
		t.Fatalf("authorized trade semantics: %v", err)
	}
	if err := ValidatePayloadSemantics(TypeAuthorizedTrade, AuthorizedTradePayload{AuthorizedTrade: auth, AuthorizedTradeID: testTxID(99)}); err == nil {
		t.Fatal("authorized trade id mismatch accepted")
	}
}

func TestEncodeDecodeMessageRoundtripAndCorruption(t *testing.T) {
	pub, priv, _ := testKey(t)
	env, payloadBytes, err := SignEnvelope(priv, TypePing, PingPayload{TimeUnix: 10}, 100, testNonce(3))
	if err != nil {
		t.Fatalf("sign envelope: %v", err)
	}
	encoded, err := EncodeMessage(Message{Envelope: env, PayloadBytes: payloadBytes})
	if err != nil {
		t.Fatalf("encode message: %v", err)
	}
	decoded, err := DecodeMessage(encoded)
	if err != nil {
		t.Fatalf("decode message: %v", err)
	}
	if decoded.Envelope.MessageID != env.MessageID || !bytes.Equal(decoded.PayloadBytes, payloadBytes) {
		t.Fatalf("bad roundtrip: %+v", decoded)
	}
	if err := VerifyEnvelope(decoded.Envelope, decoded.PayloadBytes, pub); err != nil {
		t.Fatalf("verify decoded envelope: %v", err)
	}

	encoded[len(encoded)-1] ^= 0xff
	if _, err := DecodeMessage(encoded); err == nil {
		t.Fatal("corrupted message decoded")
	}
}

func testHelloPayload(t *testing.T) HelloPayload {
	t.Helper()
	pub, _, id := testKey(t)
	return HelloPayload{
		NodeID:            id,
		PublicKey:         pub,
		SupportedVersions: []uint16{CurrentVersion},
		Features:          []string{"quotes", "trades"},
	}
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

func testTradeNodes(t *testing.T) (*node.Node, *node.Node, model.UnitID, model.UnitID) {
	t.Helper()
	seller := testNode(t, 100)
	buyer := testNode(t, 100)
	sellUnit := testUnit(t, seller.ID, "SKUG")
	buyUnit := testUnit(t, buyer.ID, "WEB4")
	configureUnit(t, seller, sellUnit)
	configureUnit(t, seller, buyUnit)
	configureUnit(t, buyer, sellUnit)
	configureUnit(t, buyer, buyUnit)
	seller.AddInventory(sellUnit, model.FromFloat(10))
	buyer.AddInventory(buyUnit, model.FromFloat(10))
	return seller, buyer, sellUnit, buyUnit
}

func testNode(t *testing.T, now int64) *node.Node {
	t.Helper()
	_, priv, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate node keypair: %v", err)
	}
	n, err := node.NewNode(priv, node.DefaultPriceConfig())
	if err != nil {
		t.Fatalf("new node: %v", err)
	}
	n.NowUnix = func() int64 { return now }
	n.AllowEphemeralReplayUnsafe = true
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

func configureUnit(t *testing.T, n *node.Node, unit model.UnitID) {
	t.Helper()
	n.Features[unit] = price.AssetFeatures{Cost: 1}
	n.PriceConfig = price.PriceConfig{
		BasePrice:       1,
		Weights:         price.FeatureWeights{Cost: 1},
		VolumeThreshold: model.FromFloat(10),
	}
	n.ComputePrice(unit)
}

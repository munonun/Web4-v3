package transport

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"web4-v3/core/crypto"
	"web4-v3/core/handler"
	"web4-v3/core/message"
	"web4-v3/core/model"
	"web4-v3/core/node"
)

func TestEncodeDecodeFrameRoundtripAndHeader(t *testing.T) {
	pub, priv, _ := testKey(t)
	msg := signedMessage(t, priv, message.TypePing, message.PingPayload{TimeUnix: 10}, 100, testNonce(1))

	frame, err := EncodeFrame(msg)
	if err != nil {
		t.Fatalf("encode frame: %v", err)
	}
	payload, err := message.EncodeMessage(msg)
	if err != nil {
		t.Fatalf("encode message: %v", err)
	}
	if string(frame[:4]) != FrameMagic {
		t.Fatalf("magic %q, want %q", string(frame[:4]), FrameMagic)
	}
	if version := binary.BigEndian.Uint16(frame[4:6]); version != FrameVersion {
		t.Fatalf("version %d, want %d", version, FrameVersion)
	}
	if length := binary.BigEndian.Uint32(frame[6:10]); length != uint32(len(payload)) {
		t.Fatalf("length %d, want %d", length, len(payload))
	}

	decoded, err := DecodeFrame(frame)
	if err != nil {
		t.Fatalf("decode frame: %v", err)
	}
	if decoded.Envelope.MessageID != msg.Envelope.MessageID || !bytes.Equal(decoded.PayloadBytes, msg.PayloadBytes) {
		t.Fatalf("bad decoded message: %+v", decoded)
	}
	if err := message.VerifyEnvelope(decoded.Envelope, decoded.PayloadBytes, pub); err != nil {
		t.Fatalf("verify decoded envelope: %v", err)
	}
}

func TestEncodePayloadFrameRejectsEmptyAndOversized(t *testing.T) {
	if _, err := EncodePayloadFrame(nil); !errors.Is(err, ErrEmptyFrame) {
		t.Fatalf("empty payload error %v, want ErrEmptyFrame", err)
	}
	if _, err := EncodePayloadFrame(make([]byte, MaxFrameSize+1)); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("oversized payload error %v, want ErrFrameTooLarge", err)
	}
}

func TestDecodePayloadFrameFailures(t *testing.T) {
	valid, err := EncodePayloadFrame([]byte("ok"))
	if err != nil {
		t.Fatalf("encode payload frame: %v", err)
	}

	badMagic := append([]byte(nil), valid...)
	copy(badMagic[:4], []byte("NOPE"))
	if _, err := DecodePayloadFrame(badMagic); !errors.Is(err, ErrBadMagic) {
		t.Fatalf("bad magic error %v, want ErrBadMagic", err)
	}

	badVersion := append([]byte(nil), valid...)
	binary.BigEndian.PutUint16(badVersion[4:6], FrameVersion+1)
	if _, err := DecodePayloadFrame(badVersion); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("bad version error %v, want ErrUnsupportedVersion", err)
	}

	zeroLength := append([]byte(nil), valid...)
	binary.BigEndian.PutUint32(zeroLength[6:10], 0)
	if _, err := DecodePayloadFrame(zeroLength); !errors.Is(err, ErrEmptyFrame) {
		t.Fatalf("zero length error %v, want ErrEmptyFrame", err)
	}

	tooLarge := append([]byte(nil), valid[:FrameHeaderSize]...)
	binary.BigEndian.PutUint32(tooLarge[6:10], uint32(MaxFrameSize+1))
	if _, err := DecodePayloadFrame(tooLarge); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("large length error %v, want ErrFrameTooLarge", err)
	}

	if _, err := DecodePayloadFrame(valid[:FrameHeaderSize-1]); !errors.Is(err, ErrTruncatedFrame) {
		t.Fatalf("truncated header error %v, want ErrTruncatedFrame", err)
	}

	if _, err := DecodePayloadFrame(valid[:len(valid)-1]); !errors.Is(err, ErrTruncatedFrame) {
		t.Fatalf("truncated payload error %v, want ErrTruncatedFrame", err)
	}

	withTrailing := append(append([]byte(nil), valid...), 0)
	if _, err := DecodePayloadFrame(withTrailing); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("trailing bytes error %v, want ErrInvalidFrame", err)
	}
}

func TestDecodeFrameRejectsCorruptedMessagePayload(t *testing.T) {
	frame, err := EncodePayloadFrame([]byte("{bad"))
	if err != nil {
		t.Fatalf("encode bad message payload frame: %v", err)
	}
	if _, err := DecodeFrame(frame); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("corrupted message error %v, want ErrInvalidFrame", err)
	}
}

func TestReadFrameHandlesPartialReader(t *testing.T) {
	msg := signedMessage(t, testPrivateKey(t), message.TypePing, message.PingPayload{TimeUnix: 20}, 200, testNonce(2))
	frame, err := EncodeFrame(msg)
	if err != nil {
		t.Fatalf("encode frame: %v", err)
	}
	decoded, err := ReadFrame(&slowReader{data: frame, chunk: 3})
	if err != nil {
		t.Fatalf("read partial frame: %v", err)
	}
	if decoded.Envelope.MessageID != msg.Envelope.MessageID {
		t.Fatal("partial reader decoded wrong message")
	}
}

func TestReadFrameRejectsTruncatedPayload(t *testing.T) {
	frame, err := EncodePayloadFrame([]byte("payload"))
	if err != nil {
		t.Fatalf("encode payload frame: %v", err)
	}
	if _, err := ReadPayloadFrame(bytes.NewReader(frame[:len(frame)-1])); !errors.Is(err, ErrTruncatedFrame) {
		t.Fatalf("truncated read error %v, want ErrTruncatedFrame", err)
	}
}

func TestWriteFrameReadFrameRoundtrip(t *testing.T) {
	msg := signedMessage(t, testPrivateKey(t), message.TypePing, message.PingPayload{TimeUnix: 30}, 300, testNonce(3))
	var pipe MemoryPipe
	if err := pipe.Write(msg); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	decoded, err := pipe.Read()
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if decoded.Envelope.MessageID != msg.Envelope.MessageID {
		t.Fatal("memory pipe decoded wrong message")
	}
}

func TestHandlerHelloFrameRoundtrip(t *testing.T) {
	local := testNode(t, 100)
	peer := testNode(t, 200)
	h := testHandler(t, local, peer)
	unit := testUnit(t, local.ID, "local")
	local.AddInventory(unit, model.FromFloat(5))
	before := local.Balance(unit)
	payload := message.HelloPayload{
		NodeID:            peer.ID,
		PublicKey:         peer.PublicKey,
		SupportedVersions: []uint16{message.CurrentVersion},
		Features:          []string{"frames"},
	}
	msg := signedMessage(t, peer.PrivateKey, message.TypeHello, payload, 200, testNonce(4))

	frame, err := EncodeFrame(msg)
	if err != nil {
		t.Fatalf("encode hello frame: %v", err)
	}
	decoded, err := DecodeFrame(frame)
	if err != nil {
		t.Fatalf("decode hello frame: %v", err)
	}
	resp, err := h.HandleMessage(decoded, peer.PublicKey)
	if err != nil {
		t.Fatalf("handle hello: %v", err)
	}
	if resp != nil {
		t.Fatalf("hello response %+v, want nil", resp)
	}
	if local.Balance(unit) != before {
		t.Fatal("transport roundtrip mutated inventory")
	}
}

func TestHandlerPingFrameProducesPong(t *testing.T) {
	local := testNode(t, 100)
	peer := testNode(t, 200)
	h := testHandler(t, local, peer)
	msg := signedMessage(t, peer.PrivateKey, message.TypePing, message.PingPayload{TimeUnix: 200}, 200, testNonce(5))

	frame, err := EncodeFrame(msg)
	if err != nil {
		t.Fatalf("encode ping frame: %v", err)
	}
	decoded, err := DecodeFrame(frame)
	if err != nil {
		t.Fatalf("decode ping frame: %v", err)
	}
	resp, err := h.HandleMessage(decoded, peer.PublicKey)
	if err != nil {
		t.Fatalf("handle ping: %v", err)
	}
	if resp == nil || resp.Envelope.Type != message.TypePong {
		t.Fatalf("response %+v, want PONG", resp)
	}
	respFrame, err := EncodeFrame(*resp)
	if err != nil {
		t.Fatalf("encode pong frame: %v", err)
	}
	decodedResp, err := DecodeFrame(respFrame)
	if err != nil {
		t.Fatalf("decode pong frame: %v", err)
	}
	if decodedResp.Envelope.Type != message.TypePong {
		t.Fatalf("decoded response type %s, want PONG", decodedResp.Envelope.Type)
	}
}

type slowReader struct {
	data  []byte
	chunk int
}

func (r *slowReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := r.chunk
	if n > len(r.data) {
		n = len(r.data)
	}
	if n > len(p) {
		n = len(p)
	}
	copy(p, r.data[:n])
	r.data = r.data[n:]
	return n, nil
}

func signedMessage(t *testing.T, priv crypto.PrivateKey, typ message.MessageType, payload any, timestamp int64, nonce [24]byte) message.Message {
	t.Helper()
	env, payloadBytes, err := message.SignEnvelope(priv, typ, payload, timestamp, nonce)
	if err != nil {
		t.Fatalf("sign message: %v", err)
	}
	return message.Message{Envelope: env, PayloadBytes: payloadBytes}
}

func testHandler(t *testing.T, local *node.Node, peer *node.Node) *handler.Handler {
	t.Helper()
	h, err := handler.NewHandler(local, handler.NewSession(local.ID, peer.ID))
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	return h
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

func testPrivateKey(t *testing.T) crypto.PrivateKey {
	t.Helper()
	_, priv, _ := testKey(t)
	return priv
}

func testUnit(t *testing.T, issuer model.NodeID, metadata string) model.UnitID {
	t.Helper()
	unit, err := model.NewUnitIDFromMetadata(issuer, []byte(metadata))
	if err != nil {
		t.Fatalf("unit id: %v", err)
	}
	return unit
}

func testNonce(seed byte) [24]byte {
	var nonce [24]byte
	for i := range nonce {
		nonce[i] = seed + byte(i)
	}
	return nonce
}

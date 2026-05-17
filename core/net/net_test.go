package net

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	stdnet "net"

	"web4-v3/core/crypto"
	"web4-v3/core/message"
	"web4-v3/core/model"
	"web4-v3/core/node"
	"web4-v3/core/price"
	"web4-v3/core/transport"
)

func TestPeerRuntimePingPongNetPipe(t *testing.T) {
	server := testNode(t, 100)
	client := testNode(t, 200)
	clientConn, serverConn := stdnet.Pipe()
	defer clientConn.Close()

	errCh := serveOne(t, server, client.PublicKey, serverConn)
	ping := signedMessage(t, client, message.TypePing, message.PingPayload{TimeUnix: 200}, testNonce(1))
	if err := transport.WriteFrame(clientConn, ping); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	resp, err := transport.ReadFrame(clientConn)
	if err != nil {
		t.Fatalf("read pong: %v", err)
	}
	if err := message.VerifyEnvelope(resp.Envelope, resp.PayloadBytes, server.PublicKey); err != nil {
		t.Fatalf("verify pong: %v", err)
	}
	pong := decodePayload[message.PongPayload](t, message.TypePong, resp.PayloadBytes)
	if pong.PingTimeUnix != 200 || pong.TimeUnix != 100 {
		t.Fatalf("pong %+v, want ping=200 time=100", pong)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("serve ping: %v", err)
	}
}

func TestPeerRuntimeHelloAcceptedAndWrongPeerKeyRejected(t *testing.T) {
	server := testNode(t, 100)
	client := testNode(t, 200)

	clientConn, serverConn := stdnet.Pipe()
	hello := signedMessage(t, client, message.TypeHello, message.HelloPayload{
		NodeID:            client.ID,
		PublicKey:         client.PublicKey,
		SupportedVersions: []uint16{message.CurrentVersion},
		Features:          []string{"tcp"},
	}, testNonce(2))
	errCh := serveOne(t, server, client.PublicKey, serverConn)
	if err := transport.WriteFrame(clientConn, hello); err != nil {
		t.Fatalf("write hello: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("serve hello: %v", err)
	}
	_ = clientConn.Close()

	wrongPeer := testNode(t, 300)
	clientConn, serverConn = stdnet.Pipe()
	defer clientConn.Close()
	errCh = serveOne(t, server, wrongPeer.PublicKey, serverConn)
	if err := transport.WriteFrame(clientConn, hello); err != nil {
		t.Fatalf("write wrong-key hello: %v", err)
	}
	if err := <-errCh; err == nil {
		t.Fatal("wrong peer key was accepted")
	}
}

func TestPeerRuntimeQuoteRequestReturnsQuoteResponse(t *testing.T) {
	seller := testNode(t, 100)
	buyer := testNode(t, 200)
	sellUnit := testUnit(t, seller.ID, "SKUG")
	buyUnit := testUnit(t, buyer.ID, "WEB4")
	configureUnit(t, seller, sellUnit)
	configureUnit(t, seller, buyUnit)
	seller.AddInventory(sellUnit, model.FromFloat(10))

	clientConn, serverConn := stdnet.Pipe()
	defer clientConn.Close()
	errCh := serveOne(t, seller, buyer.PublicKey, serverConn)

	req := message.QuoteRequestPayload{
		RequestID:   testTxID(9),
		Seller:      seller.ID,
		Buyer:       buyer.ID,
		SellUnit:    sellUnit,
		BuyUnit:     buyUnit,
		SellAmount:  model.FromFloat(2),
		SpreadLimit: 0,
		ExpiryUnix:  100,
	}
	msg := signedMessage(t, buyer, message.TypeQuoteRequest, req, testNonce(3))
	if err := transport.WriteFrame(clientConn, msg); err != nil {
		t.Fatalf("write quote request: %v", err)
	}
	resp, err := transport.ReadFrame(clientConn)
	if err != nil {
		t.Fatalf("read quote response: %v", err)
	}
	if err := message.VerifyEnvelope(resp.Envelope, resp.PayloadBytes, seller.PublicKey); err != nil {
		t.Fatalf("verify quote response: %v", err)
	}
	if resp.Envelope.Type != message.TypeQuoteResponse {
		t.Fatalf("response type %s, want QUOTE_RESPONSE", resp.Envelope.Type)
	}
	payload := decodePayload[message.QuoteResponsePayload](t, message.TypeQuoteResponse, resp.PayloadBytes)
	if payload.RequestID != req.RequestID || payload.Seller != seller.ID || payload.Buyer != buyer.ID {
		t.Fatalf("bad quote response: %+v", payload)
	}
	if !payload.Executable {
		t.Fatalf("quote response not executable: %+v", payload)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("serve quote request: %v", err)
	}
}

func TestMalformedFrameFailsClosedWithoutInventoryMutation(t *testing.T) {
	server := testNode(t, 100)
	client := testNode(t, 200)
	unit := testUnit(t, server.ID, "SKUG")
	server.AddInventory(unit, model.FromFloat(5))
	before := server.Balance(unit)

	clientConn, serverConn := stdnet.Pipe()
	defer clientConn.Close()
	errCh := serveOne(t, server, client.PublicKey, serverConn)

	malformed := append([]byte("NOPE"), make([]byte, transport.FrameHeaderSize-4)...)
	if _, err := clientConn.Write(malformed); err != nil {
		t.Fatalf("write malformed frame: %v", err)
	}
	if err := <-errCh; err == nil {
		t.Fatal("malformed frame was accepted")
	}
	if server.Balance(unit) != before {
		t.Fatal("malformed frame mutated inventory")
	}
	if _, err := clientConn.Write([]byte{1}); err == nil {
		t.Fatal("connection remained writable after malformed frame")
	}
}

func TestServeContextCancellationAndReadTimeout(t *testing.T) {
	server := testNode(t, 100)
	client := testNode(t, 200)
	clientConn, serverConn := stdnet.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		runtime := &PeerRuntime{
			Node:          server,
			PeerPublicKey: client.PublicKey,
			Conn:          serverConn,
			ReadTimeout:   time.Second,
			WriteTimeout:  time.Second,
		}
		errCh <- runtime.Serve(ctx)
	}()
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error %v, want context.Canceled", err)
	}
	_ = clientConn.Close()

	clientConn, serverConn = stdnet.Pipe()
	defer clientConn.Close()
	errCh = make(chan error, 1)
	go func() {
		runtime := &PeerRuntime{
			Node:          server,
			PeerPublicKey: client.PublicKey,
			Conn:          serverConn,
			ReadTimeout:   10 * time.Millisecond,
			WriteTimeout:  time.Second,
		}
		errCh <- runtime.Serve(context.Background())
	}()
	if err := <-errCh; err == nil {
		t.Fatal("read timeout returned nil")
	}
}

func TestPeerRuntimeAppliesDefaultLimits(t *testing.T) {
	server := testNode(t, 100)
	client := testNode(t, 200)
	clientConn, serverConn := stdnet.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	runtime := &PeerRuntime{
		Node:          server,
		PeerPublicKey: client.PublicKey,
		Conn:          serverConn,
	}
	if err := runtime.init(); err != nil {
		t.Fatalf("init runtime: %v", err)
	}
	if runtime.ReadTimeout != DefaultReadTimeout {
		t.Fatalf("read timeout %v, want %v", runtime.ReadTimeout, DefaultReadTimeout)
	}
	if runtime.WriteTimeout != DefaultWriteTimeout {
		t.Fatalf("write timeout %v, want %v", runtime.WriteTimeout, DefaultWriteTimeout)
	}
	if runtime.MaxMessages != DefaultMaxMessages {
		t.Fatalf("max messages %d, want %d", runtime.MaxMessages, DefaultMaxMessages)
	}
}

func TestServerIdlePeerReturnsOnReadTimeout(t *testing.T) {
	probe, err := stdnet.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("localhost sockets are not permitted in this sandbox: %v", err)
		}
		t.Fatalf("listen probe: %v", err)
	}
	addr := probe.Addr().String()
	_ = probe.Close()

	server := &Server{
		Addr:        addr,
		Node:        testNode(t, 100),
		ReadTimeout: 20 * time.Millisecond,
	}
	client := testNode(t, 200)
	server.PeerPublicKey = client.PublicKey
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe(ctx)
	}()

	var conn stdnet.Conn
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		conn, err = stdnet.Dial("tcp", addr)
		if err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err != nil {
		cancel()
		t.Fatalf("dial idle peer: %v", err)
	}
	defer conn.Close()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("idle server returned nil")
		}
	case <-time.After(time.Second):
		cancel()
		t.Fatal("idle peer blocked server past timeout")
	}
}

func TestDialAndServePingOverLocalhost(t *testing.T) {
	server := testNode(t, 100)
	client := testNode(t, 200)
	listener, err := stdnet.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("localhost sockets are not permitted in this sandbox: %v", err)
		}
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		runtime := &PeerRuntime{
			Node:          server,
			PeerPublicKey: client.PublicKey,
			Conn:          conn,
			ReadTimeout:   time.Second,
			WriteTimeout:  time.Second,
			MaxMessages:   1,
		}
		serverErr <- runtime.Serve(context.Background())
	}()

	ping := signedMessage(t, client, message.TypePing, message.PingPayload{TimeUnix: 200}, testNonce(4))
	if err := DialAndServe(context.Background(), listener.Addr().String(), client, server.PublicKey, []message.Message{ping}); err != nil {
		t.Fatalf("dial and serve: %v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("server runtime: %v", err)
	}
}

func TestNetPackageDoesNotExecuteTradesDirectly(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, forbidden := range []string{"ExecuteSignedTrade", "ExecuteTrade", "SubInventory", "AddInventory"} {
			if bytes.Contains(data, []byte(forbidden)) {
				t.Fatalf("%s contains direct mutation/execution call %q", file, forbidden)
			}
		}
	}
}

func serveOne(t *testing.T, server *node.Node, peerPub crypto.PublicKey, conn stdnet.Conn) <-chan error {
	t.Helper()
	errCh := make(chan error, 1)
	go func() {
		runtime := &PeerRuntime{
			Node:          server,
			PeerPublicKey: peerPub,
			Conn:          conn,
			ReadTimeout:   time.Second,
			WriteTimeout:  time.Second,
			MaxMessages:   1,
		}
		errCh <- runtime.Serve(context.Background())
	}()
	return errCh
}

func signedMessage(t *testing.T, n *node.Node, msgType message.MessageType, payload any, nonce [24]byte) message.Message {
	t.Helper()
	env, payloadBytes, err := message.SignEnvelope(n.PrivateKey, msgType, payload, n.NowUnix(), nonce)
	if err != nil {
		t.Fatalf("sign message: %v", err)
	}
	return message.Message{Envelope: env, PayloadBytes: payloadBytes}
}

func decodePayload[T any](t *testing.T, msgType message.MessageType, payloadBytes []byte) T {
	t.Helper()
	payload, err := message.DecodePayload(msgType, payloadBytes)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	typed, ok := payload.(T)
	if !ok {
		t.Fatalf("payload type %T", payload)
	}
	return typed
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
	n.PriceConfig = node.DefaultPriceConfig()
	n.ComputePrice(unit)
}

func testTxID(seed byte) model.TxID {
	var id model.TxID
	for i := range id {
		id[i] = seed + byte(i)
	}
	return id
}

func testNonce(seed byte) [24]byte {
	var nonce [24]byte
	for i := range nonce {
		nonce[i] = seed + byte(i)
	}
	return nonce
}

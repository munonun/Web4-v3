package net

import (
	"context"
	"fmt"
	stdnet "net"
	"time"

	"web4-v3/core/crypto"
	"web4-v3/core/handler"
	"web4-v3/core/message"
	"web4-v3/core/model"
	"web4-v3/core/node"
)

func DialAndServe(
	ctx context.Context,
	addr string,
	n *node.Node,
	peerPub crypto.PublicKey,
	initial []message.Message,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if addr == "" {
		return fmt.Errorf("%w: address is required", ErrInvalidClient)
	}
	if n == nil {
		return fmt.Errorf("%w: node is nil", ErrInvalidClient)
	}
	peerID, err := nodeIDFromPublicKey(peerPub)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPeerKeyRequired, err)
	}

	dialer := stdnet.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	session := handler.NewSession(n.ID, peerID)
	h, err := handler.NewHandler(n, session)
	if err != nil {
		return err
	}
	runtime := &PeerRuntime{
		Node:          n,
		Handler:       h,
		Session:       session,
		PeerPublicKey: append(crypto.PublicKey(nil), peerPub...),
		Conn:          conn,
		ReadTimeout:   5 * time.Second,
		WriteTimeout:  5 * time.Second,
	}

	expectedResponses := 0
	for _, msg := range initial {
		if err := runtime.writeFrame(msg); err != nil {
			return err
		}
		if expectsResponse(msg.Envelope.Type) {
			expectedResponses++
		}
	}

	for i := 0; i < expectedResponses; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		msg, err := runtime.readFrame()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			return err
		}
		response, err := h.HandleMessage(msg, peerPub)
		if response != nil {
			if writeErr := runtime.writeFrame(*response); writeErr != nil {
				return writeErr
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func expectsResponse(t message.MessageType) bool {
	switch t {
	case message.TypeHello, message.TypePong, message.TypeReject, message.TypeTradeResult:
		return false
	default:
		return true
	}
}

func nodeIDFromPublicKey(pub crypto.PublicKey) (model.NodeID, error) {
	return model.NodeIDFromPublicKey(pub)
}

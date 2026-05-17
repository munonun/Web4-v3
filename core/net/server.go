package net

import (
	"context"
	"fmt"
	stdnet "net"
	"time"

	"web4-v3/core/crypto"
	"web4-v3/core/model"
	"web4-v3/core/node"
)

type Server struct {
	Addr string
	Node *node.Node

	PeerPublicKey crypto.PublicKey

	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	MaxMessages  int
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil {
		return ErrInvalidServer
	}
	if s.Node == nil {
		return fmt.Errorf("%w: node is nil", ErrInvalidServer)
	}
	if _, err := model.NodeIDFromPublicKey(s.PeerPublicKey); err != nil {
		return fmt.Errorf("%w: %v", ErrPeerKeyRequired, err)
	}
	addr := s.Addr
	if addr == "" {
		addr = "127.0.0.1:0"
	}

	listener, err := stdnet.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	s.Addr = listener.Addr().String()

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = listener.Close()
		case <-done:
		}
	}()
	defer close(done)

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			return err
		}
		runtime := &PeerRuntime{
			Node:          s.Node,
			PeerPublicKey: append(crypto.PublicKey(nil), s.PeerPublicKey...),
			Conn:          conn,
			ReadTimeout:   s.ReadTimeout,
			WriteTimeout:  s.WriteTimeout,
			MaxMessages:   s.MaxMessages,
		}
		err = runtime.Serve(ctx)
		_ = conn.Close()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			continue
		}
	}
}

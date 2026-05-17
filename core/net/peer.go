package net

import (
	"context"
	"errors"
	"fmt"
	"io"
	stdnet "net"
	"strings"
	"time"

	"web4-v3/core/crypto"
	"web4-v3/core/handler"
	"web4-v3/core/message"
	"web4-v3/core/model"
	"web4-v3/core/node"
	"web4-v3/core/transport"
)

type PeerRuntime struct {
	Node    *node.Node
	Handler *handler.Handler
	Session *handler.Session

	PeerPublicKey crypto.PublicKey

	Conn stdnet.Conn

	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	MaxMessages  int
}

func NewPeerRuntime(n *node.Node, peerPub crypto.PublicKey, conn stdnet.Conn) (*PeerRuntime, error) {
	p := &PeerRuntime{Node: n, PeerPublicKey: append(crypto.PublicKey(nil), peerPub...), Conn: conn}
	if err := p.init(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *PeerRuntime) Serve(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := p.init(); err != nil {
		return err
	}
	defer p.Conn.Close()

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = p.Conn.Close()
		case <-done:
		}
	}()
	defer close(done)

	handled := 0
	for {
		if p.MaxMessages > 0 && handled >= p.MaxMessages {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		msg, err := p.readFrame()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if isCleanEOF(err) {
				return nil
			}
			return err
		}

		response, handleErr := p.Handler.HandleMessage(msg, p.PeerPublicKey)
		if response == nil && handleErr != nil && canReject(handleErr) && msg.Envelope.MessageID != (model.TxID{}) {
			response = p.reject(msg.Envelope.MessageID, handleErr)
		}
		if response != nil {
			if err := p.writeFrame(*response); err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
				return err
			}
		}
		handled++
		if handleErr != nil {
			return handleErr
		}
	}
}

func (p *PeerRuntime) init() error {
	if p == nil {
		return ErrInvalidRuntime
	}
	if p.Node == nil {
		return fmt.Errorf("%w: node is nil", ErrInvalidRuntime)
	}
	if p.Conn == nil {
		return fmt.Errorf("%w: %w", ErrInvalidRuntime, ErrConnectionMissing)
	}
	peerID, err := model.NodeIDFromPublicKey(p.PeerPublicKey)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPeerKeyRequired, err)
	}
	if p.Session == nil {
		p.Session = handler.NewSession(p.Node.ID, peerID)
	}
	if p.Session.LocalID != p.Node.ID {
		return fmt.Errorf("%w: session local id mismatch", ErrInvalidRuntime)
	}
	if p.Session.PeerID != peerID {
		return fmt.Errorf("%w: session peer id mismatch", ErrInvalidRuntime)
	}
	if p.Handler == nil {
		h, err := handler.NewHandler(p.Node, p.Session)
		if err != nil {
			return err
		}
		p.Handler = h
	}
	if p.ReadTimeout == 0 {
		p.ReadTimeout = DefaultReadTimeout
	}
	if p.WriteTimeout == 0 {
		p.WriteTimeout = DefaultWriteTimeout
	}
	if p.MaxMessages == 0 {
		p.MaxMessages = DefaultMaxMessages
	}
	return nil
}

func (p *PeerRuntime) readFrame() (message.Message, error) {
	if p.ReadTimeout > 0 {
		if err := p.Conn.SetReadDeadline(deadlineAfter(p.ReadTimeout)); err != nil {
			return message.Message{}, err
		}
	}
	return transport.ReadFrame(p.Conn)
}

func (p *PeerRuntime) writeFrame(msg message.Message) error {
	if p.WriteTimeout > 0 {
		if err := p.Conn.SetWriteDeadline(deadlineAfter(p.WriteTimeout)); err != nil {
			return err
		}
	}
	return transport.WriteFrame(p.Conn, msg)
}

func (p *PeerRuntime) reject(ref model.TxID, cause error) *message.Message {
	response, err := p.Handler.SignResponse(message.TypeReject, message.RejectPayload{
		RefMessageID: ref,
		Code:         "HANDLER_ERROR",
		Reason:       cause.Error(),
	})
	if err != nil {
		return nil
	}
	return response
}

func canReject(err error) bool {
	return !errors.Is(err, handler.ErrInvalidPeer)
}

func isCleanEOF(err error) bool {
	if errors.Is(err, io.EOF) {
		return true
	}
	text := err.Error()
	return strings.HasSuffix(text, ": EOF") || text == "EOF"
}

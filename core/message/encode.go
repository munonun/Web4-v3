package message

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"web4-v3/core/crypto"
)

const MaxMessageBytes = 1 << 20

type payloadFrame struct {
	Type    MessageType     `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type messageFrame struct {
	Envelope     Envelope `json:"envelope"`
	PayloadBytes []byte   `json:"payload_bytes"`
}

type Message struct {
	Envelope     Envelope
	PayloadBytes []byte
}

func EncodePayload(t MessageType, payload any) ([]byte, error) {
	if !IsValidMessageType(t) {
		return nil, fmt.Errorf("invalid message type %q", t)
	}

	normalized, err := normalizePayload(t, payload)
	if err != nil {
		return nil, err
	}
	payloadBytes, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	return json.Marshal(payloadFrame{Type: t, Payload: payloadBytes})
}

func DecodePayload(t MessageType, data []byte) (any, error) {
	if !IsValidMessageType(t) {
		return nil, fmt.Errorf("invalid message type %q", t)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("payload bytes are required")
	}

	var frame payloadFrame
	if err := decodeStrict(data, &frame); err != nil {
		return nil, fmt.Errorf("decode payload frame: %w", err)
	}
	if frame.Type != t {
		return nil, fmt.Errorf("payload type %q does not match %q", frame.Type, t)
	}
	if len(frame.Payload) == 0 {
		return nil, fmt.Errorf("payload body is required")
	}

	switch t {
	case TypeHello:
		var p HelloPayload
		if err := decodeStrict(frame.Payload, &p); err != nil {
			return nil, err
		}
		p.Features = sortedStrings(p.Features)
		return p, nil
	case TypeQuoteRequest:
		var p QuoteRequestPayload
		if err := decodeStrict(frame.Payload, &p); err != nil {
			return nil, err
		}
		return p, nil
	case TypeQuoteResponse:
		var p QuoteResponsePayload
		if err := decodeStrict(frame.Payload, &p); err != nil {
			return nil, err
		}
		return p, nil
	case TypeSignedIntent:
		var p SignedIntentPayload
		if err := decodeStrict(frame.Payload, &p); err != nil {
			return nil, err
		}
		return p, nil
	case TypeAuthorizedTrade:
		var p AuthorizedTradePayload
		if err := decodeStrict(frame.Payload, &p); err != nil {
			return nil, err
		}
		return p, nil
	case TypeTradeResult:
		var p TradeResultPayload
		if err := decodeStrict(frame.Payload, &p); err != nil {
			return nil, err
		}
		return p, nil
	case TypeReject:
		var p RejectPayload
		if err := decodeStrict(frame.Payload, &p); err != nil {
			return nil, err
		}
		return p, nil
	case TypePing:
		var p PingPayload
		if err := decodeStrict(frame.Payload, &p); err != nil {
			return nil, err
		}
		return p, nil
	case TypePong:
		var p PongPayload
		if err := decodeStrict(frame.Payload, &p); err != nil {
			return nil, err
		}
		return p, nil
	default:
		return nil, fmt.Errorf("invalid message type %q", t)
	}
}

func PayloadHash(t MessageType, payload any) (crypto.Hash, error) {
	encoded, err := EncodePayload(t, payload)
	if err != nil {
		return crypto.Hash{}, err
	}
	return crypto.HashBytes(encoded), nil
}

func EncodeMessage(msg Message) ([]byte, error) {
	if len(msg.PayloadBytes) > MaxMessageBytes {
		return nil, fmt.Errorf("message exceeds max size")
	}
	encoded, err := json.Marshal(messageFrame{Envelope: msg.Envelope, PayloadBytes: append([]byte(nil), msg.PayloadBytes...)})
	if err != nil {
		return nil, err
	}
	if len(encoded) > MaxMessageBytes {
		return nil, fmt.Errorf("message exceeds max size")
	}
	return encoded, nil
}

func DecodeMessage(data []byte) (Message, error) {
	if len(data) == 0 {
		return Message{}, fmt.Errorf("message bytes are required")
	}
	if len(data) > MaxMessageBytes {
		return Message{}, fmt.Errorf("message exceeds max size")
	}
	var frame messageFrame
	if err := decodeStrict(data, &frame); err != nil {
		return Message{}, fmt.Errorf("decode message: %w", err)
	}
	if len(frame.PayloadBytes) == 0 {
		return Message{}, fmt.Errorf("payload bytes are required")
	}
	return Message{Envelope: frame.Envelope, PayloadBytes: append([]byte(nil), frame.PayloadBytes...)}, nil
}

func normalizePayload(t MessageType, payload any) (any, error) {
	switch t {
	case TypeHello:
		p, ok := payload.(HelloPayload)
		if !ok {
			return nil, fmt.Errorf("HELLO requires HelloPayload")
		}
		p.PublicKey = append(crypto.PublicKey(nil), p.PublicKey...)
		p.Features = sortedStrings(p.Features)
		p.SupportedVersions = append([]uint16(nil), p.SupportedVersions...)
		return p, nil
	case TypeQuoteRequest:
		p, ok := payload.(QuoteRequestPayload)
		if !ok {
			return nil, fmt.Errorf("QUOTE_REQUEST requires QuoteRequestPayload")
		}
		return p, nil
	case TypeQuoteResponse:
		p, ok := payload.(QuoteResponsePayload)
		if !ok {
			return nil, fmt.Errorf("QUOTE_RESPONSE requires QuoteResponsePayload")
		}
		return p, nil
	case TypeSignedIntent:
		p, ok := payload.(SignedIntentPayload)
		if !ok {
			return nil, fmt.Errorf("SIGNED_INTENT requires SignedIntentPayload")
		}
		return p, nil
	case TypeAuthorizedTrade:
		p, ok := payload.(AuthorizedTradePayload)
		if !ok {
			return nil, fmt.Errorf("AUTHORIZED_TRADE requires AuthorizedTradePayload")
		}
		return p, nil
	case TypeTradeResult:
		p, ok := payload.(TradeResultPayload)
		if !ok {
			return nil, fmt.Errorf("TRADE_RESULT requires TradeResultPayload")
		}
		return p, nil
	case TypeReject:
		p, ok := payload.(RejectPayload)
		if !ok {
			return nil, fmt.Errorf("REJECT requires RejectPayload")
		}
		return p, nil
	case TypePing:
		p, ok := payload.(PingPayload)
		if !ok {
			return nil, fmt.Errorf("PING requires PingPayload")
		}
		return p, nil
	case TypePong:
		p, ok := payload.(PongPayload)
		if !ok {
			return nil, fmt.Errorf("PONG requires PongPayload")
		}
		return p, nil
	default:
		return nil, fmt.Errorf("invalid message type %q", t)
	}
}

func decodeStrict(data []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("unexpected trailing data")
	}
	return nil
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

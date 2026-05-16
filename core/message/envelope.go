package message

import (
	"fmt"

	"web4-v3/core/canonical"
	"web4-v3/core/crypto"
	"web4-v3/core/model"
)

const CurrentVersion uint16 = 1

type Envelope struct {
	Version     uint16
	Type        MessageType
	MessageID   model.TxID
	Sender      model.NodeID
	Timestamp   int64
	Nonce       [24]byte
	PayloadHash crypto.Hash
	Signature   crypto.Signature
}

func EnvelopePreimage(env Envelope) ([]byte, error) {
	return canonical.EncodeFields(
		canonical.Field{Name: "kind", Value: "message_envelope"},
		canonical.Field{Name: "version", Value: uint64(env.Version)},
		canonical.Field{Name: "type", Value: string(env.Type)},
		canonical.Field{Name: "message_id", Value: hashBytes(env.MessageID)},
		canonical.Field{Name: "sender", Value: env.Sender.Bytes()},
		canonical.Field{Name: "timestamp", Value: env.Timestamp},
		canonical.Field{Name: "nonce", Value: env.Nonce[:]},
		canonical.Field{Name: "payload_hash", Value: env.PayloadHash[:]},
	)
}

func MessageID(
	version uint16,
	msgType MessageType,
	sender model.NodeID,
	timestamp int64,
	nonce [24]byte,
	payloadHash crypto.Hash,
) (model.TxID, error) {
	if version == 0 {
		return model.TxID{}, fmt.Errorf("version must be greater than zero")
	}
	if !IsValidMessageType(msgType) {
		return model.TxID{}, fmt.Errorf("invalid message type %q", msgType)
	}
	if timestamp <= 0 {
		return model.TxID{}, fmt.Errorf("timestamp must be greater than zero")
	}
	if isZeroNonce(nonce) {
		return model.TxID{}, fmt.Errorf("nonce must not be zero")
	}

	preimage, err := canonical.EncodeFields(
		canonical.Field{Name: "kind", Value: "message_id"},
		canonical.Field{Name: "version", Value: uint64(version)},
		canonical.Field{Name: "type", Value: string(msgType)},
		canonical.Field{Name: "sender", Value: sender.Bytes()},
		canonical.Field{Name: "timestamp", Value: timestamp},
		canonical.Field{Name: "nonce", Value: nonce[:]},
		canonical.Field{Name: "payload_hash", Value: payloadHash[:]},
	)
	if err != nil {
		return model.TxID{}, err
	}
	return model.TxID(crypto.HashBytes(preimage)), nil
}

func isZeroNonce(nonce [24]byte) bool {
	return nonce == [24]byte{}
}

func hashBytes[T ~[32]byte](h T) []byte {
	b := make([]byte, 32)
	copy(b, h[:])
	return b
}

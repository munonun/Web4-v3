package message

import (
	"bytes"
	"crypto/ed25519"
	"fmt"

	"web4-v3/core/crypto"
	"web4-v3/core/model"
)

func SignEnvelope(
	priv crypto.PrivateKey,
	msgType MessageType,
	payload any,
	timestamp int64,
	nonce [24]byte,
) (Envelope, []byte, error) {
	if !IsValidMessageType(msgType) {
		return Envelope{}, nil, fmt.Errorf("invalid message type %q", msgType)
	}
	if timestamp <= 0 {
		return Envelope{}, nil, fmt.Errorf("timestamp must be greater than zero")
	}
	if isZeroNonce(nonce) {
		return Envelope{}, nil, fmt.Errorf("nonce must not be zero")
	}
	pub, err := publicKeyFromPrivate(priv)
	if err != nil {
		return Envelope{}, nil, err
	}
	sender, err := model.NodeIDFromPublicKey(pub)
	if err != nil {
		return Envelope{}, nil, err
	}

	payloadBytes, err := EncodePayload(msgType, payload)
	if err != nil {
		return Envelope{}, nil, err
	}
	payloadHash := crypto.HashBytes(payloadBytes)
	msgID, err := MessageID(CurrentVersion, msgType, sender, timestamp, nonce, payloadHash)
	if err != nil {
		return Envelope{}, nil, err
	}
	env := Envelope{
		Version:     CurrentVersion,
		Type:        msgType,
		MessageID:   msgID,
		Sender:      sender,
		Timestamp:   timestamp,
		Nonce:       nonce,
		PayloadHash: payloadHash,
	}
	preimage, err := EnvelopePreimage(env)
	if err != nil {
		return Envelope{}, nil, err
	}
	sig, err := crypto.Sign(priv, preimage)
	if err != nil {
		return Envelope{}, nil, err
	}
	env.Signature = sig
	return env, payloadBytes, nil
}

func VerifyEnvelope(env Envelope, payloadBytes []byte, pub crypto.PublicKey) error {
	if env.Version != CurrentVersion {
		return fmt.Errorf("unsupported message version %d", env.Version)
	}
	if !IsValidMessageType(env.Type) {
		return fmt.Errorf("invalid message type %q", env.Type)
	}
	if env.Timestamp <= 0 {
		return fmt.Errorf("timestamp must be greater than zero")
	}
	if isZeroNonce(env.Nonce) {
		return fmt.Errorf("nonce must not be zero")
	}
	sender, err := model.NodeIDFromPublicKey(pub)
	if err != nil {
		return err
	}
	if sender != env.Sender {
		return fmt.Errorf("sender does not match public key")
	}

	payload, err := DecodePayload(env.Type, payloadBytes)
	if err != nil {
		return fmt.Errorf("payload decode failed: %w", err)
	}
	canonicalPayloadBytes, err := EncodePayload(env.Type, payload)
	if err != nil {
		return fmt.Errorf("payload re-encode failed: %w", err)
	}
	if !bytes.Equal(payloadBytes, canonicalPayloadBytes) {
		return fmt.Errorf("payload bytes are not canonical")
	}
	payloadHash := crypto.HashBytes(payloadBytes)
	if env.PayloadHash != payloadHash {
		return fmt.Errorf("payload hash mismatch")
	}

	expectedID, err := MessageID(env.Version, env.Type, env.Sender, env.Timestamp, env.Nonce, env.PayloadHash)
	if err != nil {
		return err
	}
	if expectedID != env.MessageID {
		return fmt.Errorf("message id mismatch")
	}

	preimage, err := EnvelopePreimage(env)
	if err != nil {
		return err
	}
	if !crypto.Verify(pub, preimage, env.Signature) {
		return fmt.Errorf("invalid envelope signature")
	}
	return nil
}

func publicKeyFromPrivate(priv crypto.PrivateKey) (crypto.PublicKey, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key length: got %d, want %d", len(priv), ed25519.PrivateKeySize)
	}
	pub, ok := ed25519.PrivateKey(priv).Public().(ed25519.PublicKey)
	if !ok || len(pub) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid private key public component")
	}
	return crypto.PublicKey(pub), nil
}

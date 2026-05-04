package model

import (
	"bytes"
	"crypto/ed25519"
	"fmt"

	"web4-v3/core/canonical"
	"web4-v3/core/crypto"
)

// NewUnitID derives a unit identifier from the issuer public key and unit name.
func NewUnitID(issuer crypto.PublicKey, unitName string) (UnitID, error) {
	preimage, err := canonical.EncodeFields(
		canonical.Field{Name: "kind", Value: "unit"},
		canonical.Field{Name: "issuer", Value: []byte(issuer)},
		canonical.Field{Name: "unit_name", Value: unitName},
	)
	if err != nil {
		return UnitID{}, err
	}

	return UnitID(crypto.HashBytes(preimage)), nil
}

// IssueTxID returns the deterministic ID for tx, excluding tx.ID and tx.Signature.
func IssueTxID(tx IssueTx) (TxID, error) {
	preimage, err := issuePreimage(tx)
	if err != nil {
		return TxID{}, err
	}

	return TxID(crypto.HashBytes(preimage)), nil
}

// ValueIDFor returns the deterministic ID for value, excluding value.ID.
func ValueIDFor(value Value) (ValueID, error) {
	preimage, err := valuePreimage(value)
	if err != nil {
		return ValueID{}, err
	}

	return ValueID(crypto.HashBytes(preimage)), nil
}

// TransferTxID returns the deterministic ID for tx, excluding tx.ID and tx.Signature.
func TransferTxID(tx TransferTx) (TxID, error) {
	preimage, err := transferPreimage(tx)
	if err != nil {
		return TxID{}, err
	}

	return TxID(crypto.HashBytes(preimage)), nil
}

// TradeTxID returns the deterministic ID for an atomic bilateral exchange.
func TradeTxID(tx TradeTx) (TxID, error) {
	preimage, err := tradePreimage(tx)
	if err != nil {
		return TxID{}, err
	}

	return TxID(crypto.HashBytes(preimage)), nil
}

func issuePreimage(tx IssueTx) ([]byte, error) {
	outputs := make([][]byte, len(tx.Outputs))
	for i, output := range tx.Outputs {
		preimage, err := valuePreimage(output)
		if err != nil {
			return nil, fmt.Errorf("output %d: %w", i, err)
		}
		outputs[i] = preimage
	}

	return canonical.EncodeFields(
		canonical.Field{Name: "kind", Value: "issue_tx"},
		canonical.Field{Name: "unit_name", Value: tx.UnitName},
		canonical.Field{Name: "unit", Value: hashBytes(tx.Unit)},
		canonical.Field{Name: "amount", Value: tx.Amount},
		canonical.Field{Name: "outputs", Value: outputs},
		canonical.Field{Name: "issuer", Value: tx.Issuer.Bytes()},
		canonical.Field{Name: "owner", Value: tx.Owner.Bytes()},
		canonical.Field{Name: "expiry_unix", Value: tx.ExpiryUnix},
		canonical.Field{Name: "timestamp", Value: tx.Timestamp},
	)
}

func valuePreimage(value Value) ([]byte, error) {
	return canonical.EncodeFields(
		canonical.Field{Name: "kind", Value: "value"},
		canonical.Field{Name: "amount", Value: value.Amount},
		canonical.Field{Name: "unit", Value: hashBytes(value.Unit)},
		canonical.Field{Name: "owner", Value: value.Owner.Bytes()},
		canonical.Field{Name: "created_at", Value: value.CreatedAt},
		canonical.Field{Name: "issuer", Value: value.Issuer.Bytes()},
		canonical.Field{Name: "expiry_unix", Value: value.ExpiryUnix},
		canonical.Field{Name: "depth", Value: uint64(value.Depth)},
	)
}

func transferPreimage(tx TransferTx) ([]byte, error) {
	inputs := make([][]byte, len(tx.Inputs))
	for i, input := range tx.Inputs {
		inputs[i] = hashBytes(input)
	}

	outputs := make([][]byte, len(tx.Outputs))
	for i, output := range tx.Outputs {
		preimage, err := valuePreimage(output)
		if err != nil {
			return nil, fmt.Errorf("output %d: %w", i, err)
		}
		outputs[i] = preimage
	}

	return canonical.EncodeFields(
		canonical.Field{Name: "kind", Value: "transfer_tx"},
		canonical.Field{Name: "inputs", Value: inputs},
		canonical.Field{Name: "outputs", Value: outputs},
		canonical.Field{Name: "author", Value: tx.Author.Bytes()},
	)
}

func tradePreimage(tx TradeTx) ([]byte, error) {
	inputsA, err := valuePreimages(tx.InputsA)
	if err != nil {
		return nil, fmt.Errorf("inputs_a: %w", err)
	}
	inputsB, err := valuePreimages(tx.InputsB)
	if err != nil {
		return nil, fmt.Errorf("inputs_b: %w", err)
	}
	outputsA, err := valuePreimages(tx.OutputsA)
	if err != nil {
		return nil, fmt.Errorf("outputs_a: %w", err)
	}
	outputsB, err := valuePreimages(tx.OutputsB)
	if err != nil {
		return nil, fmt.Errorf("outputs_b: %w", err)
	}

	return canonical.EncodeFields(
		canonical.Field{Name: "kind", Value: "trade_tx"},
		canonical.Field{Name: "inputs_a", Value: inputsA},
		canonical.Field{Name: "inputs_b", Value: inputsB},
		canonical.Field{Name: "outputs_a", Value: outputsA},
		canonical.Field{Name: "outputs_b", Value: outputsB},
		canonical.Field{Name: "party_a", Value: tx.PartyA.Bytes()},
		canonical.Field{Name: "party_b", Value: tx.PartyB.Bytes()},
		canonical.Field{Name: "timestamp", Value: tx.Timestamp},
	)
}

func valuePreimages(values []Value) ([][]byte, error) {
	out := make([][]byte, len(values))
	for i, value := range values {
		preimage, err := valuePreimage(value)
		if err != nil {
			return nil, fmt.Errorf("value %d: %w", i, err)
		}
		out[i] = preimage
	}
	return out, nil
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

func hashBytes[T ~[32]byte](h T) []byte {
	b := make([]byte, 32)
	copy(b, h[:])
	return b
}

func sameHash[T ~[32]byte](a, b T) bool {
	return bytes.Equal(hashBytes(a), hashBytes(b))
}

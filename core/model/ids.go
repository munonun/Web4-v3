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

func issuePreimage(tx IssueTx) ([]byte, error) {
	return canonical.EncodeFields(
		canonical.Field{Name: "kind", Value: "issue_tx"},
		canonical.Field{Name: "unit_name", Value: tx.UnitName},
		canonical.Field{Name: "unit", Value: hashBytes(tx.Unit)},
		canonical.Field{Name: "amount", Value: tx.Amount},
		canonical.Field{Name: "issuer", Value: []byte(tx.Issuer)},
		canonical.Field{Name: "owner", Value: []byte(tx.Owner)},
		canonical.Field{Name: "expiry_unix", Value: tx.ExpiryUnix},
	)
}

func valuePreimage(value Value) ([]byte, error) {
	return canonical.EncodeFields(
		canonical.Field{Name: "kind", Value: "value"},
		canonical.Field{Name: "amount", Value: value.Amount},
		canonical.Field{Name: "unit", Value: hashBytes(value.Unit)},
		canonical.Field{Name: "owner", Value: []byte(value.Owner)},
		canonical.Field{Name: "issuer", Value: []byte(value.Issuer)},
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
		canonical.Field{Name: "author", Value: []byte(tx.Author)},
	)
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

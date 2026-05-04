package model

import (
	"fmt"

	"web4-v3/core/crypto"
)

// NewIssueTx creates and signs a minimal issuance transaction and its output value.
func NewIssueTx(issuerPriv crypto.PrivateKey, owner crypto.PublicKey, unitName string, amount Amount, expiryUnix int64) (*IssueTx, Value, error) {
	if unitName == "" {
		return nil, Value{}, fmt.Errorf("unit name is required")
	}
	if !validAmount(amount) {
		return nil, Value{}, fmt.Errorf("amount must be greater than zero")
	}

	issuer, err := publicKeyFromPrivate(issuerPriv)
	if err != nil {
		return nil, Value{}, err
	}
	issuerID, err := NodeIDFromPublicKey(issuer)
	if err != nil {
		return nil, Value{}, err
	}
	ownerID, err := NodeIDFromPublicKey(owner)
	if err != nil {
		return nil, Value{}, err
	}

	unit, err := NewUnitID(issuer, unitName)
	if err != nil {
		return nil, Value{}, err
	}

	tx := IssueTx{
		UnitName:   unitName,
		Unit:       unit,
		Amount:     amount,
		Issuer:     issuerID,
		Owner:      ownerID,
		ExpiryUnix: expiryUnix,
	}

	output := Value{
		Amount:     amount,
		Unit:       unit,
		Owner:      ownerID,
		Issuer:     issuerID,
		ExpiryUnix: expiryUnix,
		Depth:      0,
	}
	valueID, err := ValueIDFor(output)
	if err != nil {
		return nil, Value{}, err
	}
	output.ID = valueID
	tx.Outputs = []Value{output}

	txID, err := IssueTxID(tx)
	if err != nil {
		return nil, Value{}, err
	}
	tx.ID = txID

	preimage, err := issuePreimage(tx)
	if err != nil {
		return nil, Value{}, err
	}
	sig, err := crypto.Sign(issuerPriv, preimage)
	if err != nil {
		return nil, Value{}, err
	}
	tx.Signature = sig

	return &tx, output, nil
}

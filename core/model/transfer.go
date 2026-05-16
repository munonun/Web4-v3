package model

import (
	"fmt"

	"web4-v3/core/crypto"
)

// NewTransferTx creates and signs a transfer from existing input values to output values.
func NewTransferTx(authorPriv crypto.PrivateKey, inputs []Value, outputs []Value) (*TransferTx, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("inputs are required")
	}
	if len(outputs) == 0 {
		return nil, fmt.Errorf("outputs are required")
	}

	author, err := publicKeyFromPrivate(authorPriv)
	if err != nil {
		return nil, err
	}
	authorID, err := NodeIDFromPublicKey(author)
	if err != nil {
		return nil, err
	}

	canonicalInputs, err := canonicalUniqueInputs(inputs)
	if err != nil {
		return nil, err
	}

	for i, input := range canonicalInputs {
		if input.Owner != authorID {
			return nil, fmt.Errorf("input %d is not owned by author", i)
		}
	}

	nextDepth, err := nextTransferDepth(canonicalInputs)
	if err != nil {
		return nil, err
	}
	normalizedOutputs := normalizeTransferOutputExpiries(canonicalInputs, outputs)
	for i := range normalizedOutputs {
		normalizedOutputs[i].Depth = nextDepth
		id, err := ValueIDFor(normalizedOutputs[i])
		if err != nil {
			return nil, fmt.Errorf("output %d: %w", i, err)
		}
		normalizedOutputs[i].ID = id
	}

	if err := checkConservation(canonicalInputs, normalizedOutputs); err != nil {
		return nil, err
	}

	txInputs := make([]ValueID, len(canonicalInputs))
	for i, input := range canonicalInputs {
		txInputs[i] = input.ID
	}

	tx := TransferTx{
		Inputs:  txInputs,
		Outputs: normalizedOutputs,
		Author:  authorID,
	}

	txID, err := TransferTxID(tx)
	if err != nil {
		return nil, err
	}
	tx.ID = txID

	preimage, err := transferPreimage(tx)
	if err != nil {
		return nil, err
	}
	sig, err := crypto.Sign(authorPriv, preimage)
	if err != nil {
		return nil, err
	}
	tx.Signature = sig

	return &tx, nil
}

package model

import (
	"bytes"
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

	maxDepth := uint32(0)
	for i, input := range inputs {
		if !bytes.Equal(input.Owner, author) {
			return nil, fmt.Errorf("input %d is not owned by author", i)
		}
		if input.Depth > maxDepth {
			maxDepth = input.Depth
		}
	}

	nextDepth := maxDepth + 1
	normalizedOutputs := make([]Value, len(outputs))
	copy(normalizedOutputs, outputs)
	for i := range normalizedOutputs {
		normalizedOutputs[i].Depth = nextDepth
		id, err := ValueIDFor(normalizedOutputs[i])
		if err != nil {
			return nil, fmt.Errorf("output %d: %w", i, err)
		}
		normalizedOutputs[i].ID = id
	}

	if err := checkConservation(inputs, normalizedOutputs); err != nil {
		return nil, err
	}

	txInputs := make([]ValueID, len(inputs))
	for i, input := range inputs {
		txInputs[i] = input.ID
	}

	tx := TransferTx{
		Inputs:  txInputs,
		Outputs: normalizedOutputs,
		Author:  author,
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

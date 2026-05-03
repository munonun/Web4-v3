package model

import (
	"bytes"
	"fmt"
	"math"

	"web4-v3/core/crypto"
)

// ValidateIssueTx checks structural integrity and issuer signature for an issue tx.
func ValidateIssueTx(tx *IssueTx, output Value) error {
	if tx == nil {
		return fmt.Errorf("issue tx is nil")
	}
	if tx.UnitName == "" {
		return fmt.Errorf("unit name is required")
	}
	if tx.Amount == 0 {
		return fmt.Errorf("amount must be greater than zero")
	}

	unit, err := NewUnitID(tx.Issuer, tx.UnitName)
	if err != nil {
		return err
	}
	if !sameHash(tx.Unit, unit) {
		return fmt.Errorf("unit id mismatch")
	}

	expectedTxID, err := IssueTxID(*tx)
	if err != nil {
		return err
	}
	if !sameHash(tx.ID, expectedTxID) {
		return fmt.Errorf("issue tx id mismatch")
	}

	preimage, err := issuePreimage(*tx)
	if err != nil {
		return err
	}
	if !crypto.Verify(tx.Issuer, preimage, tx.Signature) {
		return fmt.Errorf("invalid issue signature")
	}

	if output.Amount != tx.Amount || !sameHash(output.Unit, tx.Unit) || !bytes.Equal(output.Owner, tx.Owner) || !bytes.Equal(output.Issuer, tx.Issuer) || output.ExpiryUnix != tx.ExpiryUnix || output.Depth != 0 {
		return fmt.Errorf("output does not match issue tx")
	}

	expectedValueID, err := ValueIDFor(output)
	if err != nil {
		return err
	}
	if !sameHash(output.ID, expectedValueID) {
		return fmt.Errorf("output value id mismatch")
	}

	return nil
}

// ValidateTransferTx checks structural integrity and author signature for a transfer tx.
func ValidateTransferTx(tx *TransferTx, inputValues []Value) error {
	if tx == nil {
		return fmt.Errorf("transfer tx is nil")
	}
	if len(tx.Inputs) == 0 {
		return fmt.Errorf("inputs are required")
	}
	if len(tx.Outputs) == 0 {
		return fmt.Errorf("outputs are required")
	}
	if len(tx.Inputs) != len(inputValues) {
		return fmt.Errorf("input count mismatch")
	}

	maxDepth := uint32(0)
	for i, input := range inputValues {
		if !sameHash(tx.Inputs[i], input.ID) {
			return fmt.Errorf("input %d id mismatch", i)
		}
		if !bytes.Equal(input.Owner, tx.Author) {
			return fmt.Errorf("input %d is not owned by author", i)
		}
		if input.Depth > maxDepth {
			maxDepth = input.Depth
		}
	}

	if maxDepth == math.MaxUint32 {
		return fmt.Errorf("input depth overflow")
	}
	expectedDepth := maxDepth + 1
	for i, output := range tx.Outputs {
		if output.Amount == 0 {
			return fmt.Errorf("output %d amount must be greater than zero", i)
		}
		if output.Depth != expectedDepth {
			return fmt.Errorf("output %d depth mismatch", i)
		}
		expectedValueID, err := ValueIDFor(output)
		if err != nil {
			return fmt.Errorf("output %d: %w", i, err)
		}
		if !sameHash(output.ID, expectedValueID) {
			return fmt.Errorf("output %d value id mismatch", i)
		}
	}

	if err := checkConservation(inputValues, tx.Outputs); err != nil {
		return err
	}

	expectedTxID, err := TransferTxID(*tx)
	if err != nil {
		return err
	}
	if !sameHash(tx.ID, expectedTxID) {
		return fmt.Errorf("transfer tx id mismatch")
	}

	preimage, err := transferPreimage(*tx)
	if err != nil {
		return err
	}
	if !crypto.Verify(tx.Author, preimage, tx.Signature) {
		return fmt.Errorf("invalid transfer signature")
	}

	return nil
}

type unitBalance struct {
	amount uint64
	issuer crypto.PublicKey
}

func checkConservation(inputs []Value, outputs []Value) error {
	balances := make(map[UnitID]unitBalance)

	for i, input := range inputs {
		if input.Amount == 0 {
			return fmt.Errorf("input %d amount must be greater than zero", i)
		}
		balance := balances[input.Unit]
		if balance.issuer != nil && !bytes.Equal(balance.issuer, input.Issuer) {
			return fmt.Errorf("input %d issuer mismatch for unit", i)
		}
		if math.MaxUint64-balance.amount < input.Amount {
			return fmt.Errorf("input amount overflow")
		}
		balance.amount += input.Amount
		balance.issuer = input.Issuer
		balances[input.Unit] = balance
	}

	for i, output := range outputs {
		if output.Amount == 0 {
			return fmt.Errorf("output %d amount must be greater than zero", i)
		}
		balance, ok := balances[output.Unit]
		if !ok {
			return fmt.Errorf("output %d uses unknown unit", i)
		}
		if !bytes.Equal(balance.issuer, output.Issuer) {
			return fmt.Errorf("output %d issuer mismatch for unit", i)
		}
		if balance.amount < output.Amount {
			return fmt.Errorf("output amount exceeds inputs for unit")
		}
		balance.amount -= output.Amount
		balances[output.Unit] = balance
	}

	for unit, balance := range balances {
		if balance.amount != 0 {
			return fmt.Errorf("unit %x is not conserved", hashBytes(unit))
		}
	}

	return nil
}

package model

import (
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
	if !validAmount(tx.Amount) {
		return fmt.Errorf("amount must be greater than zero")
	}

	unit, err := NewUnitID(tx.Issuer.PublicKey(), tx.UnitName)
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
	if !crypto.Verify(tx.Issuer.PublicKey(), preimage, tx.Signature) {
		return fmt.Errorf("invalid issue signature")
	}

	if len(tx.Outputs) != 1 {
		return fmt.Errorf("signed issue tx must have one output")
	}
	txOutput := tx.Outputs[0]
	expectedTxOutputID, err := ValueIDFor(txOutput)
	if err != nil {
		return fmt.Errorf("signed output: %w", err)
	}
	if !sameHash(txOutput.ID, expectedTxOutputID) {
		return fmt.Errorf("signed output value id mismatch")
	}
	if txOutput != output {
		return fmt.Errorf("output does not match signed issue output")
	}
	if output.Amount != tx.Amount || !sameHash(output.Unit, tx.Unit) || output.Owner != tx.Owner || output.Issuer != tx.Issuer || output.ExpiryUnix != tx.ExpiryUnix || output.Depth != 0 {
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

	inputs, err := canonicalInputsForTransfer(tx.Inputs, inputValues)
	if err != nil {
		return err
	}

	for i, input := range inputs {
		if input.Owner != tx.Author {
			return fmt.Errorf("input %d is not owned by author", i)
		}
	}

	expectedDepth, err := nextTransferDepth(inputs)
	if err != nil {
		return err
	}
	for i, output := range tx.Outputs {
		if !validAmount(output.Amount) {
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

	if err := checkConservation(inputs, tx.Outputs); err != nil {
		return err
	}
	if err := checkExpiryPropagation(inputs, tx.Outputs); err != nil {
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
	if !crypto.Verify(tx.Author.PublicKey(), preimage, tx.Signature) {
		return fmt.Errorf("invalid transfer signature")
	}

	return nil
}

func canonicalInputsForTransfer(txInputs []ValueID, inputValues []Value) ([]Value, error) {
	inputsByID := make(map[ValueID]Value, len(inputValues))
	for i, input := range inputValues {
		expectedID, err := ValueIDFor(input)
		if err != nil {
			return nil, fmt.Errorf("input %d: %w", i, err)
		}
		if input.ID != expectedID {
			return nil, fmt.Errorf("input %d value id mismatch", i)
		}
		if _, ok := inputsByID[input.ID]; ok {
			return nil, fmt.Errorf("duplicate input value id")
		}
		inputsByID[input.ID] = input
	}

	seenTxInputs := make(map[ValueID]struct{}, len(txInputs))
	inputs := make([]Value, len(txInputs))
	for i, inputID := range txInputs {
		if _, ok := seenTxInputs[inputID]; ok {
			return nil, fmt.Errorf("duplicate tx input id")
		}
		seenTxInputs[inputID] = struct{}{}

		input, ok := inputsByID[inputID]
		if !ok {
			return nil, fmt.Errorf("input %d id mismatch", i)
		}
		inputs[i] = input
	}

	if len(seenTxInputs) != len(inputsByID) {
		return nil, fmt.Errorf("input set mismatch")
	}
	return inputs, nil
}

func canonicalUniqueInputs(inputs []Value) ([]Value, error) {
	txInputs := make([]ValueID, len(inputs))
	for i, input := range inputs {
		txInputs[i] = input.ID
	}
	return canonicalInputsForTransfer(txInputs, inputs)
}

func nextTransferDepth(inputs []Value) (uint32, error) {
	maxDepth := uint32(0)
	for _, input := range inputs {
		if input.Depth > maxDepth {
			maxDepth = input.Depth
		}
	}
	if maxDepth == math.MaxUint32 {
		return 0, fmt.Errorf("input depth overflow")
	}
	return maxDepth + 1, nil
}

type unitBalance struct {
	amount Amount
	issuer NodeID
	set    bool
}

type unitIssuerKey struct {
	unit   UnitID
	issuer NodeID
}

func checkConservation(inputs []Value, outputs []Value) error {
	balances := make(map[UnitID]unitBalance)

	for i, input := range inputs {
		if !validAmount(input.Amount) {
			return fmt.Errorf("input %d amount must be greater than zero", i)
		}
		balance := balances[input.Unit]
		if balance.set && balance.issuer != input.Issuer {
			return fmt.Errorf("input %d issuer mismatch for unit", i)
		}
		next, err := CheckedAdd(balance.amount, input.Amount)
		if err != nil {
			return fmt.Errorf("input %d amount overflow", i)
		}
		balance.amount = next
		balance.issuer = input.Issuer
		balance.set = true
		balances[input.Unit] = balance
	}

	for i, output := range outputs {
		if !validAmount(output.Amount) {
			return fmt.Errorf("output %d amount must be greater than zero", i)
		}
		balance, ok := balances[output.Unit]
		if !ok {
			return fmt.Errorf("output %d uses unknown unit", i)
		}
		if balance.issuer != output.Issuer {
			return fmt.Errorf("output %d issuer mismatch for unit", i)
		}
		if balance.amount < output.Amount {
			return fmt.Errorf("output amount exceeds inputs for unit")
		}
		next, err := Sub(balance.amount, output.Amount)
		if err != nil {
			return fmt.Errorf("output amount exceeds inputs for unit")
		}
		balance.amount = next
		balances[output.Unit] = balance
	}

	for unit, balance := range balances {
		if balance.amount != 0 {
			return fmt.Errorf("unit %x is not conserved", hashBytes(unit))
		}
	}

	return nil
}

func checkExpiryPropagation(inputs []Value, outputs []Value) error {
	const noExpiry = int64(0)
	minExpiry := make(map[unitIssuerKey]int64)
	seen := make(map[unitIssuerKey]struct{})

	for _, input := range inputs {
		key := unitIssuerKey{unit: input.Unit, issuer: input.Issuer}
		seen[key] = struct{}{}
		if input.ExpiryUnix == noExpiry {
			continue
		}
		if minExpiry[key] == noExpiry || input.ExpiryUnix < minExpiry[key] {
			minExpiry[key] = input.ExpiryUnix
		}
	}

	for i, output := range outputs {
		key := unitIssuerKey{unit: output.Unit, issuer: output.Issuer}
		if _, ok := seen[key]; !ok {
			return fmt.Errorf("output %d uses unknown unit issuer", i)
		}
		expiry := minExpiry[key]
		if expiry == noExpiry {
			continue
		}
		if output.ExpiryUnix == noExpiry {
			return fmt.Errorf("output %d removes expiry constraint", i)
		}
		if output.ExpiryUnix > expiry {
			return fmt.Errorf("output %d extends expiry constraint", i)
		}
	}

	return nil
}

func normalizeTransferOutputExpiries(inputs []Value, outputs []Value) []Value {
	minExpiry := make(map[unitIssuerKey]int64)
	for _, input := range inputs {
		if input.ExpiryUnix == 0 {
			continue
		}
		key := unitIssuerKey{unit: input.Unit, issuer: input.Issuer}
		if minExpiry[key] == 0 || input.ExpiryUnix < minExpiry[key] {
			minExpiry[key] = input.ExpiryUnix
		}
	}

	normalized := make([]Value, len(outputs))
	copy(normalized, outputs)
	for i := range normalized {
		expiry := minExpiry[unitIssuerKey{unit: normalized[i].Unit, issuer: normalized[i].Issuer}]
		if expiry == 0 {
			continue
		}
		if normalized[i].ExpiryUnix == 0 || normalized[i].ExpiryUnix > expiry {
			normalized[i].ExpiryUnix = expiry
		}
	}
	return normalized
}

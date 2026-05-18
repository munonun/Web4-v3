package model

import (
	"math"
	"strings"
	"testing"

	"web4-v3/core/crypto"
)

func TestUnitIDStableForSameIssuerAndName(t *testing.T) {
	issuerPub, _, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate issuer: %v", err)
	}

	a, err := NewUnitID(issuerPub, "credits")
	if err != nil {
		t.Fatalf("unit id a: %v", err)
	}
	b, err := NewUnitID(issuerPub, "credits")
	if err != nil {
		t.Fatalf("unit id b: %v", err)
	}

	if a != b {
		t.Fatal("same issuer and unit name produced different unit IDs")
	}
}

func TestUnitIDDifferentIssuerSameNameDiffers(t *testing.T) {
	issuerA, _, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate issuer a: %v", err)
	}
	issuerB, _, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate issuer b: %v", err)
	}

	a, err := NewUnitID(issuerA, "credits")
	if err != nil {
		t.Fatalf("unit id a: %v", err)
	}
	b, err := NewUnitID(issuerB, "credits")
	if err != nil {
		t.Fatalf("unit id b: %v", err)
	}

	if a == b {
		t.Fatal("different issuers produced same unit ID")
	}
}

func TestIssueTxValidates(t *testing.T) {
	tx, output := mustIssue(t, 100, 0)

	if err := ValidateIssueTx(tx, output); err != nil {
		t.Fatalf("validate issue: %v", err)
	}
}

func TestNewIssueTxRejectsZeroOwner(t *testing.T) {
	_, issuerPriv, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate issuer: %v", err)
	}
	zeroOwner := crypto.PublicKey(make([]byte, 32))
	if _, _, err := NewIssueTx(issuerPriv, zeroOwner, "credits", 100, 0); err == nil {
		t.Fatal("expected zero owner rejection")
	}
}

func TestValidateIssueTxRejectsZeroOwner(t *testing.T) {
	issuerPub, issuerPriv, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate issuer: %v", err)
	}
	issuerID, err := NodeIDFromPublicKey(issuerPub)
	if err != nil {
		t.Fatalf("issuer id: %v", err)
	}
	unit, err := NewUnitID(issuerPub, "credits")
	if err != nil {
		t.Fatalf("unit id: %v", err)
	}
	output := Value{Amount: 100, Unit: unit, Owner: NodeID{}, Issuer: issuerID, Depth: 0}
	output.ID = mustValueID(t, output)
	tx := IssueTx{
		UnitName: "credits",
		Unit:     unit,
		Amount:   100,
		Issuer:   issuerID,
		Owner:    NodeID{},
		Outputs:  []Value{output},
	}
	tx.ID, err = IssueTxID(tx)
	if err != nil {
		t.Fatalf("tx id: %v", err)
	}
	preimage, err := issuePreimage(tx)
	if err != nil {
		t.Fatalf("issue preimage: %v", err)
	}
	tx.Signature, err = crypto.Sign(issuerPriv, preimage)
	if err != nil {
		t.Fatalf("sign issue: %v", err)
	}
	if err := ValidateIssueTx(&tx, output); err == nil {
		t.Fatal("expected zero owner validation rejection")
	}
}

func TestIssueOutputCreatedAtBoundToSignedOutput(t *testing.T) {
	tx, output := mustIssueWithCreatedAt(t, 100, 123)

	if err := ValidateIssueTx(tx, output); err != nil {
		t.Fatalf("valid signed issue output: %v", err)
	}

	mutated := output
	mutated.CreatedAt = 124
	mutated.ID = mustValueID(t, mutated)
	if err := ValidateIssueTx(tx, mutated); err == nil {
		t.Fatal("expected changed CreatedAt with alternate canonical ID to fail")
	}
}

func TestIssueOutputCreatedAtMutationWithoutValueIDFails(t *testing.T) {
	tx, output := mustIssueWithCreatedAt(t, 100, 123)
	output.CreatedAt = 124

	if err := ValidateIssueTx(tx, output); err == nil {
		t.Fatal("expected changed CreatedAt to fail")
	}
}

func TestTamperedIssueAmountFails(t *testing.T) {
	tx, output := mustIssue(t, 100, 0)
	tx.Amount = 101

	if err := ValidateIssueTx(tx, output); err == nil {
		t.Fatal("expected tampered issue amount to fail")
	}
}

func TestTamperedIssueSignatureFails(t *testing.T) {
	tx, output := mustIssue(t, 100, 0)
	tx.Signature[0] ^= 0xff

	if err := ValidateIssueTx(tx, output); err == nil {
		t.Fatal("expected tampered issue signature to fail")
	}
}

func TestTransferValidates(t *testing.T) {
	_, input, ownerPriv := mustIssueToOwner(t, 100, 0)
	tx := mustTransfer(t, ownerPriv, []Value{input}, []Value{transferOutput(input, input.Owner, 100)})

	if err := ValidateTransferTx(tx, []Value{input}); err != nil {
		t.Fatalf("validate transfer: %v", err)
	}
}

func TestTransferWrongAuthorFails(t *testing.T) {
	_, input, _ := mustIssueToOwner(t, 100, 0)
	_, wrongPriv, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate wrong author: %v", err)
	}

	if _, err := NewTransferTx(wrongPriv, []Value{input}, []Value{transferOutput(input, input.Owner, 100)}); err == nil {
		t.Fatal("expected wrong author to fail")
	}
}

func TestTransferViolatingConservationFails(t *testing.T) {
	_, input, ownerPriv := mustIssueToOwner(t, 100, 0)

	if _, err := NewTransferTx(ownerPriv, []Value{input}, []Value{transferOutput(input, input.Owner, 101)}); err == nil {
		t.Fatal("expected conservation failure")
	}
}

func TestTransferExpiringInputCannotBecomeNonExpiring(t *testing.T) {
	_, input, ownerPriv := mustIssueToOwner(t, 100, 200)
	output := transferOutput(input, input.Owner, 100)
	output.ExpiryUnix = 0
	tx := signedTransferWithOutputs(t, ownerPriv, []Value{input}, []Value{output})

	err := ValidateTransferTx(tx, []Value{input})
	if err == nil || !strings.Contains(err.Error(), "removes expiry") {
		t.Fatalf("expected expiry removal rejection, got %v", err)
	}
}

func TestTransferExpiringInputCannotBecomeLaterExpiry(t *testing.T) {
	_, input, ownerPriv := mustIssueToOwner(t, 100, 200)
	output := transferOutput(input, input.Owner, 100)
	output.ExpiryUnix = 201
	tx := signedTransferWithOutputs(t, ownerPriv, []Value{input}, []Value{output})

	err := ValidateTransferTx(tx, []Value{input})
	if err == nil || !strings.Contains(err.Error(), "extends expiry") {
		t.Fatalf("expected expiry extension rejection, got %v", err)
	}
}

func TestTransferEarlierOrEqualExpiryPasses(t *testing.T) {
	_, input, ownerPriv := mustIssueToOwner(t, 100, 200)
	for _, expiry := range []int64{199, 200} {
		output := transferOutput(input, input.Owner, 100)
		output.ExpiryUnix = expiry
		tx := signedTransferWithOutputs(t, ownerPriv, []Value{input}, []Value{output})

		if err := ValidateTransferTx(tx, []Value{input}); err != nil {
			t.Fatalf("expiry %d should validate: %v", expiry, err)
		}
	}
}

func TestTransferConstructorNormalizesExtendedExpiry(t *testing.T) {
	_, input, ownerPriv := mustIssueToOwner(t, 100, 200)
	output := transferOutput(input, input.Owner, 100)
	output.ExpiryUnix = 0

	tx := mustTransfer(t, ownerPriv, []Value{input}, []Value{output})
	if got := tx.Outputs[0].ExpiryUnix; got != 200 {
		t.Fatalf("normalized expiry %d, want 200", got)
	}
	if err := ValidateTransferTx(tx, []Value{input}); err != nil {
		t.Fatalf("validate normalized transfer: %v", err)
	}
}

func TestTransferNonExpiringInputMayRemainNonExpiring(t *testing.T) {
	_, input, ownerPriv := mustIssueToOwner(t, 100, 0)
	tx := mustTransfer(t, ownerPriv, []Value{input}, []Value{transferOutput(input, input.Owner, 100)})

	if got := tx.Outputs[0].ExpiryUnix; got != 0 {
		t.Fatalf("expiry %d, want 0", got)
	}
	if err := ValidateTransferTx(tx, []Value{input}); err != nil {
		t.Fatalf("validate non-expiring transfer: %v", err)
	}
}

func TestTransferWithZeroAmountOutputFails(t *testing.T) {
	_, input, ownerPriv := mustIssueToOwner(t, 100, 0)

	if _, err := NewTransferTx(ownerPriv, []Value{input}, []Value{transferOutput(input, input.Owner, 0)}); err == nil {
		t.Fatal("expected zero amount output failure")
	}
}

func TestNewTransferTxRejectsZeroOwnerOutput(t *testing.T) {
	_, input, ownerPriv := mustIssueToOwner(t, 100, 0)

	if _, err := NewTransferTx(ownerPriv, []Value{input}, []Value{transferOutput(input, NodeID{}, 100)}); err == nil {
		t.Fatal("expected zero-owner output rejection")
	}
}

func TestValidateTransferTxRejectsZeroOwnerOutput(t *testing.T) {
	_, input, ownerPriv := mustIssueToOwner(t, 100, 0)
	output := transferOutput(input, NodeID{}, 100)
	tx := signedTransferWithOutputs(t, ownerPriv, []Value{input}, []Value{output})

	if err := ValidateTransferTx(tx, []Value{input}); err == nil {
		t.Fatal("expected zero-owner output validation rejection")
	}
}

func TestTransferDuplicateInputValueRejected(t *testing.T) {
	_, input, ownerPriv := mustIssueToOwner(t, 100, 0)
	tx := TransferTx{
		Inputs: []ValueID{input.ID, input.ID},
		Outputs: []Value{
			transferOutput(input, input.Owner, 100),
		},
		Author: input.Owner,
	}
	tx.ID = mustTransferTxID(t, tx)
	preimage, err := transferPreimage(tx)
	if err != nil {
		t.Fatalf("transfer preimage: %v", err)
	}
	tx.Signature, err = crypto.Sign(ownerPriv, preimage)
	if err != nil {
		t.Fatalf("sign transfer: %v", err)
	}

	if err := ValidateTransferTx(&tx, []Value{input, input}); err == nil {
		t.Fatal("expected duplicate input value rejection")
	}
}

func TestTransferDuplicateTxInputIDRejected(t *testing.T) {
	_, inputA, ownerPriv := mustIssueToOwner(t, 100, 0)
	inputB := inputA
	inputB.CreatedAt++
	inputB.ID = mustValueID(t, inputB)
	tx := TransferTx{
		Inputs: []ValueID{inputA.ID, inputA.ID},
		Outputs: []Value{
			transferOutput(inputA, inputA.Owner, 100),
		},
		Author: inputA.Owner,
	}
	tx.ID = mustTransferTxID(t, tx)
	preimage, err := transferPreimage(tx)
	if err != nil {
		t.Fatalf("transfer preimage: %v", err)
	}
	tx.Signature, err = crypto.Sign(ownerPriv, preimage)
	if err != nil {
		t.Fatalf("sign transfer: %v", err)
	}

	if err := ValidateTransferTx(&tx, []Value{inputA, inputB}); err == nil {
		t.Fatal("expected duplicate tx input id rejection")
	}
}

func TestTransferNonCanonicalInputValueIDRejected(t *testing.T) {
	_, input, ownerPriv := mustIssueToOwner(t, 100, 0)
	tx := mustTransfer(t, ownerPriv, []Value{input}, []Value{transferOutput(input, input.Owner, 100)})
	forged := input
	forged.Amount = 200

	if err := ValidateTransferTx(tx, []Value{forged}); err == nil {
		t.Fatal("expected non-canonical input value rejection")
	}
}

func TestTransferConservationCannotBeInflatedByDuplicateInputs(t *testing.T) {
	_, input, ownerPriv := mustIssueToOwner(t, 100, 0)

	if _, err := NewTransferTx(ownerPriv, []Value{input, input}, []Value{transferOutput(input, input.Owner, 200)}); err == nil {
		t.Fatal("expected duplicate inputs to fail before conservation can be inflated")
	}
}

func TestTransferMaxDepthInputConstructorError(t *testing.T) {
	_, input, ownerPriv := mustIssueToOwner(t, 100, 0)
	input.Depth = math.MaxUint32
	input.ID = mustValueID(t, input)

	if _, err := NewTransferTx(ownerPriv, []Value{input}, []Value{transferOutput(input, input.Owner, 100)}); err == nil {
		t.Fatal("expected max-depth input constructor error")
	}
}

func TestTransferValidatorRejectsOverflowOutputDepth(t *testing.T) {
	_, input, ownerPriv := mustIssueToOwner(t, 100, 0)
	input.Depth = math.MaxUint32
	input.ID = mustValueID(t, input)
	output := transferOutput(input, input.Owner, 100)
	output.Depth = 0
	output.ID = mustValueID(t, output)
	tx := TransferTx{
		Inputs:  []ValueID{input.ID},
		Outputs: []Value{output},
		Author:  input.Owner,
	}
	tx.ID = mustTransferTxID(t, tx)
	preimage, err := transferPreimage(tx)
	if err != nil {
		t.Fatalf("transfer preimage: %v", err)
	}
	tx.Signature, err = crypto.Sign(ownerPriv, preimage)
	if err != nil {
		t.Fatalf("sign transfer: %v", err)
	}

	if err := ValidateTransferTx(&tx, []Value{input}); err == nil {
		t.Fatal("expected validator depth overflow rejection")
	}
}

func TestTransferValidationAmountOverflowReturnsError(t *testing.T) {
	_, input, ownerPriv := mustIssueToOwner(t, 1, 0)
	input.Amount = Amount(math.MaxInt64)
	input.ID = mustValueID(t, input)
	inputB := input
	inputB.CreatedAt = 1
	inputB.ID = mustValueID(t, inputB)
	output := transferOutput(input, input.Owner, Amount(math.MaxInt64))

	tx := signedTransferWithOutputs(t, ownerPriv, []Value{input, inputB}, []Value{output})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ValidateTransferTx panicked: %v", r)
		}
	}()
	if err := ValidateTransferTx(tx, []Value{input, inputB}); err == nil {
		t.Fatal("expected overflow validation error")
	}
}

func TestTransferValidationLargeSafeAmountPasses(t *testing.T) {
	_, input, ownerPriv := mustIssueToOwner(t, 1, 0)
	input.Amount = Amount(math.MaxInt64 - 100)
	input.ID = mustValueID(t, input)
	inputB := input
	inputB.CreatedAt = 1
	inputB.Amount = 50
	inputB.ID = mustValueID(t, inputB)
	output := transferOutput(input, input.Owner, Amount(math.MaxInt64-50))

	tx := signedTransferWithOutputs(t, ownerPriv, []Value{input, inputB}, []Value{output})

	if err := ValidateTransferTx(tx, []Value{input, inputB}); err != nil {
		t.Fatalf("validate large safe transfer: %v", err)
	}
}

func TestTransferNormalDepthIncrements(t *testing.T) {
	_, input, ownerPriv := mustIssueToOwner(t, 100, 0)
	input.Depth = 7
	input.ID = mustValueID(t, input)

	tx := mustTransfer(t, ownerPriv, []Value{input}, []Value{transferOutput(input, input.Owner, 100)})
	if got := tx.Outputs[0].Depth; got != 8 {
		t.Fatalf("output depth %d, want 8", got)
	}
	if err := ValidateTransferTx(tx, []Value{input}); err != nil {
		t.Fatalf("validate transfer: %v", err)
	}
}

func TestTxIDsStableAndSignatureIndependent(t *testing.T) {
	issue, input, ownerPriv := mustIssueToOwner(t, 100, 0)
	issueID, err := IssueTxID(*issue)
	if err != nil {
		t.Fatalf("issue id: %v", err)
	}
	issue.Signature[0] ^= 0xff
	issueIDAfterSigChange, err := IssueTxID(*issue)
	if err != nil {
		t.Fatalf("issue id after signature change: %v", err)
	}
	if issueID != issueIDAfterSigChange || issue.ID != issueID {
		t.Fatal("issue tx ID changed with signature")
	}

	transfer := mustTransfer(t, ownerPriv, []Value{input}, []Value{transferOutput(input, input.Owner, 100)})
	transferID, err := TransferTxID(*transfer)
	if err != nil {
		t.Fatalf("transfer id: %v", err)
	}
	transfer.Signature[0] ^= 0xff
	transferIDAfterSigChange, err := TransferTxID(*transfer)
	if err != nil {
		t.Fatalf("transfer id after signature change: %v", err)
	}
	if transferID != transferIDAfterSigChange || transfer.ID != transferID {
		t.Fatal("transfer tx ID changed with signature")
	}
}

func mustIssue(t *testing.T, amount Amount, expiryUnix int64) (*IssueTx, Value) {
	t.Helper()
	tx, output, _ := mustIssueToOwner(t, amount, expiryUnix)
	return tx, output
}

func mustIssueToOwner(t *testing.T, amount Amount, expiryUnix int64) (*IssueTx, Value, crypto.PrivateKey) {
	t.Helper()
	_, issuerPriv, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate issuer: %v", err)
	}
	ownerPub, ownerPriv, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate owner: %v", err)
	}

	tx, output, err := NewIssueTx(issuerPriv, ownerPub, "credits", amount, expiryUnix)
	if err != nil {
		t.Fatalf("new issue tx: %v", err)
	}
	return tx, output, ownerPriv
}

func mustIssueWithCreatedAt(t *testing.T, amount Amount, createdAt int64) (*IssueTx, Value) {
	t.Helper()
	issuerPub, issuerPriv, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate issuer: %v", err)
	}
	ownerPub, _, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate owner: %v", err)
	}
	issuerID, err := NodeIDFromPublicKey(issuerPub)
	if err != nil {
		t.Fatalf("issuer id: %v", err)
	}
	ownerID, err := NodeIDFromPublicKey(ownerPub)
	if err != nil {
		t.Fatalf("owner id: %v", err)
	}
	unit, err := NewUnitID(issuerPub, "credits")
	if err != nil {
		t.Fatalf("unit id: %v", err)
	}
	output := Value{Amount: amount, Unit: unit, Owner: ownerID, CreatedAt: createdAt, Issuer: issuerID}
	output.ID = mustValueID(t, output)
	tx := IssueTx{
		UnitName: "credits",
		Unit:     unit,
		Amount:   amount,
		Issuer:   issuerID,
		Owner:    ownerID,
		Outputs:  []Value{output},
	}
	tx.ID = mustIssueTxID(t, tx)
	preimage, err := issuePreimage(tx)
	if err != nil {
		t.Fatalf("issue preimage: %v", err)
	}
	tx.Signature, err = crypto.Sign(issuerPriv, preimage)
	if err != nil {
		t.Fatalf("sign issue: %v", err)
	}
	return &tx, output
}

func mustTransfer(t *testing.T, authorPriv crypto.PrivateKey, inputs []Value, outputs []Value) *TransferTx {
	t.Helper()
	tx, err := NewTransferTx(authorPriv, inputs, outputs)
	if err != nil {
		t.Fatalf("new transfer tx: %v", err)
	}
	return tx
}

func signedTransferWithOutputs(t *testing.T, authorPriv crypto.PrivateKey, inputs []Value, outputs []Value) *TransferTx {
	t.Helper()
	canonicalInputs, err := canonicalUniqueInputs(inputs)
	if err != nil {
		t.Fatalf("canonical inputs: %v", err)
	}
	nextDepth, err := nextTransferDepth(canonicalInputs)
	if err != nil {
		t.Fatalf("next depth: %v", err)
	}
	signedOutputs := make([]Value, len(outputs))
	copy(signedOutputs, outputs)
	for i := range signedOutputs {
		signedOutputs[i].Depth = nextDepth
		signedOutputs[i].ID = mustValueID(t, signedOutputs[i])
	}
	txInputs := make([]ValueID, len(canonicalInputs))
	for i, input := range canonicalInputs {
		txInputs[i] = input.ID
	}
	author, err := publicKeyFromPrivate(authorPriv)
	if err != nil {
		t.Fatalf("author pub: %v", err)
	}
	authorID, err := NodeIDFromPublicKey(author)
	if err != nil {
		t.Fatalf("author id: %v", err)
	}
	tx := TransferTx{Inputs: txInputs, Outputs: signedOutputs, Author: authorID}
	tx.ID = mustTransferTxID(t, tx)
	preimage, err := transferPreimage(tx)
	if err != nil {
		t.Fatalf("transfer preimage: %v", err)
	}
	tx.Signature, err = crypto.Sign(authorPriv, preimage)
	if err != nil {
		t.Fatalf("sign transfer: %v", err)
	}
	return &tx
}

func mustTransferTxID(t *testing.T, tx TransferTx) TxID {
	t.Helper()
	id, err := TransferTxID(tx)
	if err != nil {
		t.Fatalf("transfer id: %v", err)
	}
	return id
}

func mustValueID(t *testing.T, value Value) ValueID {
	t.Helper()
	id, err := ValueIDFor(value)
	if err != nil {
		t.Fatalf("value id: %v", err)
	}
	return id
}

func transferOutput(input Value, owner NodeID, amount Amount) Value {
	return Value{
		Amount:     amount,
		Unit:       input.Unit,
		Owner:      owner,
		Issuer:     input.Issuer,
		ExpiryUnix: input.ExpiryUnix,
	}
}

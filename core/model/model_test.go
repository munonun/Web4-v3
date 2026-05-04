package model

import (
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

func TestTransferWithZeroAmountOutputFails(t *testing.T) {
	_, input, ownerPriv := mustIssueToOwner(t, 100, 0)

	if _, err := NewTransferTx(ownerPriv, []Value{input}, []Value{transferOutput(input, input.Owner, 0)}); err == nil {
		t.Fatal("expected zero amount output failure")
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

func mustTransfer(t *testing.T, authorPriv crypto.PrivateKey, inputs []Value, outputs []Value) *TransferTx {
	t.Helper()
	tx, err := NewTransferTx(authorPriv, inputs, outputs)
	if err != nil {
		t.Fatalf("new transfer tx: %v", err)
	}
	return tx
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

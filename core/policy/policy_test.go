package policy

import (
	"strings"
	"testing"

	"web4-v3/core/crypto"
	"web4-v3/core/model"
)

func TestTrustedIssuerIssueAccepted(t *testing.T) {
	tx, output, issuerPub, _ := mustPolicyIssue(t, 100, 200)
	p := trustedPolicy(issuerPub, 100)

	result := p.EvaluateIssue(tx, output)
	if result.Decision != Accept {
		t.Fatalf("decision %s, want %s: %v", result.Decision, Accept, result.Reasons)
	}
	if result.Score != 0.9 {
		t.Fatalf("score %f, want 0.9", result.Score)
	}
}

func TestUnknownIssuerIssueRejected(t *testing.T) {
	tx, output, _, _ := mustPolicyIssue(t, 100, 200)
	p := Policy{TrustedIssuers: map[string]float64{}, MinScore: 0.5, MaxDepth: 10, NowUnix: func() int64 { return 100 }}

	result := p.EvaluateIssue(tx, output)
	if result.Decision != Reject {
		t.Fatalf("decision %s, want %s", result.Decision, Reject)
	}
	assertReasonContains(t, result, "not trusted")
}

func TestExpiredValueRejected(t *testing.T) {
	tx, output, issuerPub, _ := mustPolicyIssue(t, 100, 50)
	p := trustedPolicy(issuerPub, 100)

	result := p.EvaluateIssue(tx, output)
	if result.Decision != Reject {
		t.Fatalf("decision %s, want %s", result.Decision, Reject)
	}
	assertReasonContains(t, result, "expired")
}

func TestTransferFromTrustedIssuerAccepted(t *testing.T) {
	_, input, issuerPub, ownerPriv := mustPolicyIssue(t, 100, 200)
	tx := mustPolicyTransfer(t, ownerPriv, input, 100)
	p := trustedPolicy(issuerPub, 100)

	result := p.EvaluateTransfer(tx, []model.Value{input})
	if result.Decision != Accept {
		t.Fatalf("decision %s, want %s: %v", result.Decision, Accept, result.Reasons)
	}
}

func TestTransferExceedingMaxDepthRejected(t *testing.T) {
	_, input, issuerPub, ownerPriv := mustPolicyIssue(t, 100, 200)
	tx := mustPolicyTransfer(t, ownerPriv, input, 100)
	p := trustedPolicy(issuerPub, 100)
	p.MaxDepth = 0

	result := p.EvaluateTransfer(tx, []model.Value{input})
	if result.Decision != Reject {
		t.Fatalf("decision %s, want %s", result.Decision, Reject)
	}
	assertReasonContains(t, result, "depth")
}

func TestValidationFailureCausesReject(t *testing.T) {
	tx, output, issuerPub, _ := mustPolicyIssue(t, 100, 200)
	p := trustedPolicy(issuerPub, 100)
	tx.Amount = 101

	result := p.EvaluateIssue(tx, output)
	if result.Decision != Reject {
		t.Fatalf("decision %s, want %s", result.Decision, Reject)
	}
	assertReasonContains(t, result, "validation failed")
}

func TestReasonsIncludedAndUseful(t *testing.T) {
	tx, output, issuerPub, _ := mustPolicyIssue(t, 100, 200)
	p := trustedPolicy(issuerPub, 100)

	result := p.EvaluateIssue(tx, output)
	if len(result.Reasons) == 0 {
		t.Fatal("expected reasons")
	}
	assertReasonContains(t, result, "trusted")
}

func trustedPolicy(issuer crypto.PublicKey, now int64) Policy {
	return Policy{
		TrustedIssuers: map[string]float64{IssuerKey(issuer): 0.9},
		MinScore:       0.5,
		MaxDepth:       10,
		NowUnix:        func() int64 { return now },
	}
}

func mustPolicyIssue(t *testing.T, amount uint64, expiryUnix int64) (*model.IssueTx, model.Value, crypto.PublicKey, crypto.PrivateKey) {
	t.Helper()
	issuerPub, issuerPriv, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate issuer: %v", err)
	}
	ownerPub, ownerPriv, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate owner: %v", err)
	}
	tx, output, err := model.NewIssueTx(issuerPriv, ownerPub, "credits", amount, expiryUnix)
	if err != nil {
		t.Fatalf("new issue tx: %v", err)
	}
	return tx, output, issuerPub, ownerPriv
}

func mustPolicyTransfer(t *testing.T, ownerPriv crypto.PrivateKey, input model.Value, amount uint64) *model.TransferTx {
	t.Helper()
	output := model.Value{
		Amount:     amount,
		Unit:       input.Unit,
		Owner:      input.Owner,
		Issuer:     input.Issuer,
		ExpiryUnix: input.ExpiryUnix,
	}
	tx, err := model.NewTransferTx(ownerPriv, []model.Value{input}, []model.Value{output})
	if err != nil {
		t.Fatalf("new transfer tx: %v", err)
	}
	return tx
}

func assertReasonContains(t *testing.T, result AcceptanceResult, want string) {
	t.Helper()
	for _, reason := range result.Reasons {
		if strings.Contains(reason, want) {
			return
		}
	}
	t.Fatalf("reasons %v did not contain %q", result.Reasons, want)
}

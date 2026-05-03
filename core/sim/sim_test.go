package sim

import (
	"math"
	"testing"

	"web4-v3/core/crypto"
	"web4-v3/core/model"
	"web4-v3/core/policy"
)

func TestUnanimousAcceptance(t *testing.T) {
	issuer, tx, inputs := mustSimTransfer(t)
	net := SimNetwork{Nodes: []*SimNode{
		trustedNode("a", issuer),
		trustedNode("b", issuer),
		trustedNode("c", issuer),
	}}

	if ratio := net.AcceptanceRatio(tx, inputs); ratio != 1.0 {
		t.Fatalf("ratio %f, want 1.0", ratio)
	}
	if !net.Survives(tx, inputs, 1.0) {
		t.Fatal("expected tx to survive tau 1.0")
	}
	assertVector(t, net.AcceptanceVector(tx, inputs), []float64{1, 1, 1})

	result := net.Evaluate(tx, inputs)
	if result.AcceptCount != 3 || result.RejectCount != 0 {
		t.Fatalf("counts accept=%d reject=%d, want 3/0", result.AcceptCount, result.RejectCount)
	}
}

func TestPartialAcceptance(t *testing.T) {
	issuer, tx, inputs := mustSimTransfer(t)
	net := SimNetwork{Nodes: []*SimNode{
		trustedNode("a", issuer),
		trustedNode("b", issuer),
		untrustedNode("c"),
	}}

	ratio := net.AcceptanceRatio(tx, inputs)
	if math.Abs(ratio-2.0/3.0) > 0.000001 {
		t.Fatalf("ratio %f, want 2/3", ratio)
	}
	if !net.Survives(tx, inputs, 0.5) {
		t.Fatal("expected tx to survive tau 0.5")
	}
	if net.Survives(tx, inputs, 0.7) {
		t.Fatal("expected tx to fail tau 0.7")
	}
	assertVector(t, net.AcceptanceVector(tx, inputs), []float64{1, 1, 0})
}

func TestZeroAcceptance(t *testing.T) {
	_, tx, inputs := mustSimTransfer(t)
	net := SimNetwork{Nodes: []*SimNode{
		untrustedNode("a"),
		untrustedNode("b"),
		untrustedNode("c"),
	}}

	if ratio := net.AcceptanceRatio(tx, inputs); ratio != 0 {
		t.Fatalf("ratio %f, want 0", ratio)
	}
	if net.Survives(tx, inputs, 0.1) {
		t.Fatal("expected tx not to survive")
	}
	assertVector(t, net.AcceptanceVector(tx, inputs), []float64{0, 0, 0})
}

func TestTradeRequiresBothNodesToAccept(t *testing.T) {
	issuer, tx, inputs := mustSimTransfer(t)
	a := trustedNode("a", issuer)
	b := untrustedNode("b")

	if Trade(a, b, tx, inputs) {
		t.Fatal("expected trade to fail when one node rejects")
	}
}

func TestTransferPathViability(t *testing.T) {
	issuer, tx, inputs := mustSimTransfer(t)
	a := trustedNode("a", issuer)
	b := trustedNode("b", issuer)
	c := untrustedNode("c")

	if !Trade(a, b, tx, inputs) {
		t.Fatal("expected A-B trade to succeed")
	}
	if Trade(b, c, tx, inputs) {
		t.Fatal("expected B-C trade to fail")
	}
}

func mustSimTransfer(t *testing.T) (crypto.PublicKey, *model.TransferTx, []model.Value) {
	t.Helper()
	issuerPub, issuerPriv, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate issuer: %v", err)
	}
	ownerPub, ownerPriv, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate owner: %v", err)
	}

	_, input, err := model.NewIssueTx(issuerPriv, ownerPub, "credits", 100, 200)
	if err != nil {
		t.Fatalf("new issue tx: %v", err)
	}
	output := model.Value{
		Amount:     input.Amount,
		Unit:       input.Unit,
		Owner:      input.Owner,
		Issuer:     input.Issuer,
		ExpiryUnix: input.ExpiryUnix,
	}
	tx, err := model.NewTransferTx(ownerPriv, []model.Value{input}, []model.Value{output})
	if err != nil {
		t.Fatalf("new transfer tx: %v", err)
	}

	return issuerPub, tx, []model.Value{input}
}

func trustedNode(id string, issuer crypto.PublicKey) *SimNode {
	return &SimNode{
		ID: id,
		Policy: &policy.Policy{
			TrustedIssuers: map[string]float64{policy.IssuerKey(issuer): 1.0},
			MinScore:       0.5,
			MaxDepth:       10,
			NowUnix:        func() int64 { return 100 },
		},
	}
}

func untrustedNode(id string) *SimNode {
	return &SimNode{
		ID: id,
		Policy: &policy.Policy{
			TrustedIssuers: map[string]float64{},
			MinScore:       0.5,
			MaxDepth:       10,
			NowUnix:        func() int64 { return 100 },
		},
	}
}

func assertVector(t *testing.T, got []float64, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("vector length %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("vector[%d] = %f, want %f", i, got[i], want[i])
		}
	}
}

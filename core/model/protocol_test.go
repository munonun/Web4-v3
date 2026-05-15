package model

import (
	"testing"

	"web4-v3/core/crypto"
)

func TestProtocolIssueTxCreatesInventoryBackedValue(t *testing.T) {
	issuer := testNode(t)
	owner := testNode(t)
	unit := testUnit(t, issuer, "SKUG")
	output := testValue(t, unit, 10, owner, 100)
	tx := IssueTx{
		Unit:      unit,
		Outputs:   []Value{output},
		Issuer:    issuer,
		Timestamp: 100,
	}
	tx.ID = mustIssueTxID(t, tx)

	inv, err := ApplyStructuralIssueTx(NewInventoryState(), tx)
	if err != nil {
		t.Fatalf("apply issue: %v", err)
	}
	if got := inv.Get(owner, unit); got != 10 {
		t.Fatalf("holding %d, want 10", got)
	}
}

func TestStructuralIssueValidationDoesNotAuthorizeUnsignedIssue(t *testing.T) {
	issuer := testNode(t)
	owner := testNode(t)
	unit := testUnit(t, issuer, "SKUG")
	output := testValue(t, unit, 10, owner, 100)
	tx := IssueTx{
		Unit:      unit,
		Outputs:   []Value{output},
		Issuer:    issuer,
		Timestamp: 100,
	}
	tx.ID = mustIssueTxID(t, tx)

	if err := ValidateStructuralIssueTx(&tx); err != nil {
		t.Fatalf("structural validation: %v", err)
	}
	if err := ValidateIssueTx(&tx, output); err == nil {
		t.Fatal("expected signed issue validation to reject unsigned structural issue")
	}
}

func TestProtocolTradeTxPreservesValuePerUnit(t *testing.T) {
	a := testNode(t)
	b := testNode(t)
	skug := testUnit(t, a, "SKUG")
	web4 := testUnit(t, b, "WEB4")
	inputA := testValue(t, skug, 4, a, 1)
	inputB := testValue(t, web4, 2, b, 2)
	outputA := testValue(t, web4, 2, a, 3)
	outputB := testValue(t, skug, 4, b, 4)

	tx := TradeTx{
		InputsA:   []Value{inputA},
		InputsB:   []Value{inputB},
		OutputsA:  []Value{outputA},
		OutputsB:  []Value{outputB},
		PartyA:    a,
		PartyB:    b,
		Timestamp: 10,
	}
	tx.ID = mustTradeTxID(t, tx)

	inv := NewInventoryState()
	inv.Add(a, skug, 4)
	inv.Add(b, web4, 2)
	next, err := ApplyTradeTx(inv, tx)
	if err != nil {
		t.Fatalf("apply trade: %v", err)
	}
	if got := next.Get(a, skug); got != 0 {
		t.Fatalf("party A SKUG %d, want 0", got)
	}
	if got := next.Get(a, web4); got != 2 {
		t.Fatalf("party A WEB4 %d, want 2", got)
	}
	if got := next.Get(b, skug); got != 4 {
		t.Fatalf("party B SKUG %d, want 4", got)
	}
	if got := next.Get(b, web4); got != 0 {
		t.Fatalf("party B WEB4 %d, want 0", got)
	}
}

func TestProtocolTradeTxRejectsNonConservation(t *testing.T) {
	a := testNode(t)
	b := testNode(t)
	unit := testUnit(t, a, "SKUG")
	input := testValue(t, unit, 4, a, 1)
	output := testValue(t, unit, 5, b, 2)
	tx := TradeTx{
		InputsA:   []Value{input},
		OutputsB:  []Value{output},
		PartyA:    a,
		PartyB:    b,
		Timestamp: 10,
	}
	tx.ID = mustTradeTxID(t, tx)
	inv := NewInventoryState()
	inv.Add(a, unit, 4)

	if err := ValidateTradeTx(&tx, inv); err == nil {
		t.Fatal("expected conservation failure")
	}
}

func TestProtocolTradeTxExactFixedPointConservation(t *testing.T) {
	a := testNode(t)
	b := testNode(t)
	unit := testUnit(t, a, "SKUG")
	inputA := testValue(t, unit, FromFloat(0.1), a, 1)
	inputB := testValue(t, unit, FromFloat(0.2), a, 2)
	output := testValue(t, unit, FromFloat(0.3), b, 3)
	tx := TradeTx{
		InputsA:   []Value{inputA, inputB},
		OutputsB:  []Value{output},
		PartyA:    a,
		PartyB:    b,
		Timestamp: 10,
	}
	tx.ID = mustTradeTxID(t, tx)
	inv := NewInventoryState()
	inv.Add(a, unit, FromFloat(0.3))

	if err := ValidateTradeTx(&tx, inv); err != nil {
		t.Fatalf("exact fixed-point trade should validate: %v", err)
	}
}

func TestProtocolInventoryNeverNegative(t *testing.T) {
	node := testNode(t)
	unit := testUnit(t, node, "SKUG")
	inv := NewInventoryState()
	inv.Add(node, unit, 1)

	if err := inv.Sub(node, unit, 2); err == nil {
		t.Fatal("expected insufficient inventory")
	}
	if got := inv.Get(node, unit); got != 1 {
		t.Fatalf("holding mutated to %d, want 1", got)
	}
}

func TestAcceptanceRecordBoundsAndPriceQuote(t *testing.T) {
	node := testNode(t)
	unit := testUnit(t, node, "SKUG")
	record := AcceptanceRecord{Node: node, TargetID: hashBytes(unit), Score: 0.7, Timestamp: 9}
	if err := ValidateAcceptanceRecord(record); err != nil {
		t.Fatalf("validate acceptance: %v", err)
	}
	quote := PriceQuoteFromAcceptance(record, unit)
	if quote.Price != 0.7 || quote.Node != node || quote.Unit != unit || quote.Timestamp != 9 {
		t.Fatalf("bad quote: %+v", quote)
	}
	record.Score = 1.1
	if err := ValidateAcceptanceRecord(record); err == nil {
		t.Fatal("expected out-of-range acceptance to fail")
	}
}

func TestFlowRecordUpdates(t *testing.T) {
	unit := testUnit(t, testNode(t), "SKUG")
	flow := FlowRecord{Unit: unit}
	flow.AddTrade(4)
	flow.AddPayment(2)
	flow.AddConsumption(1)
	flow.AddDemandFulfilled(3)
	flow.AddTrade(-10)

	if flow.TradeVolume != 4 || flow.PaymentVolume != 2 || flow.Consumption != 1 || flow.DemandFulfilled != 3 {
		t.Fatalf("bad flow record: %+v", flow)
	}
}

func TestTradeQuoteCorrectness(t *testing.T) {
	a := testNode(t)
	b := testNode(t)
	sell := testUnit(t, a, "SKUG")
	buy := testUnit(t, b, "WEB4")

	quote := QuoteTrade(a, b, sell, buy, 2, 3)
	if !quote.Executable {
		t.Fatal("expected positive bilateral quote to be executable")
	}
	quote = QuoteTrade(a, a, sell, buy, 2, 3)
	if quote.Executable {
		t.Fatal("expected self-trade quote to be non-executable")
	}
	quote = QuoteTrade(a, b, sell, buy, 0, 3)
	if quote.Executable {
		t.Fatal("expected zero sell amount to be non-executable")
	}
}

func testNode(t *testing.T) NodeID {
	t.Helper()
	pub, _, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	id, err := NodeIDFromPublicKey(pub)
	if err != nil {
		t.Fatalf("node id: %v", err)
	}
	return id
}

func testUnit(t *testing.T, issuer NodeID, metadata string) UnitID {
	t.Helper()
	unit, err := NewUnitIDFromMetadata(issuer, []byte(metadata))
	if err != nil {
		t.Fatalf("unit id: %v", err)
	}
	return unit
}

func testValue(t *testing.T, unit UnitID, amount Amount, owner NodeID, createdAt int64) Value {
	t.Helper()
	value := Value{Unit: unit, Amount: amount, Owner: owner, CreatedAt: createdAt}
	id, err := ValueIDFor(value)
	if err != nil {
		t.Fatalf("value id: %v", err)
	}
	value.ID = id
	return value
}

func mustIssueTxID(t *testing.T, tx IssueTx) TxID {
	t.Helper()
	id, err := IssueTxID(tx)
	if err != nil {
		t.Fatalf("issue id: %v", err)
	}
	return id
}

func mustTradeTxID(t *testing.T, tx TradeTx) TxID {
	t.Helper()
	id, err := TradeTxID(tx)
	if err != nil {
		t.Fatalf("trade id: %v", err)
	}
	return id
}

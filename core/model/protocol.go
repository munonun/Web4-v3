package model

import (
	"crypto/ed25519"
	"fmt"
	"math"

	"web4-v3/core/canonical"
	"web4-v3/core/crypto"
)

type InventoryState struct {
	Holdings map[NodeID]map[UnitID]Amount
}

type AcceptanceRecord struct {
	Node      NodeID
	TargetID  []byte
	Score     float64
	Timestamp int64
}

type PriceQuote struct {
	Node      NodeID
	Unit      UnitID
	Price     float64
	Timestamp int64
}

type FlowRecord struct {
	Unit            UnitID
	TradeVolume     Amount
	PaymentVolume   Amount
	Consumption     Amount
	DemandFulfilled Amount
}

type TradeQuote struct {
	Seller     NodeID
	Buyer      NodeID
	SellUnit   UnitID
	BuyUnit    UnitID
	SellAmount Amount
	BuyAmount  Amount
	Executable bool
}

type Preference struct {
	Node    NodeID
	Utility map[UnitID]float64
}

func NodeIDFromPublicKey(pub crypto.PublicKey) (NodeID, error) {
	if len(pub) != ed25519.PublicKeySize {
		return NodeID{}, fmt.Errorf("invalid public key length: got %d, want %d", len(pub), ed25519.PublicKeySize)
	}
	var id NodeID
	copy(id[:], pub)
	return id, nil
}

func MustNodeIDFromPublicKey(pub crypto.PublicKey) NodeID {
	id, err := NodeIDFromPublicKey(pub)
	if err != nil {
		panic(err)
	}
	return id
}

func (id NodeID) Bytes() []byte {
	out := make([]byte, len(id))
	copy(out, id[:])
	return out
}

func (id NodeID) PublicKey() crypto.PublicKey {
	return crypto.PublicKey(id.Bytes())
}

// NewUnitIDFromMetadata derives a deterministic unit identifier from a local
// issuer and metadata. It does not require or imply a global registry.
func NewUnitIDFromMetadata(issuer NodeID, metadata []byte) (UnitID, error) {
	preimage, err := canonical.EncodeFields(
		canonical.Field{Name: "kind", Value: "unit"},
		canonical.Field{Name: "issuer", Value: issuer.Bytes()},
		canonical.Field{Name: "metadata", Value: append([]byte(nil), metadata...)},
	)
	if err != nil {
		return UnitID{}, err
	}

	return UnitID(crypto.HashBytes(preimage)), nil
}

func NewInventoryState() InventoryState {
	return InventoryState{Holdings: make(map[NodeID]map[UnitID]Amount)}
}

func (s *InventoryState) Add(node NodeID, unit UnitID, amount Amount) {
	if err := s.AddChecked(node, unit, amount); err != nil {
		panic(err)
	}
}

func (s *InventoryState) AddChecked(node NodeID, unit UnitID, amount Amount) error {
	if s.Holdings == nil {
		s.Holdings = make(map[NodeID]map[UnitID]Amount)
	}
	if !validAmount(amount) {
		return fmt.Errorf("amount must be greater than zero")
	}
	if s.Holdings[node] == nil {
		s.Holdings[node] = make(map[UnitID]Amount)
	}
	next, err := CheckedAdd(s.Holdings[node][unit], amount)
	if err != nil {
		return err
	}
	s.Holdings[node][unit] = next
	return nil
}

func (s *InventoryState) Sub(node NodeID, unit UnitID, amount Amount) error {
	if !validAmount(amount) {
		return fmt.Errorf("amount must be greater than zero")
	}
	current := s.Get(node, unit)
	if s.Holdings == nil || s.Holdings[node] == nil {
		return fmt.Errorf("insufficient inventory")
	}
	next, err := Sub(current, amount)
	if err != nil {
		return fmt.Errorf("insufficient inventory")
	}
	s.Holdings[node][unit] = next
	return nil
}

func (s *InventoryState) Get(node NodeID, unit UnitID) Amount {
	if s == nil || s.Holdings == nil || s.Holdings[node] == nil {
		return 0
	}
	return s.Holdings[node][unit]
}

func (s InventoryState) Copy() InventoryState {
	out := NewInventoryState()
	for node, units := range s.Holdings {
		out.Holdings[node] = make(map[UnitID]Amount, len(units))
		for unit, amount := range units {
			out.Holdings[node][unit] = amount
		}
	}
	return out
}

// ApplyStructuralIssueTx applies a structurally valid issuance transaction.
// It does not authorize issuance or verify an issuer signature; callers that
// need authorization must use ValidateIssueTx before updating accepted state.
func ApplyStructuralIssueTx(inv InventoryState, tx IssueTx) (InventoryState, error) {
	if err := ValidateStructuralIssueTx(&tx); err != nil {
		return InventoryState{}, err
	}
	next := inv.Copy()
	for _, output := range tx.Outputs {
		if err := next.AddChecked(output.Owner, output.Unit, output.Amount); err != nil {
			return InventoryState{}, fmt.Errorf("issue output overflows inventory: %w", err)
		}
	}
	return next, nil
}

func ApplyTradeTx(inv InventoryState, tx TradeTx) (InventoryState, error) {
	if err := ValidateTradeTx(&tx, inv); err != nil {
		return InventoryState{}, err
	}
	next := inv.Copy()
	for _, input := range tx.InputsA {
		if err := next.Sub(tx.PartyA, input.Unit, input.Amount); err != nil {
			return InventoryState{}, err
		}
	}
	for _, input := range tx.InputsB {
		if err := next.Sub(tx.PartyB, input.Unit, input.Amount); err != nil {
			return InventoryState{}, err
		}
	}
	for _, output := range tx.OutputsA {
		if err := next.AddChecked(output.Owner, output.Unit, output.Amount); err != nil {
			return InventoryState{}, fmt.Errorf("party A output overflows inventory: %w", err)
		}
	}
	for _, output := range tx.OutputsB {
		if err := next.AddChecked(output.Owner, output.Unit, output.Amount); err != nil {
			return InventoryState{}, fmt.Errorf("party B output overflows inventory: %w", err)
		}
	}
	return next, nil
}

// ValidateStructuralIssueTx checks deterministic IDs and output structure only.
// It does not authorize issuance or verify an issuer signature.
func ValidateStructuralIssueTx(tx *IssueTx) error {
	if tx == nil {
		return fmt.Errorf("issue tx is nil")
	}
	if len(tx.Outputs) == 0 {
		return fmt.Errorf("issue outputs are required")
	}
	expectedTxID, err := IssueTxID(*tx)
	if err != nil {
		return err
	}
	if tx.ID != expectedTxID {
		return fmt.Errorf("issue tx id mismatch")
	}
	seen := make(map[ValueID]struct{}, len(tx.Outputs))
	for i, output := range tx.Outputs {
		if !validAmount(output.Amount) {
			return fmt.Errorf("output %d amount must be greater than zero", i)
		}
		if !sameHash(output.Unit, tx.Unit) {
			return fmt.Errorf("output %d unit mismatch", i)
		}
		if isZeroNodeID(output.Owner) {
			return fmt.Errorf("output %d owner is required", i)
		}
		expectedID, err := ValueIDFor(output)
		if err != nil {
			return fmt.Errorf("output %d: %w", i, err)
		}
		if output.ID != expectedID {
			return fmt.Errorf("output %d value id mismatch", i)
		}
		if _, ok := seen[output.ID]; ok {
			return fmt.Errorf("duplicate output value id")
		}
		seen[output.ID] = struct{}{}
	}
	return nil
}

func ValidateTradeTx(tx *TradeTx, inv InventoryState) error {
	if tx == nil {
		return fmt.Errorf("trade tx is nil")
	}
	if isZeroNodeID(tx.PartyA) || isZeroNodeID(tx.PartyB) {
		return fmt.Errorf("trade parties are required")
	}
	if tx.PartyA == tx.PartyB {
		return fmt.Errorf("trade parties must differ")
	}
	if len(tx.InputsA)+len(tx.InputsB) == 0 {
		return fmt.Errorf("trade inputs are required")
	}
	if len(tx.OutputsA)+len(tx.OutputsB) == 0 {
		return fmt.Errorf("trade outputs are required")
	}
	if err := validateValueSet(tx.InputsA, tx.PartyA, true); err != nil {
		return fmt.Errorf("party A inputs: %w", err)
	}
	if err := validateValueSet(tx.InputsB, tx.PartyB, true); err != nil {
		return fmt.Errorf("party B inputs: %w", err)
	}
	if err := validateValueSet(tx.OutputsA, tx.PartyA, false); err != nil {
		return fmt.Errorf("party A outputs: %w", err)
	}
	if err := validateValueSet(tx.OutputsB, tx.PartyB, false); err != nil {
		return fmt.Errorf("party B outputs: %w", err)
	}
	if err := checkNoDuplicateValueIDs(tx.InputsA, tx.InputsB, tx.OutputsA, tx.OutputsB); err != nil {
		return err
	}
	if err := checkTradeConservation(tx); err != nil {
		return err
	}
	expectedTxID, err := TradeTxID(*tx)
	if err != nil {
		return err
	}
	if tx.ID != expectedTxID {
		return fmt.Errorf("trade tx id mismatch")
	}
	for _, input := range tx.InputsA {
		if inv.Get(tx.PartyA, input.Unit) < input.Amount {
			return fmt.Errorf("party A input does not exist in inventory")
		}
	}
	for _, input := range tx.InputsB {
		if inv.Get(tx.PartyB, input.Unit) < input.Amount {
			return fmt.Errorf("party B input does not exist in inventory")
		}
	}
	return nil
}

func validateValueSet(values []Value, owner NodeID, requireOwned bool) error {
	for i, value := range values {
		if !validAmount(value.Amount) {
			return fmt.Errorf("value %d amount must be greater than zero", i)
		}
		if requireOwned && value.Owner != owner {
			return fmt.Errorf("value %d owner mismatch", i)
		}
		if !requireOwned && isZeroNodeID(value.Owner) {
			return fmt.Errorf("value %d owner is required", i)
		}
		expectedID, err := ValueIDFor(value)
		if err != nil {
			return fmt.Errorf("value %d: %w", i, err)
		}
		if value.ID != expectedID {
			return fmt.Errorf("value %d id mismatch", i)
		}
	}
	return nil
}

func checkNoDuplicateValueIDs(groups ...[]Value) error {
	seen := make(map[ValueID]struct{})
	for _, group := range groups {
		for _, value := range group {
			if _, ok := seen[value.ID]; ok {
				return fmt.Errorf("duplicate value id")
			}
			seen[value.ID] = struct{}{}
		}
	}
	return nil
}

func checkTradeConservation(tx *TradeTx) error {
	inputs := make(map[UnitID]Amount)
	outputs := make(map[UnitID]Amount)
	for i, value := range append(append([]Value{}, tx.InputsA...), tx.InputsB...) {
		next, err := CheckedAdd(inputs[value.Unit], value.Amount)
		if err != nil {
			return fmt.Errorf("input %d amount overflow", i)
		}
		inputs[value.Unit] = next
	}
	for i, value := range append(append([]Value{}, tx.OutputsA...), tx.OutputsB...) {
		next, err := CheckedAdd(outputs[value.Unit], value.Amount)
		if err != nil {
			return fmt.Errorf("output %d amount overflow", i)
		}
		outputs[value.Unit] = next
	}
	if len(inputs) != len(outputs) {
		return fmt.Errorf("trade changes unit set")
	}
	for unit, amount := range inputs {
		if amount != outputs[unit] {
			return fmt.Errorf("unit %x is not conserved", hashBytes(unit))
		}
	}
	return nil
}

func ValidateAcceptanceRecord(record AcceptanceRecord) error {
	if isZeroNodeID(record.Node) {
		return fmt.Errorf("node is required")
	}
	if len(record.TargetID) == 0 {
		return fmt.Errorf("target id is required")
	}
	if record.Score < 0 || record.Score > 1 || math.IsNaN(record.Score) {
		return fmt.Errorf("acceptance score must be in [0,1]")
	}
	return nil
}

func PriceFromAcceptance(score float64) float64 {
	if score < 0 || math.IsNaN(score) {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func PriceQuoteFromAcceptance(record AcceptanceRecord, unit UnitID) PriceQuote {
	return PriceQuote{
		Node:      record.Node,
		Unit:      unit,
		Price:     PriceFromAcceptance(record.Score),
		Timestamp: record.Timestamp,
	}
}

func (f *FlowRecord) AddTrade(amount Amount) {
	if validFlowAmount(amount) {
		f.TradeVolume = Add(f.TradeVolume, amount)
	}
}

func (f *FlowRecord) AddPayment(amount Amount) {
	if validFlowAmount(amount) {
		f.PaymentVolume = Add(f.PaymentVolume, amount)
	}
}

func (f *FlowRecord) AddConsumption(amount Amount) {
	if validFlowAmount(amount) {
		f.Consumption = Add(f.Consumption, amount)
	}
}

func (f *FlowRecord) AddDemandFulfilled(amount Amount) {
	if validFlowAmount(amount) {
		f.DemandFulfilled = Add(f.DemandFulfilled, amount)
	}
}

func QuoteTrade(seller, buyer NodeID, sellUnit, buyUnit UnitID, sellAmount, buyAmount Amount) TradeQuote {
	return TradeQuote{
		Seller:     seller,
		Buyer:      buyer,
		SellUnit:   sellUnit,
		BuyUnit:    buyUnit,
		SellAmount: sellAmount,
		BuyAmount:  buyAmount,
		Executable: !isZeroNodeID(seller) && !isZeroNodeID(buyer) && seller != buyer && validAmount(sellAmount) && validAmount(buyAmount),
	}
}

func isZeroNodeID(id NodeID) bool {
	return id == (NodeID{})
}

func EffectiveValue(price float64, pref Preference, unit UnitID) float64 {
	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return 0
	}
	utility := 1.0
	if pref.Utility != nil {
		if u, ok := pref.Utility[unit]; ok {
			utility = u
		}
	}
	if utility <= 0 || math.IsNaN(utility) || math.IsInf(utility, 0) {
		return 0
	}
	return price * utility
}

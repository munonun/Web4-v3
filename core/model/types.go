package model

import "web4-v3/core/crypto"

type UnitID [32]byte
type ValueID [32]byte
type TxID [32]byte
type NodeID [32]byte

type Value struct {
	ID        ValueID
	Unit      UnitID
	Amount    Amount
	Owner     NodeID
	CreatedAt int64

	// Legacy local-policy metadata retained for the v2 compatibility helpers.
	Issuer     NodeID
	ExpiryUnix int64
	Depth      uint32
}

type IssueTx struct {
	ID        TxID
	Unit      UnitID
	Outputs   []Value
	Issuer    NodeID
	Timestamp int64

	// Legacy signed-issue fields retained for existing local policy flows.
	UnitName   string
	Amount     Amount
	Owner      NodeID
	ExpiryUnix int64
	Signature  crypto.Signature
}

type TradeTx struct {
	ID        TxID
	InputsA   []Value
	InputsB   []Value
	OutputsA  []Value
	OutputsB  []Value
	PartyA    NodeID
	PartyB    NodeID
	Timestamp int64
}

type TransferTx struct {
	ID        TxID
	Inputs    []ValueID
	Outputs   []Value
	Author    NodeID
	Signature crypto.Signature
}

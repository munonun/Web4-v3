package model

import "web4-v3/core/crypto"

type UnitID crypto.Hash
type ValueID crypto.Hash
type TxID crypto.Hash

type Value struct {
	ID         ValueID
	Amount     uint64
	Unit       UnitID
	Owner      crypto.PublicKey
	Issuer     crypto.PublicKey
	ExpiryUnix int64
	Depth      uint32
}

type IssueTx struct {
	ID         TxID
	UnitName   string
	Unit       UnitID
	Amount     uint64
	Issuer     crypto.PublicKey
	Owner      crypto.PublicKey
	ExpiryUnix int64
	Signature  crypto.Signature
}

type TransferTx struct {
	ID        TxID
	Inputs    []ValueID
	Outputs   []Value
	Author    crypto.PublicKey
	Signature crypto.Signature
}

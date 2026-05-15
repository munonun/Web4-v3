package store

import (
	"web4-v3/core/model"
	"web4-v3/core/node"
	"web4-v3/core/price"
)

type Store interface {
	HasExecutedTrade(id model.TxID) bool
	MarkExecutedTrade(id model.TxID) error

	SaveInventory(node model.NodeID, inv model.InventoryState) error
	LoadInventory(node model.NodeID) (model.InventoryState, error)

	SaveFlow(node model.NodeID, flow map[model.UnitID]model.FlowRecord) error
	LoadFlow(node model.NodeID) (map[model.UnitID]model.FlowRecord, error)

	SavePriceState(node model.NodeID, state map[model.UnitID]price.PriceResult) error
	LoadPriceState(node model.NodeID) (map[model.UnitID]price.PriceResult, error)

	SaveAuthorizedTrade(id model.TxID, tx node.AuthorizedTradeTx) error
	LoadAuthorizedTrade(id model.TxID) (node.AuthorizedTradeTx, bool)

	Close() error
}

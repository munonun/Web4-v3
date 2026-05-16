package node

import (
	"fmt"

	"web4-v3/core/model"
	"web4-v3/core/price"
)

type PersistedNodeState struct {
	ID         model.NodeID
	Inventory  model.InventoryState
	Flow       map[model.UnitID]model.FlowRecord
	PriceState map[model.UnitID]price.PriceResult
}

type Store interface {
	HasExecutedTrade(id model.TxID) bool
	MarkExecutedTrade(id model.TxID) error

	SaveInventory(node model.NodeID, inv model.InventoryState) error
	LoadInventory(node model.NodeID) (model.InventoryState, error)

	SaveFlow(node model.NodeID, flow map[model.UnitID]model.FlowRecord) error
	LoadFlow(node model.NodeID) (map[model.UnitID]model.FlowRecord, error)

	SavePriceState(node model.NodeID, state map[model.UnitID]price.PriceResult) error
	LoadPriceState(node model.NodeID) (map[model.UnitID]price.PriceResult, error)

	SaveAuthorizedTrade(id model.TxID, tx AuthorizedTradeTx) error
	LoadAuthorizedTrade(id model.TxID) (AuthorizedTradeTx, bool)

	PersistExecutedTrade(id model.TxID, tx AuthorizedTradeTx, states ...PersistedNodeState) error

	Close() error
}

func RejectReplay(store Store, id model.TxID) error {
	if store == nil {
		return nil
	}
	if store.HasExecutedTrade(id) {
		return fmt.Errorf("trade replay rejected")
	}
	return store.MarkExecutedTrade(id)
}

func rejectReplayCheck(store Store, id model.TxID) error {
	if store == nil {
		return nil
	}
	if store.HasExecutedTrade(id) {
		return fmt.Errorf("trade replay rejected")
	}
	return nil
}

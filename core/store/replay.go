package store

import (
	"fmt"

	"web4-v3/core/model"
)

func RejectReplay(store Store, id model.TxID) error {
	if store == nil {
		return nil
	}
	if store.HasExecutedTrade(id) {
		return fmt.Errorf("trade replay rejected")
	}
	return store.MarkExecutedTrade(id)
}

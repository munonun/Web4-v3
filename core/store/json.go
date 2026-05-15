package store

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"web4-v3/core/model"
	"web4-v3/core/node"
	"web4-v3/core/price"
)

type JSONStore struct {
	root string
}

func NewJSONStore(root string) (*JSONStore, error) {
	s := &JSONStore{root: root}
	for _, dir := range []string{"trades", "inventory", "flow", "prices"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *JSONStore) HasExecutedTrade(id model.TxID) bool {
	_, err := os.Stat(s.executedPath(id))
	return err == nil
}

func (s *JSONStore) MarkExecutedTrade(id model.TxID) error {
	return writeJSONAtomic(s.executedPath(id), map[string]string{"id": idHex(id)})
}

func (s *JSONStore) SaveInventory(id model.NodeID, inv model.InventoryState) error {
	dto := inventoryDTO{Node: nodeHex(id), Holdings: map[string]model.Amount{}}
	for unit, amount := range inv.Holdings[id] {
		dto.Holdings[unitHex(unit)] = amount
	}
	return writeJSONAtomic(s.inventoryPath(id), dto)
}

func (s *JSONStore) LoadInventory(id model.NodeID) (model.InventoryState, error) {
	var dto inventoryDTO
	if err := readJSONIfExists(s.inventoryPath(id), &dto); err != nil {
		return model.InventoryState{}, err
	}
	inv := model.NewInventoryState()
	for unitString, amount := range dto.Holdings {
		unit, err := parseUnitID(unitString)
		if err != nil {
			return model.InventoryState{}, err
		}
		inv.Add(id, unit, amount)
	}
	return inv, nil
}

func (s *JSONStore) SaveFlow(id model.NodeID, flow map[model.UnitID]model.FlowRecord) error {
	dto := flowDTO{Node: nodeHex(id), Records: map[string]flowRecordDTO{}}
	for unit, record := range flow {
		dto.Records[unitHex(unit)] = flowRecordDTO{
			TradeVolume:     record.TradeVolume,
			PaymentVolume:   record.PaymentVolume,
			Consumption:     record.Consumption,
			DemandFulfilled: record.DemandFulfilled,
		}
	}
	return writeJSONAtomic(s.flowPath(id), dto)
}

func (s *JSONStore) LoadFlow(id model.NodeID) (map[model.UnitID]model.FlowRecord, error) {
	var dto flowDTO
	if err := readJSONIfExists(s.flowPath(id), &dto); err != nil {
		return nil, err
	}
	out := make(map[model.UnitID]model.FlowRecord, len(dto.Records))
	for unitString, record := range dto.Records {
		unit, err := parseUnitID(unitString)
		if err != nil {
			return nil, err
		}
		out[unit] = model.FlowRecord{
			Unit:            unit,
			TradeVolume:     record.TradeVolume,
			PaymentVolume:   record.PaymentVolume,
			Consumption:     record.Consumption,
			DemandFulfilled: record.DemandFulfilled,
		}
	}
	return out, nil
}

func (s *JSONStore) SavePriceState(id model.NodeID, state map[model.UnitID]price.PriceResult) error {
	dto := priceDTO{Node: nodeHex(id), Prices: map[string]price.PriceResult{}}
	for unit, result := range state {
		dto.Prices[unitHex(unit)] = result
	}
	return writeJSONAtomic(s.pricePath(id), dto)
}

func (s *JSONStore) LoadPriceState(id model.NodeID) (map[model.UnitID]price.PriceResult, error) {
	var dto priceDTO
	if err := readJSONIfExists(s.pricePath(id), &dto); err != nil {
		return nil, err
	}
	out := make(map[model.UnitID]price.PriceResult, len(dto.Prices))
	for unitString, result := range dto.Prices {
		unit, err := parseUnitID(unitString)
		if err != nil {
			return nil, err
		}
		out[unit] = result
	}
	return out, nil
}

func (s *JSONStore) SaveAuthorizedTrade(id model.TxID, tx node.AuthorizedTradeTx) error {
	return writeJSONAtomic(s.tradePath(id), tx)
}

func (s *JSONStore) LoadAuthorizedTrade(id model.TxID) (node.AuthorizedTradeTx, bool) {
	var tx node.AuthorizedTradeTx
	if err := readJSONIfExists(s.tradePath(id), &tx); err != nil {
		return node.AuthorizedTradeTx{}, false
	}
	return tx, true
}

func (s *JSONStore) Close() error {
	return nil
}

func (s *JSONStore) executedPath(id model.TxID) string {
	return filepath.Join(s.root, "trades", idHex(id)+".executed.json")
}

func (s *JSONStore) tradePath(id model.TxID) string {
	return filepath.Join(s.root, "trades", idHex(id)+".json")
}

func (s *JSONStore) inventoryPath(id model.NodeID) string {
	return filepath.Join(s.root, "inventory", nodeHex(id)+".json")
}

func (s *JSONStore) flowPath(id model.NodeID) string {
	return filepath.Join(s.root, "flow", nodeHex(id)+".json")
}

func (s *JSONStore) pricePath(id model.NodeID) string {
	return filepath.Join(s.root, "prices", nodeHex(id)+".json")
}

type inventoryDTO struct {
	Node     string                  `json:"node"`
	Holdings map[string]model.Amount `json:"holdings"`
}

type flowDTO struct {
	Node    string                   `json:"node"`
	Records map[string]flowRecordDTO `json:"records"`
}

type flowRecordDTO struct {
	TradeVolume     model.Amount `json:"trade_volume"`
	PaymentVolume   model.Amount `json:"payment_volume"`
	Consumption     model.Amount `json:"consumption"`
	DemandFulfilled model.Amount `json:"demand_fulfilled"`
}

type priceDTO struct {
	Node   string                       `json:"node"`
	Prices map[string]price.PriceResult `json:"prices"`
}

func writeJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSONIfExists(path string, out any) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

func nodeHex(id model.NodeID) string {
	return hex.EncodeToString(id[:])
}

func unitHex(id model.UnitID) string {
	return hex.EncodeToString(id[:])
}

func idHex(id model.TxID) string {
	return hex.EncodeToString(id[:])
}

func parseUnitID(s string) (model.UnitID, error) {
	var id model.UnitID
	b, err := hex.DecodeString(s)
	if err != nil {
		return id, err
	}
	if len(b) != len(id) {
		return id, fmt.Errorf("invalid unit id length")
	}
	copy(id[:], b)
	return id, nil
}

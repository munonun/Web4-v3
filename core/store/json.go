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
	return writeJSONAtomic(s.inventoryPath(id), inventoryDTOFor(id, inv))
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
	return writeJSONAtomic(s.flowPath(id), flowDTOFor(id, flow))
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
	return writeJSONAtomic(s.pricePath(id), priceDTOFor(id, state))
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
	data, err := os.ReadFile(s.tradePath(id))
	if errors.Is(err, os.ErrNotExist) {
		return node.AuthorizedTradeTx{}, false
	}
	if err != nil || len(data) == 0 {
		return node.AuthorizedTradeTx{}, false
	}
	var tx node.AuthorizedTradeTx
	if err := json.Unmarshal(data, &tx); err != nil {
		return node.AuthorizedTradeTx{}, false
	}
	return tx, true
}

// PersistExecutedTrade stages every JSON document to a same-directory temp file
// before committing the replay marker, authorized trade, and node state files.
// A filesystem crash between renames can still leave a partial local commit;
// because the replay marker is renamed first, recovery rejects the signed trade
// instead of replaying it against possibly changed state.
func (s *JSONStore) PersistExecutedTrade(id model.TxID, tx node.AuthorizedTradeTx, states ...node.PersistedNodeState) error {
	if s.HasExecutedTrade(id) {
		return fmt.Errorf("trade replay rejected")
	}

	files := []atomicJSONFile{
		{path: s.executedPath(id), value: map[string]string{"id": idHex(id)}},
		{path: s.tradePath(id), value: tx},
	}
	for _, state := range states {
		files = append(files,
			atomicJSONFile{path: s.inventoryPath(state.ID), value: inventoryDTOFor(state.ID, state.Inventory)},
			atomicJSONFile{path: s.flowPath(state.ID), value: flowDTOFor(state.ID, state.Flow)},
			atomicJSONFile{path: s.pricePath(state.ID), value: priceDTOFor(state.ID, state.PriceState)},
		)
	}
	return writeJSONBatchAtomic(files)
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

func inventoryDTOFor(id model.NodeID, inv model.InventoryState) inventoryDTO {
	dto := inventoryDTO{Node: nodeHex(id), Holdings: map[string]model.Amount{}}
	for unit, amount := range inv.Holdings[id] {
		dto.Holdings[unitHex(unit)] = amount
	}
	return dto
}

func flowDTOFor(id model.NodeID, flow map[model.UnitID]model.FlowRecord) flowDTO {
	dto := flowDTO{Node: nodeHex(id), Records: map[string]flowRecordDTO{}}
	for unit, record := range flow {
		dto.Records[unitHex(unit)] = flowRecordDTO{
			TradeVolume:     record.TradeVolume,
			PaymentVolume:   record.PaymentVolume,
			Consumption:     record.Consumption,
			DemandFulfilled: record.DemandFulfilled,
		}
	}
	return dto
}

func priceDTOFor(id model.NodeID, state map[model.UnitID]price.PriceResult) priceDTO {
	dto := priceDTO{Node: nodeHex(id), Prices: map[string]price.PriceResult{}}
	for unit, result := range state {
		dto.Prices[unitHex(unit)] = result
	}
	return dto
}

type atomicJSONFile struct {
	path  string
	value any
}

type stagedJSONFile struct {
	path string
	tmp  string
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

func writeJSONBatchAtomic(files []atomicJSONFile) error {
	staged := make([]stagedJSONFile, 0, len(files))
	defer func() {
		for _, file := range staged {
			if file.tmp != "" {
				_ = os.Remove(file.tmp)
			}
		}
	}()

	for _, file := range files {
		tmp, err := stageJSONFile(file.path, file.value)
		if err != nil {
			return err
		}
		staged = append(staged, stagedJSONFile{path: file.path, tmp: tmp})
	}

	for i := range staged {
		if err := os.Rename(staged[i].tmp, staged[i].path); err != nil {
			return err
		}
		staged[i].tmp = ""
	}
	return nil
}

func stageJSONFile(path string, value any) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return "", err
	}
	tmp := file.Name()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return tmp, nil
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

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
	if err := s.recoverTransactions(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *JSONStore) HasExecutedTrade(id model.TxID) bool {
	return exists(s.executedPath(id)) ||
		exists(s.committedPath(id)) ||
		exists(s.reservationPath(id)) ||
		exists(s.manifestPath(id))
}

func (s *JSONStore) MarkExecutedTrade(id model.TxID) error {
	return writeExecutedMarkerExclusive(s.executedPath(id), id)
}

func (s *JSONStore) SaveInventory(id model.NodeID, inv model.InventoryState) error {
	return writeJSONAtomic(s.inventoryPath(id), inventoryDTOFor(id, inv))
}

func (s *JSONStore) LoadInventory(id model.NodeID) (model.InventoryState, error) {
	var dto inventoryDTO
	if err := readJSONIfExists(s.inventoryPath(id), &dto); err != nil {
		return model.InventoryState{}, err
	}
	if dto.Node == "" && dto.Holdings == nil {
		return model.NewInventoryState(), nil
	}
	if err := requireNodeMatch(dto.Node, id); err != nil {
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
	if dto.Node == "" && dto.Records == nil {
		return map[model.UnitID]model.FlowRecord{}, nil
	}
	if err := requireNodeMatch(dto.Node, id); err != nil {
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
	if dto.Node == "" && dto.Prices == nil {
		return map[model.UnitID]price.PriceResult{}, nil
	}
	if err := requireNodeMatch(dto.Node, id); err != nil {
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

// PersistExecutedTrade stages every JSON document to a same-directory temp file,
// commits trade and node state first, writes a transaction commit record, and
// creates the replay marker last with an exclusive create. Recovery treats any
// surviving commit or reservation artifact as replay evidence so a crash
// cannot make the same signed trade executable again.
func (s *JSONStore) PersistExecutedTrade(id model.TxID, tx node.AuthorizedTradeTx, states ...node.PersistedNodeState) error {
	if s.HasExecutedTrade(id) {
		return fmt.Errorf("trade replay rejected")
	}

	files := []atomicJSONFile{
		{path: s.tradePath(id), value: tx},
	}
	for _, state := range states {
		files = append(files,
			atomicJSONFile{path: s.inventoryPath(state.ID), value: inventoryDTOFor(state.ID, state.Inventory)},
			atomicJSONFile{path: s.flowPath(state.ID), value: flowDTOFor(state.ID, state.Flow)},
			atomicJSONFile{path: s.pricePath(state.ID), value: priceDTOFor(state.ID, state.PriceState)},
		)
	}
	reservation, err := reserveExecutedTrade(s.reservationPath(id), id)
	if err != nil {
		return err
	}
	defer func() {
		if reservation != "" {
			_ = os.Remove(reservation)
		}
	}()
	manifestPath := s.manifestPath(id)
	targets, err := relativeTargets(s.root, files)
	if err != nil {
		return err
	}
	if err := writeTransactionManifest(manifestPath, transactionManifest{ID: idHex(id), Phase: "preparing", Targets: targets}); err != nil {
		return err
	}
	defer func() {
		if manifestPath != "" {
			_ = os.Remove(manifestPath)
		}
	}()
	batch, err := writeJSONBatchAtomic(files, func(phase string) error {
		return writeTransactionManifest(manifestPath, transactionManifest{ID: idHex(id), Phase: phase, Targets: targets})
	})
	if err != nil {
		return err
	}
	if err := writeCommitRecordExclusive(s.committedPath(id), id); err != nil {
		batch.rollback()
		return err
	}
	if err := writeExecutedMarkerExclusive(s.executedPath(id), id); err != nil {
		return err
	}
	batch.cleanup()
	_ = os.Remove(s.committedPath(id))
	_ = os.Remove(manifestPath)
	manifestPath = ""
	return nil
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

func (s *JSONStore) committedPath(id model.TxID) string {
	return filepath.Join(s.root, "trades", idHex(id)+".committed.json")
}

func (s *JSONStore) manifestPath(id model.TxID) string {
	return filepath.Join(s.root, "trades", idHex(id)+".txn.json")
}

func (s *JSONStore) reservationPath(id model.TxID) string {
	return filepath.Join(s.root, "trades", idHex(id)+".executing")
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

type transactionManifest struct {
	ID      string   `json:"id"`
	Phase   string   `json:"phase"`
	Targets []string `json:"targets"`
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
	path      string
	tmp       string
	backup    string
	hadBackup bool
	installed bool
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

func reserveExecutedTrade(path string, id model.TxID) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("trade execution already in progress")
	}
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(map[string]string{"id": idHex(id), "state": "reserved"}, "", "  ")
	if err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	_ = fsyncDir(filepath.Dir(path))
	return path, nil
}

func writeExecutedMarkerExclusive(path string, id model.TxID) error {
	return writeTradeRecordExclusive(path, id, "executed")
}

func writeCommitRecordExclusive(path string, id model.TxID) error {
	return writeTradeRecordExclusive(path, id, "committed")
}

func writeTradeRecordExclusive(path string, id model.TxID, state string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("trade replay rejected")
	}
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(map[string]string{"id": idHex(id), "state": state}, "", "  ")
	if err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	_ = fsyncDir(filepath.Dir(path))
	return nil
}

func writeJSONBatchAtomic(files []atomicJSONFile, phase func(string) error) (*committedJSONBatch, error) {
	staged := make([]stagedJSONFile, 0, len(files))
	defer func() {
		for _, file := range staged {
			if file.tmp != "" {
				_ = os.Remove(file.tmp)
			}
		}
	}()

	for _, file := range files {
		if err := preflightTarget(file.path); err != nil {
			return nil, err
		}
		tmp, err := stageJSONFile(file.path, file.value)
		if err != nil {
			return nil, err
		}
		staged = append(staged, stagedJSONFile{path: file.path, tmp: tmp})
	}

	if phase != nil {
		if err := phase("applying"); err != nil {
			batch := &committedJSONBatch{files: staged}
			batch.rollback()
			return nil, err
		}
	}
	for i := range staged {
		backup, hadBackup, err := moveExistingAside(staged[i].path)
		if err != nil {
			batch := &committedJSONBatch{files: staged}
			batch.rollback()
			return nil, err
		}
		staged[i].backup = backup
		staged[i].hadBackup = hadBackup
		if err := os.Rename(staged[i].tmp, staged[i].path); err != nil {
			batch := &committedJSONBatch{files: staged}
			batch.rollback()
			return nil, err
		}
		staged[i].tmp = ""
		staged[i].installed = true
		_ = fsyncDir(filepath.Dir(staged[i].path))
	}
	if phase != nil {
		if err := phase("committed"); err != nil {
			batch := &committedJSONBatch{files: staged}
			batch.rollback()
			return nil, err
		}
	}
	return &committedJSONBatch{files: staged}, nil
}

type committedJSONBatch struct {
	files []stagedJSONFile
}

func (b *committedJSONBatch) rollback() {
	if b == nil {
		return
	}
	for i := len(b.files) - 1; i >= 0; i-- {
		file := b.files[i]
		if file.installed {
			_ = os.Remove(file.path)
		}
		if file.hadBackup {
			_ = os.Rename(file.backup, file.path)
		}
		if file.tmp != "" {
			_ = os.Remove(file.tmp)
		}
	}
}

func (b *committedJSONBatch) cleanup() {
	if b == nil {
		return
	}
	for _, file := range b.files {
		if file.hadBackup {
			_ = os.Remove(file.backup)
		}
		if file.tmp != "" {
			_ = os.Remove(file.tmp)
		}
	}
}

func preflightTarget(path string) error {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("target path is a directory: %s", path)
	}
	return nil
}

func moveExistingAside(path string) (string, bool, error) {
	if err := preflightTarget(path); err != nil {
		return "", false, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	} else if err != nil {
		return "", false, err
	}
	backupFile, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.bak")
	if err != nil {
		return "", false, err
	}
	backup := backupFile.Name()
	if err := backupFile.Close(); err != nil {
		_ = os.Remove(backup)
		return "", false, err
	}
	_ = os.Remove(backup)
	if err := os.Rename(path, backup); err != nil {
		return "", false, err
	}
	return backup, true, nil
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

func relativeTargets(root string, files []atomicJSONFile) ([]string, error) {
	targets := make([]string, 0, len(files))
	for _, file := range files {
		rel, err := filepath.Rel(root, file.path)
		if err != nil {
			return nil, err
		}
		if rel == "." || rel == ".." || len(rel) >= 3 && rel[:3] == "../" {
			return nil, fmt.Errorf("transaction target escapes store root: %s", file.path)
		}
		targets = append(targets, filepath.ToSlash(rel))
	}
	return targets, nil
}

func writeTransactionManifest(path string, manifest transactionManifest) error {
	return writeJSONAtomic(path, manifest)
}

func readTransactionManifest(path string) (transactionManifest, bool, error) {
	var manifest transactionManifest
	if !exists(path) {
		return manifest, false, nil
	}
	if err := readJSONIfExists(path, &manifest); err != nil {
		return transactionManifest{}, true, err
	}
	return manifest, true, nil
}

func (s *JSONStore) verifyTransactionManifest(id model.TxID, manifest transactionManifest) error {
	if manifest.ID != idHex(id) {
		return fmt.Errorf("transaction manifest id mismatch: got %q, want %q", manifest.ID, idHex(id))
	}
	if len(manifest.Targets) == 0 {
		return fmt.Errorf("transaction manifest has no targets")
	}
	for _, target := range manifest.Targets {
		path := filepath.Join(s.root, filepath.FromSlash(target))
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		if rel == "." || rel == ".." || len(rel) >= 3 && rel[:3] == "../" {
			return fmt.Errorf("transaction target escapes store root: %s", target)
		}
		if !exists(path) {
			return fmt.Errorf("committed transaction target missing: %s", target)
		}
	}
	return nil
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
		return fmt.Errorf("empty json file: %s", path)
	}
	return json.Unmarshal(data, out)
}

func (s *JSONStore) recoverTransactions() error {
	tradesDir := filepath.Join(s.root, "trades")
	entries, err := os.ReadDir(tradesDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		id, ok := tradeArtifactID(entry.Name())
		if !ok {
			continue
		}
		if exists(s.executedPath(id)) {
			_ = os.Remove(s.committedPath(id))
			_ = os.Remove(s.reservationPath(id))
			_ = os.Remove(s.manifestPath(id))
			continue
		}
		manifest, hasManifest, err := readTransactionManifest(s.manifestPath(id))
		if err != nil {
			return err
		}
		if exists(s.committedPath(id)) || (hasManifest && manifest.Phase == "committed") {
			if hasManifest {
				if err := s.verifyTransactionManifest(id, manifest); err != nil {
					return err
				}
			}
			if err := writeExecutedMarkerExclusive(s.executedPath(id), id); err != nil {
				return err
			}
			_ = os.Remove(s.committedPath(id))
			_ = os.Remove(s.reservationPath(id))
			_ = os.Remove(s.manifestPath(id))
			continue
		}
		if hasManifest {
			switch manifest.Phase {
			case "preparing":
				continue
			case "applying":
				return fmt.Errorf("incomplete json transaction for trade %x", id[:])
			default:
				return fmt.Errorf("unknown json transaction phase %q for trade %x", manifest.Phase, id[:])
			}
		}
	}
	return nil
}

func tradeArtifactID(name string) (model.TxID, bool) {
	for _, suffix := range []string{".committed.json", ".executing", ".txn.json", ".json"} {
		if !hasSuffix(name, suffix) {
			continue
		}
		idString := name[:len(name)-len(suffix)]
		b, err := hex.DecodeString(idString)
		if err != nil || len(b) != 32 {
			return model.TxID{}, false
		}
		var id model.TxID
		copy(id[:], b)
		return id, true
	}
	return model.TxID{}, false
}

func hasSuffix(s string, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func fsyncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func requireNodeMatch(got string, id model.NodeID) error {
	want := nodeHex(id)
	if got != want {
		return fmt.Errorf("node state id mismatch: got %q, want %q", got, want)
	}
	return nil
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

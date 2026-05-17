package node

import (
	"crypto/ed25519"
	"fmt"
	"sync"
	"time"

	"web4-v3/core/crypto"
	"web4-v3/core/model"
	"web4-v3/core/price"
)

type Node struct {
	mu sync.RWMutex

	ID model.NodeID

	PublicKey  crypto.PublicKey
	PrivateKey crypto.PrivateKey

	Inventory model.InventoryState

	Preferences map[model.UnitID]float64

	PriceState  map[model.UnitID]price.PriceResult
	PriceConfig price.PriceConfig

	Features map[model.UnitID]price.AssetFeatures

	TradeHistory  map[model.UnitID][]price.TradeObservation
	SettledVolume map[model.UnitID]model.Amount
	LastTradeUnix map[model.UnitID]int64

	Flow  map[model.UnitID]model.FlowRecord
	Store Store

	ExecutedTrades map[model.TxID]bool

	// AllowEphemeralReplayUnsafe permits signed execution without durable replay
	// storage. It is intended only for explicit tests and local simulations.
	AllowEphemeralReplayUnsafe bool

	NowUnix func() int64
}

func New(id model.NodeID) *Node {
	n := &Node{
		ID:             id,
		Inventory:      model.NewInventoryState(),
		Preferences:    make(map[model.UnitID]float64),
		PriceState:     make(map[model.UnitID]price.PriceResult),
		PriceConfig:    DefaultPriceConfig(),
		Features:       make(map[model.UnitID]price.AssetFeatures),
		TradeHistory:   make(map[model.UnitID][]price.TradeObservation),
		SettledVolume:  make(map[model.UnitID]model.Amount),
		LastTradeUnix:  make(map[model.UnitID]int64),
		Flow:           make(map[model.UnitID]model.FlowRecord),
		ExecutedTrades: make(map[model.TxID]bool),
		NowUnix:        func() int64 { return time.Now().Unix() },
	}
	return n
}

func NewNode(priv crypto.PrivateKey, cfg price.PriceConfig) (*Node, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key length: got %d, want %d", len(priv), ed25519.PrivateKeySize)
	}
	pub, ok := ed25519.PrivateKey(priv).Public().(ed25519.PublicKey)
	if !ok || len(pub) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid private key public component")
	}
	id, err := model.NodeIDFromPublicKey(crypto.PublicKey(pub))
	if err != nil {
		return nil, err
	}
	n := New(id)
	n.PublicKey = append(crypto.PublicKey(nil), pub...)
	n.PrivateKey = append(crypto.PrivateKey(nil), priv...)
	if cfg != (price.PriceConfig{}) {
		n.PriceConfig = cfg
	}
	return n, nil
}

func NewNodeWithStore(priv crypto.PrivateKey, cfg price.PriceConfig, store Store) (*Node, error) {
	n, err := NewNode(priv, cfg)
	if err != nil {
		return nil, err
	}
	n.Store = store
	if store == nil {
		return n, nil
	}
	if inv, err := store.LoadInventory(n.ID); err == nil {
		n.Inventory = inv
	} else {
		return nil, err
	}
	if flow, err := store.LoadFlow(n.ID); err == nil {
		n.Flow = flow
	} else {
		return nil, err
	}
	if prices, err := store.LoadPriceState(n.ID); err == nil {
		n.PriceState = prices
	} else {
		return nil, err
	}
	n.init()
	return n, nil
}

func DefaultPriceConfig() price.PriceConfig {
	return price.PriceConfig{
		BasePrice:       1,
		Weights:         price.FeatureWeights{Cost: 1, Age: 0.25, Stability: 0.25},
		VolumeThreshold: model.FromFloat(10),
		DecayK:          0,
	}
}

func (n *Node) init() {
	if n.Inventory.Holdings == nil {
		n.Inventory = model.NewInventoryState()
	}
	if n.Preferences == nil {
		n.Preferences = make(map[model.UnitID]float64)
	}
	if n.PriceState == nil {
		n.PriceState = make(map[model.UnitID]price.PriceResult)
	}
	if n.Features == nil {
		n.Features = make(map[model.UnitID]price.AssetFeatures)
	}
	if n.TradeHistory == nil {
		n.TradeHistory = make(map[model.UnitID][]price.TradeObservation)
	}
	if n.SettledVolume == nil {
		n.SettledVolume = make(map[model.UnitID]model.Amount)
	}
	if n.LastTradeUnix == nil {
		n.LastTradeUnix = make(map[model.UnitID]int64)
	}
	if n.Flow == nil {
		n.Flow = make(map[model.UnitID]model.FlowRecord)
	}
	if n.ExecutedTrades == nil {
		n.ExecutedTrades = make(map[model.TxID]bool)
	}
	if n.NowUnix == nil {
		n.NowUnix = func() int64 { return time.Now().Unix() }
	}
	if n.PriceConfig.BasePrice == 0 && n.PriceConfig.VolumeThreshold == 0 && n.PriceConfig.DecayK == 0 {
		n.PriceConfig = DefaultPriceConfig()
	}
}

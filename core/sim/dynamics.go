package sim

import (
	"fmt"
	"math"

	"web4-v3/core/model"
)

type AcceptanceState struct {
	Scores map[string]float64
}

type DynamicsConfig struct {
	Alpha float64
	Steps int
	Tau   float64
}

type LocalDynamicsConfig struct {
	Alpha       float64
	Steps       int
	Tau         float64
	IncludeSelf bool
}

func NewDynamicsConfig(alpha float64, steps int, tau float64) (DynamicsConfig, error) {
	cfg := DynamicsConfig{Alpha: alpha, Steps: steps, Tau: tau}
	if err := validateDynamicsConfig(cfg); err != nil {
		return DynamicsConfig{}, err
	}

	return cfg, nil
}

func NewLocalDynamicsConfig(alpha float64, steps int, tau float64, includeSelf bool) (LocalDynamicsConfig, error) {
	cfg := LocalDynamicsConfig{Alpha: alpha, Steps: steps, Tau: tau, IncludeSelf: includeSelf}
	if err := validateDynamicsConfig(DynamicsConfig{Alpha: alpha, Steps: steps, Tau: tau}); err != nil {
		return LocalDynamicsConfig{}, err
	}

	return cfg, nil
}

func InitialAcceptanceState(net *SimNetwork, tx *model.TransferTx, inputs []model.Value) AcceptanceState {
	state := AcceptanceState{Scores: map[string]float64{}}
	if net == nil {
		return state
	}

	for _, node := range net.Nodes {
		result := node.EvaluateTransfer(tx, inputs)
		state.Scores[node.ID] = clamp01(result.Score)
	}

	return state
}

func StepAcceptance(state AcceptanceState, tau float64, alpha float64) AcceptanceState {
	next := AcceptanceState{Scores: make(map[string]float64, len(state.Scores))}
	m := BinaryAcceptanceRatio(state, tau)

	for nodeID, score := range state.Scores {
		next.Scores[nodeID] = clamp01(score + alpha*(m-score))
	}

	return next
}

func RunDynamics(initial AcceptanceState, cfg DynamicsConfig) []AcceptanceState {
	states := make([]AcceptanceState, 0, cfg.Steps+1)
	states = append(states, copyState(initial))

	current := initial
	for i := 0; i < cfg.Steps; i++ {
		current = StepAcceptance(current, cfg.Tau, cfg.Alpha)
		states = append(states, current)
	}

	return states
}

func LocalAcceptanceRatio(state AcceptanceState, topology Topology, nodeID string, tau float64, includeSelf bool) float64 {
	neighborhood := topology.Neighborhood(nodeID, includeSelf)
	if len(neighborhood) == 0 {
		return 0
	}

	accepts := 0
	for _, neighborID := range neighborhood {
		if state.Scores[neighborID] >= tau {
			accepts++
		}
	}

	return float64(accepts) / float64(len(neighborhood))
}

func StepLocalAcceptance(state AcceptanceState, topology Topology, cfg LocalDynamicsConfig) AcceptanceState {
	next := AcceptanceState{Scores: make(map[string]float64, len(state.Scores))}
	for _, nodeID := range stateNodeIDs(state) {
		score := state.Scores[nodeID]
		m := LocalAcceptanceRatio(state, topology, nodeID, cfg.Tau, cfg.IncludeSelf)
		next.Scores[nodeID] = clamp01(score + cfg.Alpha*(m-score))
	}

	return next
}

func RunLocalDynamics(initial AcceptanceState, topology Topology, cfg LocalDynamicsConfig) []AcceptanceState {
	states := make([]AcceptanceState, 0, cfg.Steps+1)
	states = append(states, copyState(initial))

	current := initial
	for i := 0; i < cfg.Steps; i++ {
		current = StepLocalAcceptance(current, topology, cfg)
		states = append(states, current)
	}

	return states
}

func AcceptanceMean(state AcceptanceState) float64 {
	if len(state.Scores) == 0 {
		return 0
	}

	sum := 0.0
	for _, score := range state.Scores {
		sum += score
	}

	return sum / float64(len(state.Scores))
}

func BinaryAcceptanceRatio(state AcceptanceState, tau float64) float64 {
	if len(state.Scores) == 0 {
		return 0
	}

	accepts := 0
	for _, score := range state.Scores {
		if score >= tau {
			accepts++
		}
	}

	return float64(accepts) / float64(len(state.Scores))
}

func HasConverged(state AcceptanceState, epsilon float64) bool {
	if len(state.Scores) <= 1 {
		return true
	}

	minScore := math.Inf(1)
	maxScore := math.Inf(-1)
	for _, score := range state.Scores {
		if score < minScore {
			minScore = score
		}
		if score > maxScore {
			maxScore = score
		}
	}

	return maxScore-minScore <= epsilon
}

func validateDynamicsConfig(cfg DynamicsConfig) error {
	if cfg.Alpha < 0 || cfg.Alpha > 1 {
		return fmt.Errorf("alpha must be in [0,1]")
	}
	if cfg.Steps < 0 {
		return fmt.Errorf("steps must be >= 0")
	}
	if cfg.Tau < 0 || cfg.Tau > 1 {
		return fmt.Errorf("tau must be in [0,1]")
	}

	return nil
}

func copyState(state AcceptanceState) AcceptanceState {
	copied := AcceptanceState{Scores: make(map[string]float64, len(state.Scores))}
	for nodeID, score := range state.Scores {
		copied.Scores[nodeID] = score
	}

	return copied
}

func stateNodeIDs(state AcceptanceState) []string {
	ids := make([]string, 0, len(state.Scores))
	for nodeID := range state.Scores {
		ids = append(ids, nodeID)
	}

	return sortedUnique(ids)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}

	return v
}

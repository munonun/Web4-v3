package sim

import (
	"web4-v3/core/model"
	"web4-v3/core/policy"
)

type SurvivalResult struct {
	Ratio       float64
	AcceptCount int
	RejectCount int
	Decisions   []policy.Decision
}

func (net *SimNetwork) AcceptanceVector(tx *model.TransferTx, inputs []model.Value) []float64 {
	result := net.Evaluate(tx, inputs)
	vector := make([]float64, len(result.Decisions))
	for i, decision := range result.Decisions {
		if decision == policy.Accept {
			vector[i] = 1
		}
	}

	return vector
}

func (net *SimNetwork) AcceptanceRatio(tx *model.TransferTx, inputs []model.Value) float64 {
	return net.Evaluate(tx, inputs).Ratio
}

func (net *SimNetwork) Survives(tx *model.TransferTx, inputs []model.Value, tau float64) bool {
	return net.AcceptanceRatio(tx, inputs) >= tau
}

func (net *SimNetwork) Evaluate(tx *model.TransferTx, inputs []model.Value) SurvivalResult {
	if net == nil || len(net.Nodes) == 0 {
		return SurvivalResult{}
	}

	result := SurvivalResult{Decisions: make([]policy.Decision, len(net.Nodes))}
	for i, node := range net.Nodes {
		decision := node.EvaluateTransfer(tx, inputs).Decision
		result.Decisions[i] = decision
		if decision == policy.Accept {
			result.AcceptCount++
		} else {
			result.RejectCount++
		}
	}
	result.Ratio = float64(result.AcceptCount) / float64(len(net.Nodes))

	return result
}

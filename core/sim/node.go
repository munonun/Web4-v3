package sim

import (
	"web4-v3/core/model"
	"web4-v3/core/policy"
)

type SimNode struct {
	ID     string
	Policy *policy.Policy
}

func (n *SimNode) EvaluateIssue(tx *model.IssueTx, output model.Value) policy.AcceptanceResult {
	return n.Policy.EvaluateIssue(tx, output)
}

func (n *SimNode) EvaluateTransfer(tx *model.TransferTx, inputs []model.Value) policy.AcceptanceResult {
	return n.Policy.EvaluateTransfer(tx, inputs)
}

func Trade(a, b *SimNode, tx *model.TransferTx, inputs []model.Value) bool {
	return a.EvaluateTransfer(tx, inputs).Decision == policy.Accept && b.EvaluateTransfer(tx, inputs).Decision == policy.Accept
}

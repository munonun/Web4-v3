package policy

import (
	"encoding/hex"
	"fmt"
	"time"

	"web4-v3/core/crypto"
	"web4-v3/core/model"
)

type Policy struct {
	TrustedIssuers map[string]float64
	MinScore       float64
	MaxDepth       uint32
	NowUnix        func() int64
}

// EvaluateIssue validates an issuance, then applies this node's local trust policy.
// Unknown issuers receive score 0 and are rejected when below MinScore.
func (p *Policy) EvaluateIssue(tx *model.IssueTx, output model.Value) AcceptanceResult {
	if err := model.ValidateIssueTx(tx, output); err != nil {
		return reject(0, "validation failed: "+err.Error())
	}
	if p.isExpired(output.ExpiryUnix) {
		return reject(0, "value expired")
	}
	if output.Depth > p.MaxDepth {
		return reject(0, fmt.Sprintf("depth %d exceeds max depth %d", output.Depth, p.MaxDepth))
	}

	score, trusted := p.issuerTrust(output.Issuer)
	if !trusted {
		return reject(score, "issuer is not trusted")
	}
	if score < p.MinScore {
		return reject(score, fmt.Sprintf("issuer trust %.3f below minimum %.3f", score, p.MinScore))
	}

	return AcceptanceResult{Decision: Accept, Score: score, Reasons: []string{"issuer trusted", "issue accepted"}}
}

// EvaluateTransfer validates a transfer, then applies this node's local trust policy.
// The transfer score is the lowest trusted issuer score among the input values.
func (p *Policy) EvaluateTransfer(tx *model.TransferTx, inputs []model.Value) AcceptanceResult {
	if err := model.ValidateTransferTx(tx, inputs); err != nil {
		return reject(0, "validation failed: "+err.Error())
	}

	scoreSet := false
	score := 0.0
	for _, input := range inputs {
		if p.isExpired(input.ExpiryUnix) {
			return reject(0, "input value expired")
		}
		if input.Depth > p.MaxDepth {
			return reject(0, fmt.Sprintf("input depth %d exceeds max depth %d", input.Depth, p.MaxDepth))
		}

		trust, trusted := p.issuerTrust(input.Issuer)
		if !trusted {
			return reject(0, "input issuer is not trusted")
		}
		if !scoreSet || trust < score {
			score = trust
			scoreSet = true
		}
	}

	for _, output := range tx.Outputs {
		if p.isExpired(output.ExpiryUnix) {
			return reject(0, "output value expired")
		}
		if output.Depth > p.MaxDepth {
			return reject(0, fmt.Sprintf("output depth %d exceeds max depth %d", output.Depth, p.MaxDepth))
		}
	}

	if score < p.MinScore {
		return reject(score, fmt.Sprintf("issuer trust %.3f below minimum %.3f", score, p.MinScore))
	}

	return AcceptanceResult{Decision: Accept, Score: score, Reasons: []string{"input issuers trusted", "transfer accepted"}}
}

func IssuerKey(pub crypto.PublicKey) string {
	return hex.EncodeToString(pub)
}

func (p *Policy) issuerTrust(issuer crypto.PublicKey) (float64, bool) {
	if p == nil || p.TrustedIssuers == nil {
		return 0, false
	}
	score, ok := p.TrustedIssuers[IssuerKey(issuer)]
	return score, ok
}

func (p *Policy) isExpired(expiryUnix int64) bool {
	if expiryUnix == 0 {
		return false
	}

	now := time.Now().Unix()
	if p != nil && p.NowUnix != nil {
		now = p.NowUnix()
	}

	return expiryUnix <= now
}

func reject(score float64, reason string) AcceptanceResult {
	return AcceptanceResult{Decision: Reject, Score: score, Reasons: []string{reason}}
}

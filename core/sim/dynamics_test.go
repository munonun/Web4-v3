package sim

import (
	"math"
	"testing"

	"web4-v3/core/policy"
)

func TestStepAcceptanceUpwardFeedback(t *testing.T) {
	state := AcceptanceState{Scores: map[string]float64{"a": 1.0, "b": 1.0, "c": 0.0}}
	next := StepAcceptance(state, 0.5, 0.5)

	assertApprox(t, next.Scores["a"], 5.0/6.0)
	assertApprox(t, next.Scores["b"], 5.0/6.0)
	assertApprox(t, next.Scores["c"], 1.0/3.0)
}

func TestStepAcceptanceCollapse(t *testing.T) {
	state := AcceptanceState{Scores: map[string]float64{"a": 0.4, "b": 0.3, "c": 0.2}}
	next := StepAcceptance(state, 0.5, 0.5)

	assertApprox(t, next.Scores["a"], 0.2)
	assertApprox(t, next.Scores["b"], 0.15)
	assertApprox(t, next.Scores["c"], 0.1)
}

func TestStepAcceptanceFullAcceptanceStable(t *testing.T) {
	state := AcceptanceState{Scores: map[string]float64{"a": 1.0, "b": 1.0, "c": 1.0}}
	next := StepAcceptance(state, 0.5, 0.5)

	assertApprox(t, next.Scores["a"], 1.0)
	assertApprox(t, next.Scores["b"], 1.0)
	assertApprox(t, next.Scores["c"], 1.0)
}

func TestStepAcceptanceDoesNotMutateInput(t *testing.T) {
	state := AcceptanceState{Scores: map[string]float64{"a": 1.0, "b": 0.0}}
	_ = StepAcceptance(state, 0.5, 0.5)

	assertApprox(t, state.Scores["a"], 1.0)
	assertApprox(t, state.Scores["b"], 0.0)
}

func TestRunDynamicsLength(t *testing.T) {
	cfg, err := NewDynamicsConfig(0.5, 5, 0.5)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	states := RunDynamics(AcceptanceState{Scores: map[string]float64{"a": 1}}, cfg)

	if len(states) != 6 {
		t.Fatalf("states length %d, want 6", len(states))
	}
}

func TestNewDynamicsConfigRejectsInvalidValues(t *testing.T) {
	if _, err := NewDynamicsConfig(-0.1, 1, 0.5); err == nil {
		t.Fatal("expected invalid alpha error")
	}
	if _, err := NewDynamicsConfig(0.5, -1, 0.5); err == nil {
		t.Fatal("expected invalid steps error")
	}
	if _, err := NewDynamicsConfig(0.5, 1, 1.1); err == nil {
		t.Fatal("expected invalid tau error")
	}
}

func TestLocalAcceptanceRatioCanDifferFromGlobal(t *testing.T) {
	state := AcceptanceState{Scores: map[string]float64{"A": 1, "B": 1, "C": 0, "D": 0}}
	topology := ClusteredTopology([][]string{{"A", "B"}, {"C", "D"}})

	assertApprox(t, BinaryAcceptanceRatio(state, 0.5), 0.5)
	assertApprox(t, LocalAcceptanceRatio(state, topology, "A", 0.5, true), 1.0)
	assertApprox(t, LocalAcceptanceRatio(state, topology, "C", 0.5, true), 0.0)
}

func TestLocalAcceptanceRatioIncludeSelfChangesRatio(t *testing.T) {
	state := AcceptanceState{Scores: map[string]float64{"A": 1, "B": 0}}
	topology := NewTopology(map[string][]string{"A": {"B"}})

	assertApprox(t, LocalAcceptanceRatio(state, topology, "A", 0.5, false), 0.0)
	assertApprox(t, LocalAcceptanceRatio(state, topology, "A", 0.5, true), 0.5)
}

func TestLocalAcceptanceRatioUnknownNode(t *testing.T) {
	state := AcceptanceState{Scores: map[string]float64{"A": 1}}
	topology := NewTopology(nil)

	assertApprox(t, LocalAcceptanceRatio(state, topology, "Z", 0.5, false), 0.0)
	assertApprox(t, LocalAcceptanceRatio(state, topology, "Z", 0.5, true), 0.0)
}

func TestSplitGlobalDynamicsConvergesTowardGlobalMarket(t *testing.T) {
	state := AcceptanceState{Scores: map[string]float64{"A": 1, "B": 0, "C": 1, "D": 0}}
	cfg, err := NewDynamicsConfig(0.5, 4, 0.5)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	states := RunDynamics(state, cfg)
	final := states[len(states)-1]

	if !HasConverged(final, 0.1) {
		t.Fatalf("expected global dynamics to converge, got %#v", final.Scores)
	}
}

func TestSplitClusteredLocalDynamicsPreservesDivergence(t *testing.T) {
	state := AcceptanceState{Scores: map[string]float64{"A": 1, "B": 0, "C": 1, "D": 0}}
	topology := ClusteredTopology([][]string{{"A", "C"}, {"B", "D"}})
	cfg, err := NewLocalDynamicsConfig(0.5, 4, 0.5, true)
	if err != nil {
		t.Fatalf("new local config: %v", err)
	}
	states := RunLocalDynamics(state, topology, cfg)
	final := states[len(states)-1]

	if HasConverged(final, 0.1) {
		t.Fatalf("expected clustered local dynamics to preserve divergence, got %#v", final.Scores)
	}
	if final.Scores["A"] < 0.9 || final.Scores["C"] < 0.9 || final.Scores["B"] > 0.1 || final.Scores["D"] > 0.1 {
		t.Fatalf("unexpected final clustered scores: %#v", final.Scores)
	}
}

func TestStepLocalAcceptanceDoesNotMutateInput(t *testing.T) {
	state := AcceptanceState{Scores: map[string]float64{"A": 1, "B": 0}}
	topology := FullMeshTopology([]string{"A", "B"})
	cfg, err := NewLocalDynamicsConfig(0.5, 1, 0.5, true)
	if err != nil {
		t.Fatalf("new local config: %v", err)
	}
	_ = StepLocalAcceptance(state, topology, cfg)

	assertApprox(t, state.Scores["A"], 1.0)
	assertApprox(t, state.Scores["B"], 0.0)
}

func TestRunLocalDynamicsLength(t *testing.T) {
	cfg, err := NewLocalDynamicsConfig(0.5, 5, 0.5, true)
	if err != nil {
		t.Fatalf("new local config: %v", err)
	}
	states := RunLocalDynamics(AcceptanceState{Scores: map[string]float64{"A": 1}}, NewTopology(nil), cfg)

	if len(states) != 6 {
		t.Fatalf("states length %d, want 6", len(states))
	}
}

func TestNewLocalDynamicsConfigRejectsInvalidValues(t *testing.T) {
	if _, err := NewLocalDynamicsConfig(-0.1, 1, 0.5, true); err == nil {
		t.Fatal("expected invalid alpha error")
	}
	if _, err := NewLocalDynamicsConfig(0.5, -1, 0.5, true); err == nil {
		t.Fatal("expected invalid steps error")
	}
	if _, err := NewLocalDynamicsConfig(0.5, 1, 1.1, true); err == nil {
		t.Fatal("expected invalid tau error")
	}
}

func TestAcceptanceMetrics(t *testing.T) {
	state := AcceptanceState{Scores: map[string]float64{"a": 0.8, "b": 0.6, "c": 0.1}}

	assertApprox(t, AcceptanceMean(state), 0.5)
	assertApprox(t, BinaryAcceptanceRatio(state, 0.5), 2.0/3.0)
	if HasConverged(state, 0.1) {
		t.Fatal("expected state not to be converged")
	}
	if !HasConverged(AcceptanceState{Scores: map[string]float64{"a": 0.5, "b": 0.55}}, 0.1) {
		t.Fatal("expected state to be converged")
	}
}

func TestInitialAcceptanceStateUsesPolicyScores(t *testing.T) {
	issuer, tx, inputs := mustSimTransfer(t)
	net := SimNetwork{Nodes: []*SimNode{
		{
			ID: "high",
			Policy: &policy.Policy{
				TrustedIssuers: map[string]float64{policy.IssuerKey(issuer): 0.8},
				MinScore:       0.5,
				MaxDepth:       10,
				NowUnix:        func() int64 { return 100 },
			},
		},
		untrustedNode("zero"),
	}}

	state := InitialAcceptanceState(&net, tx, inputs)
	assertApprox(t, state.Scores["high"], 0.8)
	assertApprox(t, state.Scores["zero"], 0.0)
}

func assertApprox(t *testing.T, got float64, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.000001 {
		t.Fatalf("got %f, want %f", got, want)
	}
}

package cli

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIPartialScenarioRuns(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"sim", "acceptance", "--scenario", "partial", "--alpha", "0.5", "--tau", "0.5", "--steps", "1"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "scenario: partial") || !strings.Contains(stdout.String(), "step 1:") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestCLICollapseScenarioRuns(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"sim", "acceptance", "--scenario", "collapse", "--steps", "1"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "scenario: collapse") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestCLIInvalidAlphaFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"sim", "acceptance", "--alpha", "1.1"}, &stdout, &stderr)

	if code == 0 {
		t.Fatal("expected failure")
	}
	if !strings.Contains(stderr.String(), "alpha") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestCLIInvalidTauFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"sim", "acceptance", "--tau", "-0.1"}, &stdout, &stderr)

	if code == 0 {
		t.Fatal("expected failure")
	}
	if !strings.Contains(stderr.String(), "tau") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestCLIInvalidScenarioFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"sim", "acceptance", "--scenario", "missing"}, &stdout, &stderr)

	if code == 0 {
		t.Fatal("expected failure")
	}
	if !strings.Contains(stderr.String(), "unknown scenario") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestCLIJSONOutputIsValid(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"sim", "acceptance", "--scenario", "partial", "--steps", "1", "--json"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, stderr.String())
	}

	var out jsonOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json unmarshal: %v\n%s", err, stdout.String())
	}
	if out.Scenario != "partial" || len(out.States) != 2 {
		t.Fatalf("unexpected json: %+v", out)
	}
}

func TestCLIGlobalTopologyWorks(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"sim", "acceptance", "--scenario", "split", "--topology", "global", "--steps", "1"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "topology: global") || strings.Contains(stdout.String(), "local M:") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestCLIClusteredTopologyWorks(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"sim", "acceptance", "--scenario", "split", "--topology", "clustered", "--steps", "1"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "topology: clustered") || !strings.Contains(stdout.String(), "local M:") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestCLIInvalidTopologyFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"sim", "acceptance", "--topology", "missing"}, &stdout, &stderr)

	if code == 0 {
		t.Fatal("expected failure")
	}
	if !strings.Contains(stderr.String(), "unknown topology") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestScenarioStatePresets(t *testing.T) {
	state, err := ScenarioState("split")
	if err != nil {
		t.Fatalf("scenario state: %v", err)
	}
	if len(state.Scores) != 4 || state.Scores["A"] != 1 || state.Scores["B"] != 0 {
		t.Fatalf("unexpected split state: %+v", state.Scores)
	}
}

func TestCLIMarketRuns(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"sim", "market", "--scenario", "split", "--topology", "full", "--steps", "10"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Web4 market simulation") || !strings.Contains(stdout.String(), "total_trades:") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestCLIMarketCSVOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	path := filepath.Join(t.TempDir(), "market.csv")
	code := Run([]string{"sim", "market", "--steps", "3", "--csv", path}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, stderr.String())
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open csv: %v", err)
	}
	defer file.Close()

	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	if len(rows) != 5 {
		t.Fatalf("row count %d, want 5", len(rows))
	}
	wantHeader := "step,mean_price,min_price,max_price,price_spread,price_variance,executed_trades,demand_trades,arbitrage_trades,total_volume,produced_volume,consumed_volume,available_arbitrage_count,max_arbitrage_profit,liquidity_score,global_mean_acceptance,unmet_demand,total_surplus"
	if strings.Join(rows[0], ",") != wantHeader {
		t.Fatalf("header %q, want %q", strings.Join(rows[0], ","), wantHeader)
	}
}

func TestCLIMarketInvalidOptionsFail(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"sim", "market", "--alpha", "1.1"}, &stdout, &stderr)

	if code == 0 {
		t.Fatal("expected failure")
	}
	if !strings.Contains(stderr.String(), "alpha") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestCLIMarketJSONOmitsStepsByDefault(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"sim", "market", "--steps", "2", "--json"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, stderr.String())
	}

	var out marketJSONOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json unmarshal: %v\n%s", err, stdout.String())
	}
	if out.Summary.InitialPriceSpread == 0 {
		t.Fatalf("unexpected summary: %+v", out.Summary)
	}
	if len(out.Metrics) != 0 {
		t.Fatalf("expected compact json by default, got %d metrics", len(out.Metrics))
	}
}

func TestCLIMultiAssetMarketRuns(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"sim", "market", "--scenario", "multi-basic", "--multi-asset", "--assets", "SKUG,WEB4", "--steps", "5", "--enable-demand", "--enable-cycle"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Web4 multi-asset market simulation") || !strings.Contains(stdout.String(), "SKUG_WEB4_rate:") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestCLIMultiAssetCSVOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	path := filepath.Join(t.TempDir(), "multi.csv")
	code := Run([]string{"sim", "market", "--scenario", "multi-basic", "--multi-asset", "--assets", "WEB4,SKUG", "--steps", "2", "--enable-demand", "--enable-cycle", "--csv", path}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, stderr.String())
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open csv: %v", err)
	}
	defer file.Close()

	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("row count %d, want 4", len(rows))
	}
	wantHeader := "step,total_trades,total_volume,dominant_asset,dominant_asset_by_flow,flow_concentration,switch_count,SKUG_mean,SKUG_spread,SKUG_share,SKUG_flow_share,SKUG_trade_flow,SKUG_payment_flow,SKUG_consumption_flow,SKUG_demand_fulfilled,WEB4_mean,WEB4_spread,WEB4_share,WEB4_flow_share,WEB4_trade_flow,WEB4_payment_flow,WEB4_consumption_flow,WEB4_demand_fulfilled,SKUG_WEB4_rate"
	if strings.Join(rows[0], ",") != wantHeader {
		t.Fatalf("header %q, want %q", strings.Join(rows[0], ","), wantHeader)
	}
}

func TestCLIMultiAssetJSONIncludesFlowSummary(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"sim", "market", "--scenario", "multi-basic", "--multi-asset", "--assets", "SKUG,WEB4", "--steps", "2", "--enable-demand", "--enable-cycle", "--json"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, stderr.String())
	}

	var out multiMarketJSONOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json unmarshal: %v\n%s", err, stdout.String())
	}
	if out.Summary.DominantAssetByFlow == "" {
		t.Fatalf("expected dominant flow asset in summary: %+v", out.Summary)
	}
	if out.Summary.TradeFlow["SKUG"] == 0 && out.Summary.TradeFlow["WEB4"] == 0 {
		t.Fatalf("expected trade flow in summary: %+v", out.Summary.TradeFlow)
	}
}

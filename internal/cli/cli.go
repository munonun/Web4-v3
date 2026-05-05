package cli

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"web4-v3/core/sim"
)

const usage = `usage:
  web4 sim acceptance [--scenario partial|unanimous|collapse|split] [--alpha n] [--tau n] [--steps n] [--epsilon n] [--topology global|full|chain|clustered] [--no-self] [--json]
  web4 sim market [--scenario split|clustered|random|collapse|high-liquidity|fragmented|demand-basic|demand-fragmented|demand-random|cycle-basic|cycle-fragmented|cycle-random|multi-basic|multi-compete|multi-fragmented|multi-flight|multi-coexist] [--topology full|chain|clustered] [--steps n] [--alpha n] [--spread n] [--min-profit n] [--seed n] [--trades-per-step n] [--enable-demand] [--max-qty n] [--enable-cycle] [--consumption-rate n] [--production-rate n] [--price-model acceptance|pipeline] [--multi-asset] [--assets SKUG,WEB4] [--enable-substitution] [--utility-mode fixed|random|clustered] [--json] [--json-steps] [--csv path]
`

type acceptanceOptions struct {
	Scenario string
	Alpha    float64
	Tau      float64
	Steps    int
	Epsilon  float64
	Topology string
	NoSelf   bool
	JSON     bool
}

type jsonState struct {
	Step      int                `json:"step"`
	Scores    map[string]float64 `json:"scores"`
	Ratio     float64            `json:"ratio"`
	Local     map[string]float64 `json:"local,omitempty"`
	Mean      float64            `json:"mean"`
	Converged bool               `json:"converged"`
}

type jsonOutput struct {
	Scenario    string      `json:"scenario"`
	Alpha       float64     `json:"alpha"`
	Tau         float64     `json:"tau"`
	Steps       int         `json:"steps"`
	Topology    string      `json:"topology"`
	IncludeSelf bool        `json:"include_self"`
	States      []jsonState `json:"states"`
	FinalRatio  float64     `json:"final_ratio"`
	FinalMean   float64     `json:"final_mean"`
	Survived    bool        `json:"survived"`
	Converged   bool        `json:"converged"`
}

type marketOptions struct {
	Config     sim.MarketConfig
	JSON       bool
	JSONSteps  bool
	CSVPath    string
	MultiAsset bool
	Assets     string
}

type marketJSONOutput struct {
	Scenario        string                  `json:"scenario"`
	Topology        string                  `json:"topology"`
	Steps           int                     `json:"steps"`
	Alpha           float64                 `json:"alpha"`
	Spread          float64                 `json:"spread"`
	MinProfit       float64                 `json:"min_profit"`
	Seed            int64                   `json:"seed"`
	EnableDemand    bool                    `json:"enable_demand"`
	MaxQty          float64                 `json:"max_qty"`
	EnableCycle     bool                    `json:"enable_cycle"`
	ConsumptionRate float64                 `json:"consumption_rate"`
	ProductionRate  float64                 `json:"production_rate"`
	PriceModel      string                  `json:"price_model"`
	Summary         sim.MarketSummary       `json:"summary"`
	Metrics         []sim.MarketStepMetrics `json:"metrics,omitempty"`
}

type multiMarketJSONOutput struct {
	Scenario           string                   `json:"scenario"`
	Topology           string                   `json:"topology"`
	Assets             []string                 `json:"assets"`
	Steps              int                      `json:"steps"`
	Alpha              float64                  `json:"alpha"`
	Spread             float64                  `json:"spread"`
	Seed               int64                    `json:"seed"`
	EnableSubstitution bool                     `json:"enable_substitution"`
	UtilityMode        string                   `json:"utility_mode"`
	PriceModel         string                   `json:"price_model"`
	Summary            sim.MultiMarketSummary   `json:"summary"`
	Metrics            []sim.MultiMarketMetrics `json:"metrics,omitempty"`
}

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) < 2 || args[0] != "sim" {
		fmt.Fprint(stderr, usage)
		return 2
	}

	var err error
	switch args[1] {
	case "acceptance":
		err = runAcceptance(args[2:], stdout)
	case "market":
		err = runMarket(args[2:], stdout)
	default:
		fmt.Fprint(stderr, usage)
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	return 0
}

func runMarket(args []string, stdout io.Writer) error {
	opts, err := parseMarketOptions(args)
	if err != nil {
		return err
	}
	if opts.MultiAsset {
		return runMultiMarket(opts, stdout)
	}

	result, err := sim.RunMarketSimulation(opts.Config)
	if err != nil {
		return err
	}
	if opts.CSVPath != "" {
		if err := writeMarketCSV(opts.CSVPath, result.Metrics); err != nil {
			return err
		}
	}
	if opts.JSON {
		return writeMarketJSON(stdout, result, opts.JSONSteps)
	}

	writeMarketText(stdout, result)
	return nil
}

func runMultiMarket(opts marketOptions, stdout io.Writer) error {
	cfg := sim.DefaultMultiMarketConfig()
	cfg.Scenario = opts.Config.Scenario
	cfg.Topology = opts.Config.Topology
	cfg.Steps = opts.Config.Steps
	cfg.Alpha = opts.Config.Alpha
	cfg.Spread = opts.Config.Spread
	cfg.Seed = opts.Config.Seed
	cfg.TradesPerStep = opts.Config.TradesPerStep
	cfg.EnableDemand = opts.Config.EnableDemand
	cfg.EnableCycle = opts.Config.EnableCycle
	cfg.EnableSubstitution = opts.Config.EnableSubstitution
	cfg.UtilityMode = opts.Config.UtilityMode
	cfg.PriceModel = opts.Config.PriceModel
	cfg.MaxQty = opts.Config.MaxQty
	cfg.Universe = sim.NewAssetUniverse(parseAssetIDs(opts.Assets))

	result, err := sim.RunMultiMarketSimulation(cfg)
	if err != nil {
		return err
	}
	if opts.CSVPath != "" {
		if err := writeMultiMarketCSV(opts.CSVPath, result.Metrics, cfg.Universe); err != nil {
			return err
		}
	}
	if opts.JSON {
		return writeMultiMarketJSON(stdout, result, opts.JSONSteps)
	}
	writeMultiMarketText(stdout, result)
	return nil
}

func parseMarketOptions(args []string) (marketOptions, error) {
	opts := marketOptions{Config: sim.DefaultMarketConfig()}
	fs := flag.NewFlagSet("market", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.Config.Scenario, "scenario", opts.Config.Scenario, "market scenario")
	fs.StringVar(&opts.Config.Topology, "topology", opts.Config.Topology, "topology mode: full, chain, clustered")
	fs.IntVar(&opts.Config.Steps, "steps", opts.Config.Steps, "number of simulation steps")
	fs.Float64Var(&opts.Config.Alpha, "alpha", opts.Config.Alpha, "feedback rate in [0,1]")
	fs.Float64Var(&opts.Config.Spread, "spread", opts.Config.Spread, "required trade spread")
	fs.Float64Var(&opts.Config.MinProfit, "min-profit", opts.Config.MinProfit, "minimum arbitrage profit")
	fs.Int64Var(&opts.Config.Seed, "seed", opts.Config.Seed, "deterministic random seed")
	fs.IntVar(&opts.Config.TradesPerStep, "trades-per-step", opts.Config.TradesPerStep, "max trades per step; 0 means node count")
	fs.BoolVar(&opts.Config.EnableDemand, "enable-demand", false, "enable inventory and demand-driven trades")
	fs.Float64Var(&opts.Config.MaxQty, "max-qty", opts.Config.MaxQty, "maximum quantity per demand trade")
	fs.BoolVar(&opts.Config.EnableCycle, "enable-cycle", false, "enable recurring consumption and simulation-only production")
	fs.Float64Var(&opts.Config.ConsumptionRate, "consumption-rate", opts.Config.ConsumptionRate, "quantity consumed per consumer per step")
	fs.Float64Var(&opts.Config.ProductionRate, "production-rate", opts.Config.ProductionRate, "quantity produced per producer per step")
	fs.StringVar(&opts.Config.PriceModel, "price-model", opts.Config.PriceModel, "price model: acceptance, pipeline")
	fs.BoolVar(&opts.MultiAsset, "multi-asset", false, "run multi-asset market simulation")
	fs.StringVar(&opts.Assets, "assets", "SKUG,WEB4", "comma-separated asset IDs for multi-asset simulation")
	fs.BoolVar(&opts.Config.EnableSubstitution, "enable-substitution", false, "enable portfolio preferences and substitution utility")
	fs.StringVar(&opts.Config.UtilityMode, "utility-mode", "fixed", "portfolio utility mode: fixed, random, clustered")
	fs.BoolVar(&opts.JSON, "json", false, "write JSON summary")
	fs.BoolVar(&opts.JSONSteps, "json-steps", false, "include per-step metrics in JSON")
	fs.StringVar(&opts.CSVPath, "csv", "", "write per-step metrics to CSV")

	if err := fs.Parse(args); err != nil {
		return marketOptions{}, err
	}
	if fs.NArg() != 0 {
		return marketOptions{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	if opts.MultiAsset {
		cfg := sim.DefaultMultiMarketConfig()
		cfg.Scenario = opts.Config.Scenario
		cfg.Topology = opts.Config.Topology
		cfg.Steps = opts.Config.Steps
		cfg.Alpha = opts.Config.Alpha
		cfg.Spread = opts.Config.Spread
		cfg.TradesPerStep = opts.Config.TradesPerStep
		cfg.Seed = opts.Config.Seed
		cfg.EnableDemand = opts.Config.EnableDemand
		cfg.EnableCycle = opts.Config.EnableCycle
		cfg.EnableSubstitution = opts.Config.EnableSubstitution
		cfg.UtilityMode = opts.Config.UtilityMode
		cfg.PriceModel = opts.Config.PriceModel
		cfg.MaxQty = opts.Config.MaxQty
		cfg.Universe = sim.NewAssetUniverse(parseAssetIDs(opts.Assets))
		if err := sim.ValidateMultiMarketConfig(cfg); err != nil {
			return marketOptions{}, err
		}
	} else {
		if err := sim.ValidateMarketConfig(opts.Config); err != nil {
			return marketOptions{}, err
		}
	}

	return opts, nil
}

func runAcceptance(args []string, stdout io.Writer) error {
	opts, err := parseAcceptanceOptions(args)
	if err != nil {
		return err
	}

	initial, err := ScenarioState(opts.Scenario)
	if err != nil {
		return err
	}
	states, topology, err := runScenarioDynamics(initial, opts)
	if err != nil {
		return err
	}

	if opts.JSON {
		return writeJSON(stdout, opts, states, topology)
	}

	writeText(stdout, opts, states, topology)
	return nil
}

func parseAcceptanceOptions(args []string) (acceptanceOptions, error) {
	opts := acceptanceOptions{Scenario: "partial", Alpha: 0.5, Tau: 0.5, Steps: 10, Epsilon: 0.01, Topology: "global"}
	fs := flag.NewFlagSet("acceptance", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.Scenario, "scenario", opts.Scenario, "scenario preset")
	fs.Float64Var(&opts.Alpha, "alpha", opts.Alpha, "feedback rate in [0,1]")
	fs.Float64Var(&opts.Tau, "tau", opts.Tau, "acceptance threshold in [0,1]")
	fs.IntVar(&opts.Steps, "steps", opts.Steps, "number of dynamics steps")
	fs.Float64Var(&opts.Epsilon, "epsilon", opts.Epsilon, "convergence epsilon")
	fs.StringVar(&opts.Topology, "topology", opts.Topology, "topology mode: global, full, chain, clustered")
	fs.BoolVar(&opts.NoSelf, "no-self", false, "exclude self from local topology observations")
	fs.BoolVar(&opts.JSON, "json", false, "write JSON output")

	if err := fs.Parse(args); err != nil {
		return acceptanceOptions{}, err
	}
	if fs.NArg() != 0 {
		return acceptanceOptions{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if opts.Epsilon <= 0 {
		return acceptanceOptions{}, fmt.Errorf("epsilon must be > 0")
	}
	if _, err := sim.NewDynamicsConfig(opts.Alpha, opts.Steps, opts.Tau); err != nil {
		return acceptanceOptions{}, err
	}
	if opts.Topology != "global" && opts.Topology != "full" && opts.Topology != "chain" && opts.Topology != "clustered" {
		return acceptanceOptions{}, fmt.Errorf("unknown topology %q", opts.Topology)
	}

	return opts, nil
}

func ScenarioState(name string) (sim.AcceptanceState, error) {
	scores := map[string]float64{}
	switch name {
	case "partial":
		scores = map[string]float64{"A": 1.0, "B": 1.0, "C": 0.0}
	case "unanimous":
		scores = map[string]float64{"A": 1.0, "B": 1.0, "C": 1.0}
	case "collapse":
		scores = map[string]float64{"A": 0.4, "B": 0.3, "C": 0.2}
	case "split":
		scores = map[string]float64{"A": 1.0, "B": 0.0, "C": 1.0, "D": 0.0}
	default:
		return sim.AcceptanceState{}, fmt.Errorf("unknown scenario %q", name)
	}

	return sim.AcceptanceState{Scores: scores}, nil
}

func runScenarioDynamics(initial sim.AcceptanceState, opts acceptanceOptions) ([]sim.AcceptanceState, sim.Topology, error) {
	if opts.Topology == "global" {
		cfg, err := sim.NewDynamicsConfig(opts.Alpha, opts.Steps, opts.Tau)
		if err != nil {
			return nil, sim.Topology{}, err
		}
		return sim.RunDynamics(initial, cfg), sim.Topology{}, nil
	}

	cfg, err := sim.NewLocalDynamicsConfig(opts.Alpha, opts.Steps, opts.Tau, !opts.NoSelf)
	if err != nil {
		return nil, sim.Topology{}, err
	}
	topology := ScenarioTopology(opts.Scenario, opts.Topology, initial)
	return sim.RunLocalDynamics(initial, topology, cfg), topology, nil
}

func ScenarioTopology(scenario string, topologyName string, state sim.AcceptanceState) sim.Topology {
	nodeIDs := stateNodeIDs(state)
	switch topologyName {
	case "full":
		return sim.FullMeshTopology(nodeIDs)
	case "chain":
		return sim.ChainTopology(nodeIDs)
	case "clustered":
		return sim.ClusteredTopology(scenarioClusters(scenario, nodeIDs))
	default:
		return sim.Topology{}
	}
}

func writeText(w io.Writer, opts acceptanceOptions, states []sim.AcceptanceState, topology sim.Topology) {
	fmt.Fprintln(w, "Web4 acceptance simulation")
	fmt.Fprintf(w, "scenario: %s\n", opts.Scenario)
	fmt.Fprintf(w, "alpha: %.2f\n", opts.Alpha)
	fmt.Fprintf(w, "tau: %.2f\n", opts.Tau)
	fmt.Fprintf(w, "topology: %s\n", opts.Topology)
	if opts.Topology != "global" {
		fmt.Fprintf(w, "include_self: %t\n", !opts.NoSelf)
	}
	fmt.Fprintf(w, "steps: %d\n\n", opts.Steps)

	for i, state := range states {
		fmt.Fprintf(w, "step %d:\n", i)
		fmt.Fprintf(w, "scores: %s\n", formatScores(state))
		fmt.Fprintf(w, "global M: %.2f\n", sim.BinaryAcceptanceRatio(state, opts.Tau))
		if opts.Topology != "global" {
			fmt.Fprintf(w, "local M: %s\n", formatLocalRatios(state, topology, opts))
		}
		fmt.Fprintf(w, "mean: %.2f\n", sim.AcceptanceMean(state))
		fmt.Fprintf(w, "converged: %t\n\n", sim.HasConverged(state, opts.Epsilon))
	}

	final := states[len(states)-1]
	finalRatio := sim.BinaryAcceptanceRatio(final, opts.Tau)
	fmt.Fprintln(w, "final:")
	fmt.Fprintf(w, "global M: %.2f\n", finalRatio)
	fmt.Fprintf(w, "mean: %.2f\n", sim.AcceptanceMean(final))
	fmt.Fprintf(w, "survived: %t\n", finalRatio >= opts.Tau)
	fmt.Fprintf(w, "converged: %t\n", sim.HasConverged(final, opts.Epsilon))
}

func writeJSON(w io.Writer, opts acceptanceOptions, states []sim.AcceptanceState, topology sim.Topology) error {
	out := jsonOutput{Scenario: opts.Scenario, Alpha: opts.Alpha, Tau: opts.Tau, Steps: opts.Steps, Topology: opts.Topology, IncludeSelf: !opts.NoSelf}
	for i, state := range states {
		entry := jsonState{
			Step:      i,
			Scores:    copyScores(state.Scores),
			Ratio:     sim.BinaryAcceptanceRatio(state, opts.Tau),
			Mean:      sim.AcceptanceMean(state),
			Converged: sim.HasConverged(state, opts.Epsilon),
		}
		if opts.Topology != "global" {
			entry.Local = localRatios(state, topology, opts)
		}
		out.States = append(out.States, entry)
	}
	final := states[len(states)-1]
	out.FinalRatio = sim.BinaryAcceptanceRatio(final, opts.Tau)
	out.FinalMean = sim.AcceptanceMean(final)
	out.Survived = out.FinalRatio >= opts.Tau
	out.Converged = sim.HasConverged(final, opts.Epsilon)

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func writeMarketText(w io.Writer, result sim.MarketResult) {
	cfg := result.Config
	summary := result.Summary
	fmt.Fprintln(w, "Web4 market simulation")
	fmt.Fprintf(w, "scenario: %s\n", cfg.Scenario)
	fmt.Fprintf(w, "topology: %s\n", cfg.Topology)
	fmt.Fprintf(w, "steps: %d\n", cfg.Steps)
	fmt.Fprintf(w, "alpha: %.2f\n", cfg.Alpha)
	fmt.Fprintf(w, "spread: %.2f\n", cfg.Spread)
	fmt.Fprintf(w, "min_profit: %.2f\n", cfg.MinProfit)
	fmt.Fprintf(w, "enable_demand: %t\n", cfg.EnableDemand)
	fmt.Fprintf(w, "max_qty: %.2f\n", cfg.MaxQty)
	fmt.Fprintf(w, "enable_cycle: %t\n", cfg.EnableCycle)
	fmt.Fprintf(w, "consumption_rate: %.2f\n", cfg.ConsumptionRate)
	fmt.Fprintf(w, "production_rate: %.2f\n", cfg.ProductionRate)
	fmt.Fprintf(w, "price_model: %s\n\n", cfg.PriceModel)

	fmt.Fprintln(w, "initial:")
	fmt.Fprintf(w, "mean_price: %.2f\n", summary.InitialMeanPrice)
	fmt.Fprintf(w, "price_spread: %.2f\n\n", summary.InitialPriceSpread)

	fmt.Fprintln(w, "final:")
	fmt.Fprintf(w, "mean_price: %.2f\n", summary.FinalMeanPrice)
	fmt.Fprintf(w, "price_spread: %.2f\n", summary.FinalPriceSpread)
	fmt.Fprintf(w, "total_trades: %d\n", summary.TotalTrades)
	fmt.Fprintf(w, "demand_trades: %d\n", summary.DemandTrades)
	fmt.Fprintf(w, "arbitrage_trades: %d\n", summary.ArbitrageTrades)
	fmt.Fprintf(w, "total_volume: %.2f\n", summary.TotalVolume)
	fmt.Fprintf(w, "total_produced: %.2f\n", summary.TotalProduced)
	fmt.Fprintf(w, "total_consumed: %.2f\n", summary.TotalConsumed)
	fmt.Fprintf(w, "avg_trades_per_step: %.2f\n", summary.AverageTradesPerStep)
	fmt.Fprintf(w, "avg_volume_per_step: %.2f\n", summary.AverageVolumePerStep)
	fmt.Fprintf(w, "avg_unmet_demand: %.2f\n", summary.AverageUnmetDemand)
	fmt.Fprintf(w, "final_unmet_demand: %.2f\n", summary.FinalUnmetDemand)
	fmt.Fprintf(w, "final_total_surplus: %.2f\n", summary.FinalTotalSurplus)
	fmt.Fprintf(w, "final_liquidity_score: %.4f\n", summary.FinalLiquidityScore)
	fmt.Fprintf(w, "price_spread_decreased: %t\n", summary.PriceSpreadDecreased)
	fmt.Fprintf(w, "converged: %t\n", summary.Converged)
	fmt.Fprintf(w, "fragmented: %t\n", summary.Fragmented)
	fmt.Fprintf(w, "collapsed: %t\n", summary.Collapsed)
	fmt.Fprintf(w, "liquid: %t\n", summary.Liquid)
}

func writeMarketJSON(w io.Writer, result sim.MarketResult, includeSteps bool) error {
	out := marketJSONOutput{
		Scenario:        result.Config.Scenario,
		Topology:        result.Config.Topology,
		Steps:           result.Config.Steps,
		Alpha:           result.Config.Alpha,
		Spread:          result.Config.Spread,
		MinProfit:       result.Config.MinProfit,
		Seed:            result.Config.Seed,
		EnableDemand:    result.Config.EnableDemand,
		MaxQty:          result.Config.MaxQty,
		EnableCycle:     result.Config.EnableCycle,
		ConsumptionRate: result.Config.ConsumptionRate,
		ProductionRate:  result.Config.ProductionRate,
		PriceModel:      result.Config.PriceModel,
		Summary:         result.Summary,
	}
	if includeSteps {
		out.Metrics = result.Metrics
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func writeMultiMarketText(w io.Writer, result sim.MultiMarketResult) {
	cfg := result.Config
	summary := result.Summary
	fmt.Fprintln(w, "Web4 multi-asset market simulation")
	fmt.Fprintf(w, "scenario: %s\n", cfg.Scenario)
	fmt.Fprintf(w, "topology: %s\n", cfg.Topology)
	fmt.Fprintf(w, "assets: %s\n", strings.Join(cfg.Universe.IDs(), ","))
	fmt.Fprintf(w, "steps: %d\n", cfg.Steps)
	fmt.Fprintf(w, "alpha: %.2f\n", cfg.Alpha)
	fmt.Fprintf(w, "spread: %.2f\n\n", cfg.Spread)
	fmt.Fprintf(w, "enable_substitution: %t\n", cfg.EnableSubstitution)
	fmt.Fprintf(w, "utility_mode: %s\n\n", cfg.UtilityMode)
	fmt.Fprintf(w, "price_model: %s\n\n", cfg.PriceModel)

	fmt.Fprintln(w, "final:")
	for _, assetID := range cfg.Universe.IDs() {
		fmt.Fprintf(w, "%s_mean: %.2f\n", assetID, summary.FinalAssetMeans[assetID])
		fmt.Fprintf(w, "%s_spread: %.2f\n", assetID, summary.FinalAssetSpreads[assetID])
		fmt.Fprintf(w, "%s_share: %.2f\n", assetID, summary.FinalAssetShares[assetID])
		fmt.Fprintf(w, "%s_flow_share: %.2f\n", assetID, summary.FlowShares[assetID])
		fmt.Fprintf(w, "%s_trade_flow: %.2f\n", assetID, summary.TradeFlow[assetID])
		fmt.Fprintf(w, "%s_payment_flow: %.2f\n", assetID, summary.PaymentFlow[assetID])
		fmt.Fprintf(w, "%s_consumption_flow: %.2f\n", assetID, summary.ConsumptionFlow[assetID])
		fmt.Fprintf(w, "%s_demand_fulfilled: %.2f\n", assetID, summary.DemandFulfilled[assetID])
	}
	fmt.Fprintf(w, "dominant_asset: %s\n", summary.DominantAsset)
	fmt.Fprintf(w, "dominant_asset_by_holdings: %s\n", summary.DominantAssetByHoldings)
	fmt.Fprintf(w, "dominant_asset_by_flow: %s\n", summary.DominantAssetByFlow)
	fmt.Fprintf(w, "flow_concentration: %.4f\n", summary.FlowConcentration)
	fmt.Fprintf(w, "switch_count: %d\n", summary.TotalSwitchCount)
	fmt.Fprintf(w, "total_trades: %d\n", summary.TotalTrades)
	fmt.Fprintf(w, "total_cross_asset_trades: %d\n", summary.TotalCrossAssetTrades)
	fmt.Fprintf(w, "total_volume: %.2f\n", summary.TotalVolume)
	fmt.Fprintf(w, "total_trade_flow: %.2f\n", summary.TotalTradeFlow)
	fmt.Fprintf(w, "total_payment_flow: %.2f\n", summary.TotalPaymentFlow)
	fmt.Fprintf(w, "total_consumption_flow: %.2f\n", summary.TotalConsumptionFlow)
	fmt.Fprintf(w, "total_demand_fulfilled: %.2f\n", summary.TotalDemandFulfilled)
	ids := cfg.Universe.IDs()
	if len(ids) >= 2 {
		fmt.Fprintf(w, "%s_%s_rate: %.4f\n", ids[0], ids[1], summary.ExampleExchangeRate)
	}
}

func writeMultiMarketJSON(w io.Writer, result sim.MultiMarketResult, includeSteps bool) error {
	out := multiMarketJSONOutput{
		Scenario:           result.Config.Scenario,
		Topology:           result.Config.Topology,
		Assets:             result.Config.Universe.IDs(),
		Steps:              result.Config.Steps,
		Alpha:              result.Config.Alpha,
		Spread:             result.Config.Spread,
		Seed:               result.Config.Seed,
		EnableSubstitution: result.Config.EnableSubstitution,
		UtilityMode:        result.Config.UtilityMode,
		PriceModel:         result.Config.PriceModel,
		Summary:            result.Summary,
	}
	if includeSteps {
		out.Metrics = result.Metrics
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func writeMultiMarketCSV(path string, metrics []sim.MultiMarketMetrics, universe sim.AssetUniverse) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	assetIDs := universe.IDs()
	header := []string{"step", "total_trades", "total_volume", "dominant_asset", "dominant_asset_by_flow", "flow_concentration", "switch_count"}
	for _, assetID := range assetIDs {
		header = append(header,
			assetID+"_mean",
			assetID+"_spread",
			assetID+"_share",
			assetID+"_flow_share",
			assetID+"_trade_flow",
			assetID+"_payment_flow",
			assetID+"_consumption_flow",
			assetID+"_demand_fulfilled",
		)
	}
	if len(assetIDs) >= 2 {
		header = append(header, assetIDs[0]+"_"+assetIDs[1]+"_rate")
	}

	writer := csv.NewWriter(file)
	if err := writer.Write(header); err != nil {
		return err
	}
	for _, metric := range metrics {
		row := []string{
			strconv.Itoa(metric.Step),
			strconv.Itoa(metric.ExecutedTrades),
			formatFloat(metric.TotalVolume),
			metric.DominantAsset,
			metric.DominantAssetByFlow,
			formatFloat(metric.FlowConcentration),
			strconv.Itoa(metric.SwitchCount),
		}
		for _, assetID := range assetIDs {
			row = append(row,
				formatFloat(metric.AssetMeans[assetID]),
				formatFloat(metric.AssetSpreads[assetID]),
				formatFloat(metric.AssetShares[assetID]),
				formatFloat(metric.FlowShares[assetID]),
				formatFloat(metric.Flows.TradeVolume[assetID]),
				formatFloat(metric.Flows.PaymentVolume[assetID]),
				formatFloat(metric.Flows.ConsumptionVolume[assetID]),
				formatFloat(metric.Flows.DemandFulfilled[assetID]),
			)
		}
		if len(assetIDs) >= 2 {
			row = append(row, formatFloat(metric.ExchangeRates[assetIDs[0]+"/"+assetIDs[1]]))
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

func writeMarketCSV(path string, metrics []sim.MarketStepMetrics) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	header := []string{
		"step",
		"mean_price",
		"min_price",
		"max_price",
		"price_spread",
		"price_variance",
		"executed_trades",
		"demand_trades",
		"arbitrage_trades",
		"total_volume",
		"produced_volume",
		"consumed_volume",
		"available_arbitrage_count",
		"max_arbitrage_profit",
		"liquidity_score",
		"global_mean_acceptance",
		"unmet_demand",
		"total_surplus",
	}
	if err := writer.Write(header); err != nil {
		return err
	}
	for _, metric := range metrics {
		row := []string{
			strconv.Itoa(metric.Step),
			formatFloat(metric.MeanPrice),
			formatFloat(metric.MinPrice),
			formatFloat(metric.MaxPrice),
			formatFloat(metric.PriceSpread),
			formatFloat(metric.PriceVariance),
			strconv.Itoa(metric.ExecutedTrades),
			strconv.Itoa(metric.DemandTrades),
			strconv.Itoa(metric.ArbitrageTrades),
			formatFloat(metric.TotalVolume),
			formatFloat(metric.ProducedVolume),
			formatFloat(metric.ConsumedVolume),
			strconv.Itoa(metric.AvailableArbitrageCount),
			formatFloat(metric.MaxArbitrageProfit),
			formatFloat(metric.LiquidityScore),
			formatFloat(metric.GlobalMeanAcceptance),
			formatFloat(metric.UnmetDemand),
			formatFloat(metric.TotalSurplus),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

func formatLocalRatios(state sim.AcceptanceState, topology sim.Topology, opts acceptanceOptions) string {
	return formatScores(sim.AcceptanceState{Scores: localRatios(state, topology, opts)})
}

func localRatios(state sim.AcceptanceState, topology sim.Topology, opts acceptanceOptions) map[string]float64 {
	ratios := make(map[string]float64, len(state.Scores))
	for _, nodeID := range stateNodeIDs(state) {
		ratios[nodeID] = sim.LocalAcceptanceRatio(state, topology, nodeID, opts.Tau, !opts.NoSelf)
	}

	return ratios
}

func stateNodeIDs(state sim.AcceptanceState) []string {
	keys := make([]string, 0, len(state.Scores))
	for key := range state.Scores {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func scenarioClusters(scenario string, nodeIDs []string) [][]string {
	switch scenario {
	case "partial":
		return [][]string{{"A", "B"}, {"C"}}
	case "split":
		return [][]string{{"A", "C"}, {"B", "D"}}
	default:
		return [][]string{append([]string(nil), nodeIDs...)}
	}
}

func formatScores(state sim.AcceptanceState) string {
	keys := make([]string, 0, len(state.Scores))
	for key := range state.Scores {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := ""
	for i, key := range keys {
		if i > 0 {
			parts += " "
		}
		parts += fmt.Sprintf("%s=%.2f", key, state.Scores[key])
	}

	return parts
}

func copyScores(scores map[string]float64) map[string]float64 {
	copied := make(map[string]float64, len(scores))
	for key, score := range scores {
		copied[key] = score
	}

	return copied
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 6, 64)
}

func parseAssetIDs(raw string) []string {
	parts := strings.Split(raw, ",")
	ids := make([]string, 0, len(parts))
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

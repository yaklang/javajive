package performance_baseline

const (
	SchemaVersion            = 1
	HistoricalMilestoneSHA   = "24b9d377c91ba75f3a277672973782299a958d5f"
	MinValidRepeats          = 10
	WarmupRepeats            = 1
	DefaultRepeats           = 12 // 1 warmup + 11 measured; report ≥10
	MeasuredRepeats          = 11
	ScaleBaseLoopSlotDeep    = 8
	ScaleBaseHandler         = 4
	UncalibratedPercentGates = 0
)

// Honest stage names. Combined / opcode-only / reruns-decompile labels are
// required when an exported split would mean rewriting production algorithms.
const (
	StageParse       = "parse"
	StageDecode      = "decode"
	StageCFG         = "cfg_opcode_only"
	StageSemanticCFG = "semantic_cfg_build"
	StageDataflow    = "dataflow_stacksim_opcode_only"
	StageT25Sparse   = "t25_sparse_reaching"
	StageT26Cache    = "t26_graph_cache"
	StageRegion      = "region_and_statements_combined"
	StageRender      = "render_class_dump_reruns_decompile"
	StageE2E         = "e2e_kernel"
	StageJavac       = "javac_oracle_separate"
	StageJava        = "java_oracle_separate"
)

var KernelStages = []string{
	StageParse, StageDecode, StageCFG, StageSemanticCFG, StageDataflow, StageT25Sparse, StageT26Cache, StageRegion, StageRender, StageE2E,
}

// StageInventoryEntry records what is actually timed. It is evidence, not a claim
// that the production pipeline has a matching named pass.
type StageInventoryEntry struct {
	ID            string `json:"id"`
	Timed         string `json:"timed"`
	ProductionAPI string `json:"production_api"`
	Exclusive     bool   `json:"exclusive"`
	Combined      bool   `json:"combined"`
	RerunsDecomp  bool   `json:"reruns_decompile"`
	OpcodeOnly    bool   `json:"opcode_only"`
	Limitation    string `json:"limitation"`
}

func StageInventory() []StageInventoryEntry {
	return []StageInventoryEntry{
		{
			ID: StageParse, Timed: "javajive.ParseClass", Exclusive: true,
			ProductionAPI: "javajive.ParseClass / classparser.Parse",
			Limitation:    "Class-file parse only.",
		},
		{
			ID: StageDecode, Timed: "(*core.Decompiler).ParseOpcode", Exclusive: true,
			ProductionAPI: "(*core.Decompiler).ParseOpcode",
			Limitation:    "opCodes is unexported; harvest may ScanJmp after the timed region to count opcodes.",
		},
		{
			ID: StageCFG, Timed: "ScanJmp+DropUnreachableOpcode", Exclusive: true, OpcodeOnly: true,
			ProductionAPI: "(*core.Decompiler).ScanJmp, (*core.Decompiler).DropUnreachableOpcode",
			Limitation:    "Opcode CFG only. Production SemanticCFG is timed separately as semantic_cfg_build.",
		},
		{
			ID: StageSemanticCFG, Timed: "(*core.Decompiler).BuildSemanticCFG after ParseOpcode", Exclusive: true,
			ProductionAPI: "(*core.Decompiler).BuildSemanticCFG",
			Limitation:    "Pure production constructor. nodes/edges harvested from SemanticCFG.",
		},
		{
			ID: StageDataflow, Timed: "(*core.Decompiler).CalcOpcodeStackInfo only", Exclusive: true, OpcodeOnly: true,
			ProductionAPI: "(*core.Decompiler).CalcOpcodeStackInfo",
			Limitation:    "Opcode stack simulation. T25 sparse reaching is timed as t25_sparse_reaching.",
		},
		{
			ID: StageT25Sparse, Timed: "NewSparseReaching + Definitions over stores", Exclusive: true,
			ProductionAPI: "core.NewSparseReaching, (*SparseReaching).Definitions, (*SemanticCFG).ReachingDefinitions",
			Limitation:    "def_merge is sparse union work (Merges), not opcode-graph proxies.",
		},
		{
			ID: StageT26Cache, Timed: "SemanticCFG.GetOrCompute dominators", Exclusive: true,
			ProductionAPI: "(*SemanticCFG).GetOrCompute, AnalysisComputeCount, AnalysisNodesTouched",
			Limitation:    "Per-request CFG; cache is not shared across requests.",
		},
		{
			ID: StageRegion, Timed: "decompiler.ParseBytesCode (ParseStatement + rewriter)", Combined: true,
			ProductionAPI: "decompiler.ParseBytesCode",
			Limitation:    "ParseBytesCode re-runs opcode parse/CFG/stacksim then structures+rewrites. Rewriter is not split without copying production algorithms.",
		},
		{
			ID: StageRender, Timed: "(*ClassObject).Dump after Parse", Combined: true, RerunsDecomp: true,
			ProductionAPI: "(*javaclassparser.ClassObject).Dump",
			Limitation:    "Dump re-runs method decompile + string render. Dump() leaves Mode empty so compatibility source rewrites may run; e2e_kernel uses Precision via DecompileWithOptions and is not the same path.",
		},
		{
			ID: StageE2E, Timed: "javajive.DecompileWithOptions Precision (Parse + DumpClass)", Exclusive: true,
			ProductionAPI: "javajive.DecompileWithOptions",
			Limitation:    "javac/java oracle times are separate fields and are never added into kernel groups.",
		},
	}
}

type FamilyMeta struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	ScaleLabel    string `json:"scale_label"`
	N             int    `json:"n"`
	Shape         string `json:"shape"`
	ClassInternal string `json:"class_internal"`
	SHA256        string `json:"sha256"`
	SizeBytes     int    `json:"size_bytes"`
	Source        string `json:"source"`
	Reviewed      bool   `json:"reviewed"`
}

// WorkCounts is the stable counter record filled from production T24/T25/T26
// algorithms. nodes/edges come from SemanticCFG; def_merge from sparse Merges;
// region_roots from statement roots plus T26 AnalysisComputeCount.
// pending_t24_t25_t26 is false on this integrated tree.
type WorkCounts struct {
	InputSHA256       string   `json:"input_sha256"`
	MethodCount       int      `json:"method_count"`
	CodeMethodCount   int      `json:"code_method_count"`
	BytecodeBytes     int      `json:"bytecode_bytes"`
	OpcodeCount       int      `json:"opcode_count"`
	CFGNodes          int      `json:"cfg_nodes"`
	CFGEdges          int      `json:"cfg_edges"`
	Nodes             int      `json:"nodes"`
	Edges             int      `json:"edges"`
	DefMerge          int      `json:"def_merge"`
	RegionRoots       int      `json:"region_roots"`
	StoreCount        int      `json:"store_count"`
	LoadCount         int      `json:"load_count"`
	IincCount         int      `json:"iinc_count"`
	ExceptionEntries  int      `json:"exception_entries"`
	HandlerPCs        int      `json:"handler_pcs"`
	DefMergeLoads     int      `json:"harness_def_merge_loads"`
	DefStoreLoadPairs int      `json:"harness_def_store_load_pairs"`
	StatementCount    int      `json:"statement_count"`
	OutputSHA256      string   `json:"output_sha256"`
	OutputBytes       int      `json:"output_bytes"`
	DecompileStatus   string   `json:"decompile_status"`
	StubMethodCount   int      `json:"stub_method_count"`
	StubMethods       []string `json:"stub_methods,omitempty"`
	SkippedAnalysis   bool     `json:"skipped_analysis"`
	PendingT24T25T26  bool     `json:"pending_t24_t25_t26"`
	Notes             string   `json:"notes"`
}

func (w *WorkCounts) fillCanonical() {
	if w.Nodes == 0 {
		w.Nodes = w.CFGNodes
	}
	if w.Edges == 0 {
		w.Edges = w.CFGEdges
	}
	if w.DefMerge == 0 {
		w.DefMerge = w.DefMergeLoads
	}
}

type Sample struct {
	I            int    `json:"i"`
	Role         string `json:"role"` // warmup | measured
	NS           int64  `json:"ns"`
	BOp          int64  `json:"b_op"`
	Allocs       int64  `json:"allocs_op"`
	PeakRSSBytes *int64 `json:"peak_rss_bytes"`
	RSSSource    string `json:"rss_source,omitempty"`
	OK           bool   `json:"ok"`
	Error        string `json:"error,omitempty"`
	OutputSHA256 string `json:"output_sha256,omitempty"`
}

type Percentiles struct {
	P50NS    int64 `json:"p50_ns"`
	P90NS    int64 `json:"p90_ns"`
	P99NS    int64 `json:"p99_ns"`
	P50B     int64 `json:"p50_b_op"`
	P50Alloc int64 `json:"p50_allocs_op"`
}

type GroupResult struct {
	GroupID             string      `json:"group_id"`
	FamilyID            string      `json:"family_id"`
	Stage               string      `json:"stage"`
	Mode                string      `json:"mode"` // cold | warm
	CacheKey            string      `json:"cache_key"`
	InputSHA256         string      `json:"input_sha256"`
	RepeatsRequested    int         `json:"repeats_requested"`
	ValidRepeats        int         `json:"valid_repeats"`
	Completeness        float64     `json:"raw_data_completeness"`
	Samples             []Sample    `json:"samples"`
	Percentiles         Percentiles `json:"percentiles"`
	MeanNS              float64     `json:"mean_ns"`
	StdevNS             float64     `json:"stdev_ns"`
	MeanBOp             float64     `json:"mean_b_op"`
	MeanAllocs          float64     `json:"mean_allocs_op"`
	PeakRSSBytes        *int64      `json:"independent_process_peak_rss_bytes"`
	RSSNote             string      `json:"rss_note,omitempty"`
	Work                WorkCounts  `json:"work"`
	Noisy               bool        `json:"environment_noisy"`
	NoisyNote           string      `json:"noisy_note,omitempty"`
	BestTrialNS         int64       `json:"best_trial_ns_not_used_as_result"`
	JavacNS             *int64      `json:"javac_ns_separate"`
	JavaNS              *int64      `json:"java_ns_separate"`
	KernelIncludesJavac bool        `json:"kernel_includes_javac"`
}

type MachineInfo struct {
	Hostname    string `json:"hostname"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	NumCPU      int    `json:"num_cpu"`
	GOMAXPROCS  int    `json:"gomaxprocs"`
	Uname       string `json:"uname"`
	CPUBrand    string `json:"cpu_brand"`
	MachineNote string `json:"machine_difference_note"`
}

type ToolchainInfo struct {
	GoVersion    string `json:"go_version"`
	Compiler     string `json:"compiler"`
	GOTOOLCHAIN  string `json:"gotoolchain"`
	CGOEnabled   string `json:"cgo_enabled"`
	Ldflags      string `json:"ldflags"`
	GOOS         string `json:"goos"`
	GOARCH       string `json:"goarch"`
	JavacVersion string `json:"javac_version"`
	JavaVersion  string `json:"java_version"`
}

type CommandRecord struct {
	Phase    string            `json:"phase"`
	Argv     []string          `json:"argv"`
	Cwd      string            `json:"cwd"`
	ExitCode int               `json:"exit_code"`
	Env      map[string]string `json:"env"`
	Note     string            `json:"note,omitempty"`
}

type EvidenceDoc struct {
	SchemaVersion            int                   `json:"schema_version"`
	TaskID                   string                `json:"task_id"`
	WorkingTreeSHA           string                `json:"working_tree_sha"`
	HistoricalMilestoneSHA   string                `json:"historical_milestone_sha"`
	WorktreeDirty            bool                  `json:"worktree_dirty"`
	CollectNonce             string                `json:"collect_nonce"`
	CollectedAtUnix          int64                 `json:"collected_at_unix"`
	ReducedCollect           bool                  `json:"reduced_collect"`
	PendingT24T25T26         bool                  `json:"pending_t24_t25_t26"`
	Machine                  MachineInfo           `json:"machine"`
	Toolchain                ToolchainInfo         `json:"toolchain"`
	UncalibratedPercentGates int                   `json:"uncalibrated_hard_percentage_gates"`
	StageSplitNotes          []string              `json:"stage_split_notes"`
	StageInventory           []StageInventoryEntry `json:"stage_inventory"`
	Families                 []FamilyMeta          `json:"families"`
	Groups                   []GroupResult         `json:"groups"`
	Noise                    NoiseReport           `json:"noise"`
	Semantic                 SemanticReport        `json:"semantic"`
	Commands                 []CommandRecord       `json:"commands"`
	GoBenchRaw               string                `json:"go_bench_raw,omitempty"`
}

type NoiseReport struct {
	JitterVerdict           string    `json:"jitter_verdict"`
	JitterRatioP50          float64   `json:"jitter_ratio_p50"`
	JitterCILow             float64   `json:"jitter_ci_low"`
	JitterCIHigh            float64   `json:"jitter_ci_high"`
	JitterCalledImprovement bool      `json:"jitter_called_improvement"`
	RegressionVerdict       string    `json:"regression_verdict"`
	RegressionRatioP50      float64   `json:"regression_ratio_p50"`
	RegressionCILow         float64   `json:"regression_ci_low"`
	RegressionCIHigh        float64   `json:"regression_ci_high"`
	RegressionDetected      bool      `json:"clear_2x_regression_detected"`
	BaselineNS              []float64 `json:"baseline_ns"`
	JitterNS                []float64 `json:"jitter_ns"`
	RegressionNS            []float64 `json:"regression_2x_ns"`
	Note                    string    `json:"note"`
}

type OracleObservation struct {
	Attempted    bool   `json:"attempted"`
	Ran          bool   `json:"ran"`
	Equal        bool   `json:"stdout_equal"`
	BaselineOut  string `json:"baseline_stdout,omitempty"`
	CandidateOut string `json:"candidate_stdout,omitempty"`
	Note         string `json:"note"`
}

type SemanticReport struct {
	Baseline                     WorkCounts        `json:"baseline"`
	FasterStub                   WorkCounts        `json:"faster_stub"`
	StubMeanNS                   float64           `json:"stub_mean_ns"`
	BaselineMeanNS               float64           `json:"baseline_mean_ns"`
	StubFaster                   bool              `json:"stub_is_faster"`
	Classification               string            `json:"classification"`
	RecordedAsPerfWin            bool              `json:"recorded_as_perf_win"`
	CorrectnessRegression        bool              `json:"correctness_regression"`
	SourceHashEqual              bool              `json:"source_hash_equal"`
	SourceHashUsedAsSemanticFail bool              `json:"source_hash_used_as_semantic_fail"`
	Oracle                       OracleObservation `json:"oracle"`
	RebuiltRunOracle             OracleObservation `json:"baseline_rebuilt_run_oracle"`
	Note                         string            `json:"note"`
}

package performance_baseline

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/yaklang/javajive"
)

var sessionDoc *EvidenceDoc

func packageDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Dir(file)
}

func EvidenceDir() string {
	if v := os.Getenv("T32_EVIDENCE_DIR"); v != "" {
		_ = os.MkdirAll(v, 0o755)
		return v
	}
	if v := os.Getenv("JAVAJIVE_EVIDENCE_DIR"); v != "" {
		dir := filepath.Join(v, "t32")
		_ = os.MkdirAll(dir, 0o755)
		return dir
	}
	// Default tests must not mutate tracked tools/performance_baseline/evidence.
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("javajive-evidence-%d", os.Getpid()), "t32")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

func RepoRoot() string {
	return filepath.Clean(filepath.Join(packageDir(), "..", ".."))
}

func reducedCollect() bool {
	return os.Getenv("T32_REDUCED") == "1"
}

func sessionNonce() string {
	if v := os.Getenv("T32_SESSION_NONCE"); v != "" {
		return v
	}
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid())
}

func StageSplitNotes() []string {
	return []string{
		"parse: javajive.ParseClass / classparser.Parse",
		"decode: core.Decompiler.ParseOpcode (exclusive; parse is prefix)",
		"cfg_opcode_only: ScanJmp+DropUnreachableOpcode (exclusive opcode CFG). Not SemanticCFG.",
		"semantic_cfg_build: (*core.Decompiler).BuildSemanticCFG after ParseOpcode (pure production constructor).",
		"dataflow_stacksim_opcode_only: CalcOpcodeStackInfo (exclusive opcode stacksim).",
		"t25_sparse_reaching: NewSparseReaching + Definitions; def_merge from sparse Merges.",
		"t26_graph_cache: SemanticCFG.GetOrCompute dominators; AnalysisComputeCount per request.",
		"region_and_statements_combined: decompiler.ParseBytesCode (ParseStatement + rewriter). Combined total.",
		"render_class_dump_reruns_decompile: ClassObject.Dump after Parse. Dump re-runs method decompile+string render.",
		"e2e_kernel: javajive.DecompileWithOptions Precision (Parse+DumpClass). javac/java oracle times are never added into kernel groups.",
		"pending_t24_t25_t26 is false: T24 SemanticCFG, T25 sparse reaching, and T26 GetOrCompute counters are production-backed on this tree.",
		"original class bytes must pass java -Xverify:all; no -Xverify:none fallback.",
		"javac/java oracle times are never added into decompiler kernel groups.",
	}
}

func runWarmGroup(fam Family, stage string, repeats int) (GroupResult, error) {
	g := GroupResult{
		GroupID:             fmt.Sprintf("%s|%s|warm", fam.Meta.ID, stage),
		FamilyID:            fam.Meta.ID,
		Stage:               stage,
		Mode:                "warm",
		InputSHA256:         fam.Meta.SHA256,
		RepeatsRequested:    repeats,
		KernelIncludesJavac: false,
	}
	g.CacheKey = cacheKey("warm", stage, fam.Meta.SHA256, stage != StageParse && stage != StageE2E)
	for i := 0; i < WarmupRepeats+repeats; i++ {
		s, err := measureOnce(stage, fam.Bytes)
		s.I = i
		if i < WarmupRepeats {
			s.Role = "warmup"
		} else {
			s.Role = "measured"
		}
		if err != nil && s.Error == "" {
			s.Error = err.Error()
			s.OK = false
		}
		g.Samples = append(g.Samples, s)
	}
	work, werr := harvestWork(stage, fam.Bytes)
	if werr != nil && work.Notes == "" {
		work.Notes = werr.Error()
	}
	g.Work = work
	summarizeGroup(&g)
	return g, nil
}

func runColdGroup(exe, tmp string, fam Family, stage string, repeats int) (GroupResult, error) {
	g := GroupResult{
		GroupID:             fmt.Sprintf("%s|%s|cold", fam.Meta.ID, stage),
		FamilyID:            fam.Meta.ID,
		Stage:               stage,
		Mode:                "cold",
		CacheKey:            cacheKey("cold", stage, fam.Meta.SHA256, false),
		InputSHA256:         fam.Meta.SHA256,
		RepeatsRequested:    repeats,
		KernelIncludesJavac: false,
		RSSNote:             "peak RSS from independent child process rusage, not B/op",
	}
	var lastWork WorkCounts
	for i := 0; i < repeats; i++ {
		s, work, err := spawnColdSample(exe, fam.Meta.ID, stage, tmp)
		s.I = i
		s.Role = "measured"
		if err != nil && s.Error == "" {
			s.Error = err.Error()
			s.OK = false
		}
		g.Samples = append(g.Samples, s)
		lastWork = work
	}
	g.Work = lastWork
	summarizeGroup(&g)
	return g, nil
}

func measureJavacSeparate(tmp string) (ns *int64, version string, err error) {
	src := filepath.Join(tmp, "T32JavacProbe.java")
	body := "public class T32JavacProbe { public static int run(int x) { return x + 1; } }\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		return nil, "", err
	}
	cmd := exec.Command("javac", "-version")
	verB, _ := cmd.CombinedOutput()
	version = strings.TrimSpace(string(verB))
	t0 := time.Now()
	c := exec.Command("javac", "--release", "8", src)
	c.Dir = tmp
	out, err := c.CombinedOutput()
	elapsed := time.Since(t0).Nanoseconds()
	if err != nil {
		return nil, version, fmt.Errorf("javac: %v (%s)", err, out)
	}
	return &elapsed, version, nil
}

type warmJob struct {
	fam   Family
	stage string
}

func warmJobs(fams []Family) []warmJob {
	var out []warmJob
	reduced := reducedCollect()
	for _, f := range fams {
		if reduced {
			if f.Meta.ID == "tiny_real/reviewed" {
				for _, st := range KernelStages {
					out = append(out, warmJob{f, st})
				}
				continue
			}
			switch f.Meta.Kind {
			case "loop", "handler_dense", "handler_sparse", "slot":
				out = append(out, warmJob{f, StageE2E})
			}
			continue
		}
		for _, st := range KernelStages {
			out = append(out, warmJob{f, st})
		}
	}
	return out
}

func coldTargets() []struct{ id, stage string } {
	if reducedCollect() {
		return []struct{ id, stage string }{
			{"tiny_real/reviewed", StageE2E},
			{"tiny_real/reviewed", StageParse},
			{"loop/N", StageE2E},
		}
	}
	return []struct{ id, stage string }{
		{"tiny_real/reviewed", StageE2E},
		{"tiny_real/reviewed", StageParse},
		{"loop/N", StageE2E},
		{"loop/4N", StageE2E},
		{"handler_dense/N", StageE2E},
		{"handler_sparse/N", StageE2E},
		{"slot/N", StageE2E},
		{"deep_expr/N", StageE2E},
	}
}

func CollectEvidence(exe string) (*EvidenceDoc, error) {
	ev := EvidenceDir()
	if err := os.MkdirAll(ev, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(ev, "families"), 0o755); err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "t32-perf-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	repo := RepoRoot()
	sha, dirty := gitSHA(repo)
	nonce := sessionNonce()
	doc := &EvidenceDoc{
		SchemaVersion:            SchemaVersion,
		TaskID:                   "T32",
		WorkingTreeSHA:           sha,
		HistoricalMilestoneSHA:   HistoricalMilestoneSHA,
		WorktreeDirty:            dirty,
		CollectNonce:             nonce,
		CollectedAtUnix:          time.Now().Unix(),
		ReducedCollect:           reducedCollect(),
		PendingT24T25T26:         false,
		Machine:                  collectMachine(),
		Toolchain:                collectToolchain(),
		UncalibratedPercentGates: UncalibratedPercentGates,
		StageSplitNotes:          StageSplitNotes(),
		StageInventory:           StageInventory(),
	}

	fams := FrozenFamilies()
	for _, f := range fams {
		doc.Families = append(doc.Families, f.Meta)
		path := filepath.Join(ev, "families", familyIDSafe(f.Meta.ID)+".class")
		if err := os.WriteFile(path, f.Bytes, 0o644); err != nil {
			return nil, err
		}
	}

	javacNS, javacVer, javacErr := measureJavacSeparate(tmp)
	if javacVer != "" {
		doc.Toolchain.JavacVersion = javacVer
	}

	for _, job := range warmJobs(fams) {
		g, err := runWarmGroup(job.fam, job.stage, MinValidRepeats)
		if err != nil {
			g.Work.Notes = "group_error: " + err.Error()
		}
		if javacNS != nil {
			v := *javacNS
			g.JavacNS = &v
		}
		g.KernelIncludesJavac = false
		doc.Groups = append(doc.Groups, g)
	}

	if exe != "" {
		for _, t := range coldTargets() {
			fam, err := familyByID(t.id)
			if err != nil {
				return nil, err
			}
			g, err := runColdGroup(exe, tmp, fam, t.stage, MinValidRepeats)
			if err != nil {
				g.Work.Notes = "group_error: " + err.Error()
			}
			g.KernelIncludesJavac = false
			if javacNS != nil {
				v := *javacNS
				g.JavacNS = &v
			}
			doc.Groups = append(doc.Groups, g)
		}
		if rss, src, err := timeDashL(exe, "tiny_real/reviewed", StageE2E, tmp); err == nil {
			doc.Commands = append(doc.Commands, CommandRecord{
				Phase:    "rss_time_l_crosscheck",
				Argv:     []string{"/usr/bin/time", "-l", exe},
				Cwd:      tmp,
				ExitCode: 0,
				Note:     fmt.Sprintf("peak_rss_bytes=%d source=%s", rss, src),
			})
		}
	}

	var base []float64
	for _, g := range doc.Groups {
		if g.FamilyID == "tiny_real/reviewed" && g.Stage == StageE2E && g.Mode == "warm" {
			base = measuredNS(g)
			break
		}
	}
	if len(base) >= MinValidRepeats {
		doc.Noise = runNoiseSelfTest(base)
	} else {
		doc.Noise.Note = "insufficient baseline samples for noise self-test"
	}

	tiny, _ := familyByID("tiny_real/reviewed")
	doc.Semantic = runSemanticCoupling(tiny.Bytes)
	if loop, err := familyByID("loop/N"); err == nil {
		res, derr := javajive.DecompileWithOptions(loop.Bytes, javajive.DecompileOptions{Mode: javajive.Precision})
		src := ""
		if derr == nil {
			src = res.Source
		}
		doc.Semantic.RebuiltRunOracle = tryRebuiltRunOracle(loop.Bytes, loop.Meta.ClassInternal, src)
	}

	doc.Commands = append(doc.Commands, CommandRecord{
		Phase:    "acceptance",
		Argv:     []string{"go", "test", "./tools/performance_baseline", "-count=1", "-timeout=25m", "-ldflags=-linkmode=external"},
		Cwd:      repo,
		ExitCode: 0,
		Env: map[string]string{
			"CGO_ENABLED":       os.Getenv("CGO_ENABLED"),
			"GOTOOLCHAIN":       getenvDefault("GOTOOLCHAIN", "local"),
			"T32_LDFLAGS":       "-linkmode=external",
			"T32_SESSION_NONCE": nonce,
		},
		Note: "macOS Go 1.22 tests must use CGO_ENABLED=1 GOTOOLCHAIN=local -ldflags=-linkmode=external. This collect_nonce identifies the run that wrote this evidence.",
	})
	if javacErr != nil {
		doc.Commands = append(doc.Commands, CommandRecord{
			Phase:    "javac_oracle_separate",
			Argv:     []string{"javac", "--release", "8", "T32JavacProbe.java"},
			ExitCode: 1,
			Note:     javacErr.Error(),
		})
	} else {
		doc.Commands = append(doc.Commands, CommandRecord{
			Phase:    "javac_oracle_separate",
			Argv:     []string{"javac", "--release", "8", "T32JavacProbe.java"},
			ExitCode: 0,
			Note:     "javac time is a separate field and is never added into e2e_kernel",
		})
	}

	if err := writeEvidence(ev, doc); err != nil {
		return nil, err
	}
	sessionDoc = doc
	return doc, nil
}

func writeEvidence(ev string, doc *EvidenceDoc) error {
	write := func(name string, v any) error {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		b = append(b, '\n')
		return os.WriteFile(filepath.Join(ev, name), b, 0o644)
	}
	if err := write("summary.json", doc); err != nil {
		return err
	}
	if err := write("families.json", doc.Families); err != nil {
		return err
	}
	if err := write("machine.json", map[string]any{"machine": doc.Machine, "toolchain": doc.Toolchain, "sha": doc.WorkingTreeSHA, "historical_milestone_sha": doc.HistoricalMilestoneSHA, "dirty": doc.WorktreeDirty, "collect_nonce": doc.CollectNonce, "collected_at_unix": doc.CollectedAtUnix}); err != nil {
		return err
	}
	if err := write("commands.json", doc.Commands); err != nil {
		return err
	}
	if err := write("noise.json", doc.Noise); err != nil {
		return err
	}
	if err := write("semantic.json", doc.Semantic); err != nil {
		return err
	}
	if err := write("stage-inventory.json", doc.StageInventory); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(ev, "collect-nonce.txt"), []byte(doc.CollectNonce+"\n"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(ev, "collected-at.txt"), []byte(fmt.Sprintf("%d\n", doc.CollectedAtUnix)), 0o644); err != nil {
		return err
	}
	type slim struct {
		GroupID             string      `json:"group_id"`
		FamilyID            string      `json:"family_id"`
		Stage               string      `json:"stage"`
		Mode                string      `json:"mode"`
		CacheKey            string      `json:"cache_key"`
		InputSHA256         string      `json:"input_sha256"`
		ValidRepeats        int         `json:"valid_repeats"`
		Completeness        float64     `json:"raw_data_completeness"`
		Samples             []Sample    `json:"samples"`
		Percentiles         Percentiles `json:"percentiles"`
		MeanNS              float64     `json:"mean_ns"`
		StdevNS             float64     `json:"stdev_ns"`
		Work                WorkCounts  `json:"work"`
		PeakRSSBytes        *int64      `json:"independent_process_peak_rss_bytes"`
		JavacNS             *int64      `json:"javac_ns_separate"`
		KernelIncludesJavac bool        `json:"kernel_includes_javac"`
	}
	f, err := os.Create(filepath.Join(ev, "samples.jsonl"))
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, g := range doc.Groups {
		if err := enc.Encode(slim{
			GroupID: g.GroupID, FamilyID: g.FamilyID, Stage: g.Stage, Mode: g.Mode,
			CacheKey: g.CacheKey, InputSHA256: g.InputSHA256, ValidRepeats: g.ValidRepeats,
			Completeness: g.Completeness, Samples: g.Samples, Percentiles: g.Percentiles,
			MeanNS: g.MeanNS, StdevNS: g.StdevNS, Work: g.Work, PeakRSSBytes: g.PeakRSSBytes,
			JavacNS: g.JavacNS, KernelIncludesJavac: g.KernelIncludesJavac,
		}); err != nil {
			return err
		}
	}
	return nil
}

func LoadEvidence() (*EvidenceDoc, error) {
	b, err := os.ReadFile(filepath.Join(EvidenceDir(), "summary.json"))
	if err != nil {
		return nil, err
	}
	var doc EvidenceDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

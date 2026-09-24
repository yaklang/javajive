package performance_baseline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive"
	javaclassparser "github.com/yaklang/javajive/classparser"
)

func TestMain(m *testing.M) {
	if os.Getenv("T32_CHILD") == "1" {
		if err := RunChild(); err != nil {
			os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func ensureEvidence(t *testing.T) *EvidenceDoc {
	t.Helper()
	if sessionDoc != nil && len(sessionDoc.Groups) > 0 {
		return sessionDoc
	}
	if os.Getenv("T32_FORCE_REFRESH") == "0" {
		doc, err := LoadEvidence()
		if err == nil && doc != nil && len(doc.Groups) > 0 {
			sessionDoc = doc
			return doc
		}
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("executable: %v", err)
	}
	doc, err := CollectEvidence(exe)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	sessionDoc = doc
	return doc
}

func TestTaskT32CollectEvidence(t *testing.T) {
	if os.Getenv("T32_REDUCED") == "1" {
		t.Fatal("TestTaskT32CollectEvidence requires a full collect; unset T32_REDUCED")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	doc, err := CollectEvidence(exe)
	if err != nil {
		t.Fatal(err)
	}
	sessionDoc = doc
	if len(doc.Families) < 16 {
		t.Fatalf("expected frozen families, got %d", len(doc.Families))
	}
	if doc.ReducedCollect {
		t.Fatal("full collect must not set reduced_collect")
	}
	if doc.UncalibratedPercentGates != 0 {
		t.Fatalf("uncalibrated gates must be 0")
	}
	if doc.PendingT24T25T26 {
		t.Fatal("pending_t24_t25_t26 must be false: T24/T25/T26 counters are production-backed")
	}
	if doc.CollectNonce == "" || doc.CollectedAtUnix == 0 {
		t.Fatal("collect nonce/timestamp missing; evidence was not refreshed")
	}
	if noiseOK(doc.Noise) != nil && len(doc.Noise.BaselineNS) >= MinValidRepeats {
		t.Fatal(noiseOK(doc.Noise))
	}
	wantWarm := len(FrozenFamilies()) * len(KernelStages)
	wantCold := len(coldTargets())
	if len(doc.Groups) < wantWarm+wantCold {
		t.Fatalf("full collect expected ≥%d groups, got %d", wantWarm+wantCold, len(doc.Groups))
	}
	for _, g := range doc.Groups {
		if g.ValidRepeats < MinValidRepeats {
			t.Fatalf("%s valid repeats %d < 10", g.GroupID, g.ValidRepeats)
		}
	}
}

func TestTaskT32PythonCollect(t *testing.T) {
	if os.Getenv("T32_COLLECT") != "1" {
		t.Skip("python harness collect; set T32_COLLECT=1")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	doc, err := CollectEvidence(exe)
	if err != nil {
		t.Fatal(err)
	}
	sessionDoc = doc
	if len(doc.Groups) == 0 {
		t.Fatal("no groups")
	}
	if doc.CollectNonce == "" {
		t.Fatal("missing collect nonce")
	}
	for _, g := range doc.Groups {
		if g.ValidRepeats < MinValidRepeats {
			t.Fatalf("%s repeats %d", g.GroupID, g.ValidRepeats)
		}
	}
}

func TestTaskT32HonestStageLabels(t *testing.T) {
	inv := StageInventory()
	seen := map[string]StageInventoryEntry{}
	for _, e := range inv {
		seen[e.ID] = e
	}
	for _, id := range []string{StageCFG, StageDataflow, StageRegion, StageRender} {
		if _, ok := seen[id]; !ok {
			t.Fatalf("missing inventory entry %s", id)
		}
	}
	if !strings.Contains(StageCFG, "_opcode_only") {
		t.Fatalf("cfg stage %s must contain _opcode_only", StageCFG)
	}
	if !strings.Contains(StageDataflow, "_opcode_only") {
		t.Fatalf("dataflow stage %s must contain _opcode_only", StageDataflow)
	}
	if !strings.Contains(StageRegion, "_combined") {
		t.Fatalf("region stage %s must contain _combined", StageRegion)
	}
	if !strings.Contains(StageRender, "_reruns_decompile") {
		t.Fatalf("render stage %s must contain _reruns_decompile", StageRender)
	}
	if seen[StageCFG].Timed == "" || !seen[StageCFG].OpcodeOnly {
		t.Fatal("cfg inventory must mark opcode_only and record what is timed")
	}
	if strings.Contains(strings.ToLower(seen[StageCFG].Timed), "semanticcfg") {
		t.Fatal("cfg must not claim SemanticCFG is timed")
	}
	if !strings.Contains(strings.ToLower(seen[StageDataflow].Timed), "calcopcodestackinfo") {
		t.Fatal("dataflow must time CalcOpcodeStackInfo")
	}
	if !seen[StageRegion].Combined {
		t.Fatal("region must be marked combined")
	}
	if !seen[StageRender].RerunsDecomp {
		t.Fatal("render must be marked reruns_decompile")
	}
	for _, st := range KernelStages {
		ok := false
		for _, e := range inv {
			if e.ID == st {
				ok = true
			}
		}
		if !ok {
			t.Fatalf("kernel stage %s missing from inventory", st)
		}
	}
}

func TestTaskT32C01StageIsolation(t *testing.T) {
	doc := ensureEvidence(t)
	var parse, e2e *GroupResult
	seen := map[string]bool{}
	for i := range doc.Groups {
		g := &doc.Groups[i]
		if g.FamilyID != "tiny_real/reviewed" || g.Mode != "warm" {
			continue
		}
		seen[g.Stage] = true
		if g.ValidRepeats < MinValidRepeats {
			t.Fatalf("%s valid repeats %d < 10", g.GroupID, g.ValidRepeats)
		}
		if g.KernelIncludesJavac {
			t.Fatalf("%s kernel includes javac", g.GroupID)
		}
		if g.Stage == StageE2E && g.JavacNS != nil && g.MeanNS == float64(*g.JavacNS) && *g.JavacNS > 0 {
			t.Fatalf("e2e mean equals javac time; javac was mixed into kernel")
		}
		if g.Stage == StageParse {
			parse = g
		}
		if g.Stage == StageE2E {
			e2e = g
		}
	}
	for _, st := range KernelStages {
		if !seen[st] {
			t.Fatalf("missing stage %s", st)
		}
	}
	if !seen[StageCFG] || !seen[StageDataflow] || !seen[StageRegion] || !seen[StageRender] {
		t.Fatalf("honest labels missing: %#v", seen)
	}
	if parse == nil || e2e == nil {
		t.Fatal("need parse and e2e on the same class")
	}
	if parse.InputSHA256 != e2e.InputSHA256 {
		t.Fatalf("stage isolation must use the same class hash: parse=%s e2e=%s", parse.InputSHA256, e2e.InputSHA256)
	}
	if e2e.Stage != StageE2E {
		t.Fatalf("e2e stage label %s", e2e.Stage)
	}
	split := strings.ToLower(strings.Join(doc.StageSplitNotes, " "))
	if !strings.Contains(split, "semanticcfg") && !strings.Contains(split, "semantic cfg") {
		t.Fatal("stage notes must mention SemanticCFG")
	}
	if !strings.Contains(split, "t25") || !strings.Contains(split, "t26") {
		t.Fatal("stage notes must mention production T25/T26 stages")
	}
	if doc.PendingT24T25T26 {
		t.Fatal("pending_t24_t25_t26 must be false: T24/T25/T26 are production-backed")
	}
}

func TestTaskT32C02ColdWarm(t *testing.T) {
	doc := ensureEvidence(t)
	if doc.CollectNonce == "" || doc.CollectedAtUnix == 0 {
		t.Fatal("cold/warm evidence must be from a collect in this process, not stale JSON without nonce")
	}
	var cold, warm *GroupResult
	for i := range doc.Groups {
		g := &doc.Groups[i]
		if g.FamilyID == "tiny_real/reviewed" && g.Stage == StageE2E {
			if g.Mode == "cold" {
				cold = g
			}
			if g.Mode == "warm" {
				warm = g
			}
		}
	}
	if cold == nil || warm == nil {
		t.Fatal("need cold and warm e2e groups")
	}
	if !strings.Contains(cold.CacheKey, "process=fresh") {
		t.Fatalf("cold cache key not trusted: %s", cold.CacheKey)
	}
	if !strings.Contains(warm.CacheKey, "process=shared") {
		t.Fatalf("warm cache key not trusted: %s", warm.CacheKey)
	}
	if cold.Mode != "cold" || warm.Mode != "warm" {
		t.Fatal("labels inaccurate")
	}
	if cold.ValidRepeats < MinValidRepeats || warm.ValidRepeats < MinValidRepeats {
		t.Fatalf("repeats cold=%d warm=%d", cold.ValidRepeats, warm.ValidRepeats)
	}
	if warm.BestTrialNS != 0 && warm.MeanNS == float64(warm.BestTrialNS) && warm.StdevNS > 0 {
		t.Fatal("mean equals best trial while stdev>0: picking the best trial is forbidden")
	}
	if cold.PeakRSSBytes == nil {
		t.Fatal("cold group must record independent-process peak RSS")
	}
}

func TestTaskT32C03ScaleCurve(t *testing.T) {
	doc := ensureEvidence(t)
	for _, kind := range []string{"loop", "handler_dense", "handler_sparse", "slot"} {
		var n, n2, n4 *GroupResult
		for i := range doc.Groups {
			g := &doc.Groups[i]
			if g.Mode != "warm" || g.Stage != StageE2E {
				continue
			}
			switch g.FamilyID {
			case kind + "/N":
				n = g
			case kind + "/2N":
				n2 = g
			case kind + "/4N":
				n4 = g
			}
		}
		if n == nil || n2 == nil || n4 == nil {
			t.Fatalf("%s missing N/2N/4N e2e groups", kind)
		}
		if n.InputSHA256 == n2.InputSHA256 || n2.InputSHA256 == n4.InputSHA256 {
			t.Fatalf("%s scale hashes must differ", kind)
		}
		if n.Work.CFGNodes == 0 && n.Work.OpcodeCount == 0 && n.Work.BytecodeBytes == 0 && n.Work.Nodes == 0 {
			t.Fatalf("%s missing node/edge/bytecode work counts", kind)
		}
		if n.Work.PendingT24T25T26 {
			t.Fatalf("%s work.pending_t24_t25_t26 still true", kind)
		}
		if n4.Work.BytecodeBytes < n2.Work.BytecodeBytes || n2.Work.BytecodeBytes < n.Work.BytecodeBytes {
			t.Fatalf("%s bytecode did not grow N=%d 2N=%d 4N=%d", kind, n.Work.BytecodeBytes, n2.Work.BytecodeBytes, n4.Work.BytecodeBytes)
		}
		if n4.Work.OpcodeCount > 0 && n.Work.OpcodeCount > 0 && n4.Work.OpcodeCount < n.Work.OpcodeCount {
			t.Fatalf("%s opcode counts shrank with scale", kind)
		}
		if n.Work.Nodes == 0 || n.Work.Edges == 0 {
			t.Fatalf("%s e2e SemanticCFG nodes/edges missing: nodes=%d edges=%d", kind, n.Work.Nodes, n.Work.Edges)
		}
		if n.Work.DefMerge < 0 {
			t.Fatalf("%s def_merge negative: %d", kind, n.Work.DefMerge)
		}
	}
}

func TestTaskT32C04NoiseSelfTest(t *testing.T) {
	doc := ensureEvidence(t)
	if err := noiseOK(doc.Noise); err != nil {
		t.Fatal(err)
	}
	if doc.Noise.JitterVerdict == "improvement" {
		t.Fatal("uncertain jitter called improvement")
	}
	if !doc.Noise.RegressionDetected {
		t.Fatal("2x regression not detected")
	}
}

func TestTaskT32C05StubNotPerfWin(t *testing.T) {
	doc := ensureEvidence(t)
	s := doc.Semantic
	if !s.CorrectnessRegression {
		t.Fatal("faster stub must be a correctness regression")
	}
	if s.RecordedAsPerfWin {
		t.Fatal("stub recorded as perf win")
	}
	if !s.FasterStub.SkippedAnalysis {
		t.Fatal("fixture must skip analysis")
	}
	if s.Classification != "correctness_regression_not_perf_win" {
		t.Fatalf("classification %s", s.Classification)
	}
	if s.SourceHashUsedAsSemanticFail {
		t.Fatal("source hash inequality must not be used as a semantic fail")
	}
}

func TestTaskT32OracleSmoke(t *testing.T) {
	loop, err := familyByID("loop/N")
	if err != nil {
		t.Fatal(err)
	}
	res, err := javajive.DecompileWithOptions(loop.Bytes, javajive.DecompileOptions{Mode: javajive.Precision})
	if err != nil {
		t.Fatal(err)
	}
	obs := tryRebuiltRunOracle(loop.Bytes, loop.Meta.ClassInternal, res.Source)
	if !obs.Attempted {
		t.Fatal("oracle must attempt rebuilt run")
	}
	if !obs.Ran {
		t.Fatalf("loop/N rebuilt-run oracle did not run: %s", obs.Note)
	}
	if !obs.Equal {
		t.Fatalf("loop/N original vs rebuilt stdout mismatch orig=%q rebuilt=%q note=%s", obs.BaselineOut, obs.CandidateOut, obs.Note)
	}
	tiny, err := familyByID("tiny_real/reviewed")
	if err != nil {
		t.Fatal(err)
	}
	tres, err := javajive.DecompileWithOptions(tiny.Bytes, javajive.DecompileOptions{Mode: javajive.Precision})
	if err != nil {
		t.Fatal(err)
	}
	tobs := tryRebuiltRunOracle(tiny.Bytes, tiny.Meta.ClassInternal, tres.Source)
	if !tobs.Attempted {
		t.Fatal("tiny_real oracle must attempt")
	}
	if tobs.Ran && !tobs.Equal {
		t.Fatalf("tiny_real rebuilt stdout mismatch orig=%q rebuilt=%q note=%s", tobs.BaselineOut, tobs.CandidateOut, tobs.Note)
	}
}

func TestTaskT32C05SourceHashInequalityIsNotSemanticFail(t *testing.T) {
	a := WorkCounts{DecompileStatus: "complete", OutputSHA256: "aaa", OpcodeCount: 4, StatementCount: 2}
	b := WorkCounts{DecompileStatus: "complete", OutputSHA256: "bbb", OpcodeCount: 4, StatementCount: 2}
	class, corr, win := classifySemantic(a, b, true, OracleObservation{})
	if class != "uncertain_source_diff_not_semantic_fail" {
		t.Fatalf("classification %s", class)
	}
	if corr {
		t.Fatal("byte-different source is not automatically a semantic mismatch")
	}
	if win {
		t.Fatal("uncertain source diff must not be a perf win")
	}
	class, corr, win = classifySemantic(a, b, true, OracleObservation{Attempted: true, Ran: true, Equal: true})
	if class != "eligible_for_perf_compare" || corr || win {
		t.Fatalf("matching rebuilt-run stdout must not be a semantic fail: %s corr=%v win=%v", class, corr, win)
	}
	stub := WorkCounts{DecompileStatus: "partial", SkippedAnalysis: true, StubMethodCount: 1, OutputSHA256: "ccc"}
	class, corr, win = classifySemantic(a, stub, true, OracleObservation{Attempted: true, Ran: false})
	if class != "correctness_regression_not_perf_win" || !corr || win {
		t.Fatalf("skip/status must classify stub: %s corr=%v win=%v", class, corr, win)
	}
}

func TestTaskT32C06ReplayCommand(t *testing.T) {
	doc := ensureEvidence(t)
	if doc.SchemaVersion != SchemaVersion {
		t.Fatalf("schema %d", doc.SchemaVersion)
	}
	if len(doc.Commands) == 0 {
		t.Fatal("missing command records")
	}
	found := false
	for _, c := range doc.Commands {
		if len(c.Argv) == 0 {
			t.Fatal("command missing argv")
		}
		if c.Phase == "acceptance" {
			found = true
			joined := strings.Join(c.Argv, " ")
			if !strings.Contains(joined, "go") {
				t.Fatalf("acceptance argv %v", c.Argv)
			}
		}
	}
	if !found {
		t.Fatal("missing acceptance command")
	}
	if doc.Machine.OS == "" || doc.Toolchain.GoVersion == "" {
		t.Fatal("machine/toolchain missing")
	}
	if !strings.Contains(doc.Machine.MachineNote, "differences") {
		t.Fatal("machine differences must be explicit")
	}
	again := FrozenFamilies()
	if len(again) != len(doc.Families) {
		t.Fatalf("family count %d vs %d", len(again), len(doc.Families))
	}
	for i, f := range again {
		if f.Meta.SHA256 != doc.Families[i].SHA256 {
			t.Fatalf("replay hash mismatch %s", f.Meta.ID)
		}
		onDisk := filepath.Join(EvidenceDir(), "families", familyIDSafe(f.Meta.ID)+".class")
		raw, err := os.ReadFile(onDisk)
		if err != nil {
			t.Fatal(err)
		}
		if sha256Hex(raw) != f.Meta.SHA256 {
			t.Fatalf("on-disk family bytes mismatch %s", f.Meta.ID)
		}
	}
}

func TestTaskT32StageSmoke(t *testing.T) {
	for _, id := range []string{"tiny_real/reviewed", "loop/N", "handler_dense/N", "slot/N", "deep_expr/N"} {
		fam, err := familyByID(id)
		if err != nil {
			t.Fatal(err)
		}
		for _, stage := range KernelStages {
			s, err := measureOnce(stage, fam.Bytes)
			if err != nil || !s.OK {
				t.Fatalf("%s %s: %v %s", id, stage, err, s.Error)
			}
			if s.NS < 0 {
				t.Fatalf("negative ns %s %s", id, stage)
			}
			w, err := harvestWork(stage, fam.Bytes)
			if err != nil {
				t.Fatalf("harvest %s %s: %v", id, stage, err)
			}
			if w.PendingT24T25T26 {
				t.Fatalf("harvest %s %s pending_t24_t25_t26 still true", id, stage)
			}
			if w.InputSHA256 != fam.Meta.SHA256 && stage != StageE2E {
				if w.InputSHA256 != fam.Meta.SHA256 {
					t.Fatalf("hash %s %s got %s want %s", id, stage, w.InputSHA256, fam.Meta.SHA256)
				}
			}
		}
	}
}

func TestTaskT32FamiliesParseAndDump(t *testing.T) {
	for _, f := range FrozenFamilies() {
		obj, err := javaclassparser.Parse(f.Bytes)
		if err != nil {
			t.Fatalf("parse %s: %v", f.Meta.ID, err)
		}
		if obj.GetClassName() == "" {
			t.Fatalf("%s empty class name", f.Meta.ID)
		}
		src, err := javajive.Decompile(f.Bytes)
		if err != nil {
			t.Fatalf("decompile %s: %v", f.Meta.ID, err)
		}
		if strings.TrimSpace(src) == "" {
			t.Fatalf("empty dump %s", f.Meta.ID)
		}
	}
}

func TestTaskT32MetricsCompleteness(t *testing.T) {
	doc := ensureEvidence(t)
	incomplete := 0
	for _, g := range doc.Groups {
		if g.ValidRepeats < MinValidRepeats || g.Completeness < 1 {
			incomplete++
			t.Errorf("incomplete group %s valid=%d completeness=%.2f", g.GroupID, g.ValidRepeats, g.Completeness)
		}
		if g.Work.InputSHA256 == "" {
			t.Errorf("%s missing input hash", g.GroupID)
		}
		if g.Work.PendingT24T25T26 {
			t.Errorf("%s pending_t24_t25_t26 still true", g.GroupID)
		}
	}
	if incomplete != 0 {
		t.Fatalf("%d incomplete groups", incomplete)
	}
	if doc.UncalibratedPercentGates != 0 {
		t.Fatal("hard percentage gates must be 0")
	}
	if doc.Semantic.RecordedAsPerfWin {
		t.Fatal("semantic regression masked as perf pass")
	}
}

func BenchmarkTaskT32E2ETinyReal(b *testing.B)    { benchStage(b, "tiny_real/reviewed", StageE2E) }
func BenchmarkTaskT32ParseTinyReal(b *testing.B)  { benchStage(b, "tiny_real/reviewed", StageParse) }
func BenchmarkTaskT32DecodeTinyReal(b *testing.B) { benchStage(b, "tiny_real/reviewed", StageDecode) }
func BenchmarkTaskT32CFGTinyReal(b *testing.B)    { benchStage(b, "tiny_real/reviewed", StageCFG) }
func BenchmarkTaskT32DataflowTinyReal(b *testing.B) {
	benchStage(b, "tiny_real/reviewed", StageDataflow)
}
func BenchmarkTaskT32RegionTinyReal(b *testing.B) { benchStage(b, "tiny_real/reviewed", StageRegion) }
func BenchmarkTaskT32RenderTinyReal(b *testing.B) { benchStage(b, "tiny_real/reviewed", StageRender) }
func BenchmarkTaskT32E2ELoopN(b *testing.B)       { benchStage(b, "loop/N", StageE2E) }
func BenchmarkTaskT32E2ELoop2N(b *testing.B)      { benchStage(b, "loop/2N", StageE2E) }
func BenchmarkTaskT32E2ELoop4N(b *testing.B)      { benchStage(b, "loop/4N", StageE2E) }
func BenchmarkTaskT32E2ESlotN(b *testing.B)       { benchStage(b, "slot/N", StageE2E) }
func BenchmarkTaskT32E2EHandlerDenseN(b *testing.B) {
	benchStage(b, "handler_dense/N", StageE2E)
}

func benchStage(b *testing.B, famID, stage string) {
	b.Helper()
	fam, err := familyByID(famID)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		prefix, timed, err := exclusiveFns(stage, fam.Bytes)
		if err != nil {
			b.Fatal(err)
		}
		if prefix != nil {
			if err := prefix(); err != nil {
				b.Fatal(err)
			}
		}
		b.StartTimer()
		if err := timed(); err != nil {
			b.Fatal(err)
		}
	}
}

func TestTaskT32EvidenceJSONShape(t *testing.T) {
	doc := ensureEvidence(t)
	b, err := json.Marshal(doc.Groups[0])
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"group_id", "stage", "mode", "cache_key", "samples", "work", "kernel_includes_javac"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing field %s", k)
		}
	}
	wb, err := json.Marshal(doc.Groups[0].Work)
	if err != nil {
		t.Fatal(err)
	}
	var wm map[string]any
	if err := json.Unmarshal(wb, &wm); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"nodes", "edges", "def_merge", "region_roots", "pending_t24_t25_t26"} {
		if _, ok := wm[k]; !ok {
			t.Fatalf("work missing %s", k)
		}
	}
}

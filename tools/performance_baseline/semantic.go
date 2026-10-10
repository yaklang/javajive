package performance_baseline

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/yaklang/javajive"
)

func fasterStub(classBytes []byte) WorkCounts {
	h := sha256.Sum256(classBytes)
	src := fmt.Sprintf("public class FastStub { /* analysis skipped %x */ }\n", h[:4])
	w := WorkCounts{
		InputSHA256:      sha256Hex(classBytes),
		OutputSHA256:     sha256Hex([]byte(src)),
		OutputBytes:      len(src),
		DecompileStatus:  "partial",
		StubMethodCount:  1,
		StubMethods:      []string{"<all>"},
		SkippedAnalysis:  true,
		OpcodeCount:      0,
		StatementCount:   0,
		CFGNodes:         0,
		CFGEdges:         0,
		PendingT24T25T26: false,
		Notes:            "synthetic faster stub: skips parse/decode/cfg/dataflow/region/render",
	}
	w.fillCanonical()
	return w
}

func statusRank(status string) int {
	switch status {
	case "complete":
		return 3
	case "partial":
		return 2
	case "unsupported", "error":
		return 1
	case "invalid_input":
		return 0
	default:
		if status == "" {
			return -1
		}
		return 1
	}
}

func classifySemantic(baseline, candidate WorkCounts, candidateFaster bool, oracle OracleObservation) (classification string, correctness bool, perfWin bool) {
	if candidate.SkippedAnalysis && !baseline.SkippedAnalysis {
		return "correctness_regression_not_perf_win", true, false
	}
	if candidate.StubMethodCount > baseline.StubMethodCount {
		return "correctness_regression_not_perf_win", true, false
	}
	br, cr := statusRank(baseline.DecompileStatus), statusRank(candidate.DecompileStatus)
	if baseline.DecompileStatus != "" && candidate.DecompileStatus != "" && cr >= 0 && br >= 0 && cr < br {
		return "correctness_regression_not_perf_win", true, false
	}
	if oracle.Ran {
		if !oracle.Equal {
			return "correctness_regression_not_perf_win", true, false
		}
		_ = candidateFaster
		return "eligible_for_perf_compare", false, false
	}
	if oracle.Attempted && !oracle.Ran && candidate.SkippedAnalysis {
		return "correctness_regression_not_perf_win", true, false
	}
	// Byte-different Dump()/source is not a semantic fail. Equivalent programs
	// may pretty-print differently. Keep that case uncertain until an oracle runs.
	if candidate.OutputSHA256 != "" && baseline.OutputSHA256 != "" && candidate.OutputSHA256 != baseline.OutputSHA256 {
		return "uncertain_source_diff_not_semantic_fail", false, false
	}
	return "eligible_for_perf_compare", false, false
}

func runSemanticCoupling(classBytes []byte) SemanticReport {
	var baseNS []int64
	var stubNS []int64
	for i := 0; i < DefaultRepeats; i++ {
		t0 := time.Now()
		_, _ = javajive.DecompileWithOptions(classBytes, javajive.DecompileOptions{Mode: javajive.Precision})
		baseNS = append(baseNS, time.Since(t0).Nanoseconds())
		t1 := time.Now()
		_ = fasterStub(classBytes)
		stubNS = append(stubNS, time.Since(t1).Nanoseconds())
	}
	baseNS = baseNS[1:]
	stubNS = stubNS[1:]
	bm, _ := meanStdev(baseNS)
	sm, _ := meanStdev(stubNS)
	baseWork, _ := harvestWork(StageE2E, classBytes)
	stubWork := fasterStub(classBytes)
	faster := sm < bm
	oracle := observeStubOracle(classBytes, stubWork)
	class, corr, win := classifySemantic(baseWork, stubWork, faster, oracle)
	return SemanticReport{
		Baseline:                     baseWork,
		FasterStub:                   stubWork,
		StubMeanNS:                   sm,
		BaselineMeanNS:               bm,
		StubFaster:                   faster,
		Classification:               class,
		RecordedAsPerfWin:            win,
		CorrectnessRegression:        corr,
		SourceHashEqual:              baseWork.OutputSHA256 != "" && baseWork.OutputSHA256 == stubWork.OutputSHA256,
		SourceHashUsedAsSemanticFail: false,
		Oracle:                       oracle,
		Note:                         "Classification uses decompile status, skipped_analysis, and rebuilt-run stdout oracle. Dump()/source hash inequality is not a semantic fail (byte-different source may be equivalent). Uncertain remains explicit. A faster stub that skips analysis is a correctness regression, never a performance win. No speedup percentage is claimed.",
	}
}

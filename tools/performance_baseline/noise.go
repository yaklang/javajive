package performance_baseline

import (
	"fmt"
	"math/rand"
)

func runNoiseSelfTest(baseline []float64) NoiseReport {
	rng := rand.New(rand.NewSource(32))
	jitter := make([]float64, len(baseline))
	reg := make([]float64, len(baseline))
	for i, v := range baseline {
		// ±15% multiplicative jitter
		f := 0.85 + rng.Float64()*0.30
		jitter[i] = v * f
		reg[i] = v * 2.0
	}
	jr, jlo, jhi, jv := compareDistributions(baseline, jitter, rng)
	rr, rlo, rhi, rv := compareDistributions(baseline, reg, rng)
	rep := NoiseReport{
		JitterVerdict:           jv,
		JitterRatioP50:          jr,
		JitterCILow:             jlo,
		JitterCIHigh:            jhi,
		JitterCalledImprovement: jv == "faster_uncalibrated_not_claimed" || jv == "improvement",
		RegressionVerdict:       rv,
		RegressionRatioP50:      rr,
		RegressionCILow:         rlo,
		RegressionCIHigh:        rhi,
		RegressionDetected:      rv == "clear_regression",
		BaselineNS:              append([]float64(nil), baseline...),
		JitterNS:                jitter,
		RegressionNS:            reg,
		Note:                    "Uncertain jitter must not be called an improvement. Clear 2x must be detected. No uncalibrated percentage pass gate.",
	}
	// JitterCalledImprovement is true only if we wrongly used a "claimed improvement" label.
	// faster_uncalibrated_not_claimed is NOT a claimed improvement.
	rep.JitterCalledImprovement = false
	if jv == "improvement" || jv == "pass_faster" {
		rep.JitterCalledImprovement = true
	}
	return rep
}

func noiseOK(n NoiseReport) error {
	if n.JitterCalledImprovement {
		return fmt.Errorf("jitter was called an improvement")
	}
	if n.JitterVerdict == "clear_regression" {
		return fmt.Errorf("jitter misclassified as clear regression")
	}
	if !n.RegressionDetected {
		return fmt.Errorf("clear 2x regression not detected (verdict=%s ratio=%.3f ci=[%.3f,%.3f])",
			n.RegressionVerdict, n.RegressionRatioP50, n.RegressionCILow, n.RegressionCIHigh)
	}
	return nil
}

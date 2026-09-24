package performance_baseline

import (
	"math"
	"math/rand"
	"sort"
)

func percentileInt(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	idx := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func meanStdev(vals []int64) (mean, stdev float64) {
	if len(vals) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range vals {
		sum += float64(v)
	}
	mean = sum / float64(len(vals))
	if len(vals) == 1 {
		return mean, 0
	}
	var ss float64
	for _, v := range vals {
		d := float64(v) - mean
		ss += d * d
	}
	stdev = math.Sqrt(ss / float64(len(vals)-1))
	return mean, stdev
}

func summarizeGroup(g *GroupResult) {
	var ns, b, al []int64
	var best int64
	for i, s := range g.Samples {
		if !s.OK || s.Role != "measured" {
			continue
		}
		ns = append(ns, s.NS)
		b = append(b, s.BOp)
		al = append(al, s.Allocs)
		if i == 0 || s.NS < best || best == 0 {
			if best == 0 || s.NS < best {
				best = s.NS
			}
		}
	}
	g.ValidRepeats = len(ns)
	if g.RepeatsRequested > 0 {
		g.Completeness = float64(g.ValidRepeats) / float64(g.RepeatsRequested)
	}
	g.BestTrialNS = best
	if len(ns) == 0 {
		return
	}
	sorted := append([]int64(nil), ns...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	sortedB := append([]int64(nil), b...)
	sort.Slice(sortedB, func(i, j int) bool { return sortedB[i] < sortedB[j] })
	sortedA := append([]int64(nil), al...)
	sort.Slice(sortedA, func(i, j int) bool { return sortedA[i] < sortedA[j] })
	g.Percentiles = Percentiles{
		P50NS:    percentileInt(sorted, 50),
		P90NS:    percentileInt(sorted, 90),
		P99NS:    percentileInt(sorted, 99),
		P50B:     percentileInt(sortedB, 50),
		P50Alloc: percentileInt(sortedA, 50),
	}
	g.MeanNS, g.StdevNS = meanStdev(ns)
	mb, _ := meanStdev(b)
	ma, _ := meanStdev(al)
	g.MeanBOp = mb
	g.MeanAllocs = ma
	if g.MeanNS > 0 && g.StdevNS/g.MeanNS > 0.25 {
		g.Noisy = true
		g.NoisyNote = "stdev/mean > 0.25; do not treat mean shifts as improvements"
	}
	var rssMax int64
	var haveRSS bool
	for _, s := range g.Samples {
		if s.PeakRSSBytes != nil {
			if !haveRSS || *s.PeakRSSBytes > rssMax {
				rssMax = *s.PeakRSSBytes
				haveRSS = true
			}
		}
	}
	if haveRSS {
		v := rssMax
		g.PeakRSSBytes = &v
	}
}

func measuredNS(g GroupResult) []float64 {
	var out []float64
	for _, s := range g.Samples {
		if s.OK && s.Role == "measured" {
			out = append(out, float64(s.NS))
		}
	}
	return out
}

func median(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	c := append([]float64(nil), vals...)
	sort.Float64s(c)
	mid := len(c) / 2
	if len(c)%2 == 0 {
		return (c[mid-1] + c[mid]) / 2
	}
	return c[mid]
}

// compareDistributions returns a verdict that never treats uncertain data as an improvement.
func compareDistributions(baseline, candidate []float64, rng *rand.Rand) (ratio, lo, hi float64, verdict string) {
	if len(baseline) == 0 || len(candidate) == 0 {
		return 0, 0, 0, "invalid_input"
	}
	bMed := median(baseline)
	cMed := median(candidate)
	if bMed == 0 {
		return 0, 0, 0, "invalid_input"
	}
	ratio = cMed / bMed
	lo, hi = bootstrapMedianRatio(baseline, candidate, rng, 2000)
	if lo <= 1 && hi >= 1 {
		return ratio, lo, hi, "uncertain_not_improvement"
	}
	// Independent bootstrap of an exact 2x series is not a point mass at 2.0.
	// Require median ratio >= 1.8 and the CI entirely above 1.
	if ratio >= 1.8 && lo > 1 {
		return ratio, lo, hi, "clear_regression"
	}
	if lo > 1 {
		return ratio, lo, hi, "possible_regression_uncalibrated"
	}
	// candidate appears faster (hi < 1). Uncalibrated: not a claimed improvement.
	return ratio, lo, hi, "faster_uncalibrated_not_claimed"
}

func bootstrapMedianRatio(a, b []float64, rng *rand.Rand, n int) (lo, hi float64) {
	ratios := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		am := median(resample(a, rng))
		bm := median(resample(b, rng))
		if am == 0 {
			continue
		}
		ratios = append(ratios, bm/am)
	}
	if len(ratios) == 0 {
		return 0, 0
	}
	sort.Float64s(ratios)
	loIdx := int(0.025 * float64(len(ratios)))
	hiIdx := int(0.975 * float64(len(ratios)))
	if hiIdx >= len(ratios) {
		hiIdx = len(ratios) - 1
	}
	return ratios[loIdx], ratios[hiIdx]
}

func resample(vals []float64, rng *rand.Rand) []float64 {
	out := make([]float64, len(vals))
	for i := range out {
		out[i] = vals[rng.Intn(len(vals))]
	}
	return out
}

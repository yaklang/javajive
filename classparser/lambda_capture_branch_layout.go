package javaclassparser

import (
	"strconv"
	"strings"

	"github.com/yaklang/javajive/internal/workbudget"
)

// A fragment's lexical capture discovery is a partial transfer on an incoming
// already-discovered prefix q. It accepts exactly q >= required and returns
// max(q, highest+1). This upward-closed domain avoids subset enumeration or
// guessing which outer branches have already mentioned a capture.
type lambdaCaptureDiscovery struct {
	required int
	highest  int
}

func (a lambdaCaptureDiscovery) then(b lambdaCaptureDiscovery) lambdaCaptureDiscovery {
	required := a.required
	if a.highest+1 < b.required && b.required > required {
		required = b.required
	}
	highest := a.highest
	if b.highest > highest {
		highest = b.highest
	}
	return lambdaCaptureDiscovery{required: required, highest: highest}
}

func lambdaCaptureDiscoveryOf(source string, count, offset int, work *workbudget.Budget) (lambdaCaptureDiscovery, bool) {
	if count < 0 || count > 255-offset || offset < 0 || offset > 1 || len(source) > 1<<20 || !nativeProofWork(work, int64(len(source))+1) || work != nil && work.CheckAlloc(int64(count)*8+64) != nil {
		return lambdaCaptureDiscovery{}, false
	}
	positions := make([]int, count)
	for i := range positions {
		positions[i] = -1
	}
	next, highest := 0, -1
	for cursor := 0; cursor < len(source); {
		start := strings.IndexByte(source[cursor:], 0)
		if start < 0 {
			break
		}
		start += cursor + 1
		end := strings.IndexByte(source[start:], 0)
		if end < 0 {
			return lambdaCaptureDiscovery{}, false
		}
		end += start
		ordinal, prefix := strings.CutPrefix(source[start:end], "LCAP")
		index, err := strconv.Atoi(ordinal)
		if !prefix || err != nil || index < 0 || index >= count+offset || ordinal != strconv.Itoa(index) {
			return lambdaCaptureDiscovery{}, false
		}
		if index >= offset {
			index -= offset
			if positions[index] < 0 {
				positions[index] = next
				next++
			}
			if index > highest {
				highest = index
			}
		}
		cursor = end + 1
	}
	required := 0
	for rank := 0; rank <= highest; rank++ {
		// A missing rank must have been discovered outside this fragment.
		// An inverted adjacent pair similarly requires its lower rank to
		// have been discovered already. Above the last such obstruction,
		// every remaining rank occurs in the required ascending order.
		if positions[rank] < 0 || rank < highest && positions[rank+1] >= 0 && positions[rank] > positions[rank+1] {
			required = rank + 1
		}
	}
	return lambdaCaptureDiscovery{required: required, highest: highest}, true
}

func lambdaCaptureBranchSwap(left, right string, count, offset int, work *workbudget.Budget) bool {
	// The source renderer may omit an absent else. Exchange only two actual
	// arms; do not manufacture empty guards or additional capture reads.
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	a, ak := lambdaCaptureDiscoveryOf(left, count, offset, work)
	b, bk := lambdaCaptureDiscoveryOf(right, count, offset, work)
	if !ak || !bk {
		return false
	}
	// Both layouts discover the same set and have the same outgoing prefix.
	// The smaller required prefix dominates for every enclosing context.
	// Existing binary-layout obligations take precedence at the caller, and
	// the final complete-source capture validator still certifies publication.
	return b.then(a).required < a.then(b).required
}

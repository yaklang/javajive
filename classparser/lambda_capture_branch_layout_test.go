package javaclassparser

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func captureDiscoveryTestSource(word []int, offset int) string {
	var source strings.Builder
	source.WriteString("actualBody(")
	for _, rank := range word {
		source.WriteString("\x00LCAP" + strconv.Itoa(rank+offset) + "\x00;")
	}
	source.WriteString(");")
	return source.String()
}

func captureDiscoveryTestOracle(word []int, prefix int) (int, bool) {
	next := prefix
	for _, rank := range word {
		if rank < next {
			continue
		}
		if rank != next {
			return next, false
		}
		next++
	}
	return next, true
}

func TestNativeLambdaCaptureDiscoveryTransferMatchesIndependentPrefixOracle(t *testing.T) {
	words := [][]int{nil}
	frontier := [][]int{nil}
	for length := 1; length <= 4; length++ {
		var next [][]int
		for _, word := range frontier {
			for rank := 0; rank < 4; rank++ {
				candidate := append(append([]int(nil), word...), rank)
				next = append(next, candidate)
			}
		}
		words = append(words, next...)
		frontier = next
	}
	for _, offset := range []int{0, 1} {
		fragments := make([]lambdaCaptureDiscovery, len(words))
		for i, word := range words {
			fragment, known := lambdaCaptureDiscoveryOf(captureDiscoveryTestSource(word, offset), 4, offset, nil)
			if !known {
				t.Fatal("original capture word refused", word)
			}
			fragments[i] = fragment
			for prefix := 0; prefix <= 4; prefix++ {
				end, valid := captureDiscoveryTestOracle(word, prefix)
				wantEnd := prefix
				if fragment.highest+1 > wantEnd {
					wantEnd = fragment.highest + 1
				}
				if valid != (prefix >= fragment.required) || valid && end != wantEnd {
					t.Fatalf("word=%v prefix=%d transfer=%+v oracle=%d/%v", word, prefix, fragment, end, valid)
				}
			}
		}
		for i, left := range words {
			for j, right := range words {
				ab := append(append([]int(nil), left...), right...)
				ba := append(append([]int(nil), right...), left...)
				transfer := fragments[i].then(fragments[j])
				swap := lambdaCaptureBranchSwap(captureDiscoveryTestSource(left, offset), captureDiscoveryTestSource(right, offset), 4, offset, nil)
				for prefix := 0; prefix <= 4; prefix++ {
					end, valid := captureDiscoveryTestOracle(ab, prefix)
					if valid != (prefix >= transfer.required) || valid && end != max(prefix, transfer.highest+1) {
						t.Fatalf("composition %v then %v prefix%d", left, right, prefix)
					}
					_, reverse := captureDiscoveryTestOracle(ba, prefix)
					chosen := valid
					if swap {
						chosen = reverse
					}
					if (valid || reverse) && !chosen {
						t.Fatalf("layout lost a valid enclosing prefix: %v/%v prefix=%d swap=%v", left, right, prefix, swap)
					}
				}
			}
		}
	}
}

func TestNativeLambdaCaptureBranchLayoutRejectsUnknownOrUnbudgetedTokens(t *testing.T) {
	left, right := captureDiscoveryTestSource([]int{2, 1}, 0), captureDiscoveryTestSource([]int{1}, 0)
	for _, kind := range []string{"original", "absent else", "absent if", "unknown marker", "unterminated", "negative ordinal", "noncanonical ordinal", "out of domain", "too many captures", "invalid receiver", "oversize", "work", "memory", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			a, b := left, right
			count, offset := 3, 0
			var work *workbudget.Budget
			switch kind {
			case "absent else":
				b = " \n\t"
			case "absent if":
				a = ""
			case "unknown marker":
				a = "\x00OTHER1\x00"
			case "unterminated":
				a = "\x00LCAP1"
			case "negative ordinal":
				a = "\x00LCAP-1\x00"
			case "noncanonical ordinal":
				a = "\x00LCAP01\x00"
			case "out of domain":
				a = "\x00LCAP3\x00"
			case "too many captures":
				count = 256
			case "invalid receiver":
				offset = 2
			case "oversize":
				a = strings.Repeat(" ", 1<<20) + left
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := lambdaCaptureBranchSwap(a, b, count, offset, work); got != (kind == "original") {
				t.Fatalf("bounded original layout=%v", got)
			}
		})
	}
}

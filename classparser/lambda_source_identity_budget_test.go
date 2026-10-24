package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialLambdaArrowStorageFollowsActualOccurrences(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		count        int
		known, limit bool
	}{
		{"absent", "class Plain { int read() { return 7; } }", 0, true, false},
		{"quoted and commented", "\"->\"; /* -> */ // ->", 0, true, false},
		{"one actual arrow", "v -> v", 1, true, false},
		{"second actual arrow exceeds storage", "v -> v; w -> w", 0, false, true},
		{"invalid source still inspected", "/* unfinished", 0, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			work := workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1, MaxGraphScans: 1024})
			positions, known := nativeLambdaSourceArrows(tc.source, work)
			if known != tc.known || len(positions) != tc.count || (work.Err() != nil) != tc.limit {
				t.Fatal("arrow inventory or storage guard", positions, known, work.Err())
			}
			if tc.count == 1 && positions[0] != 2 {
				t.Fatal("lost physical source occurrence", positions)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	work := workbudget.New(ctx, workbudget.Limits{})
	if _, known := nativeLambdaSourceArrows("class Plain {}", work); known || work.Err() == nil {
		t.Fatal("absent arrows concealed cancellation")
	}
}

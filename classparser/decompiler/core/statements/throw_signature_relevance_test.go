package statements

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

// No generic throws clause can restore a method formal. A linear relevance
// scan must remain charged, but unrelated parameters/bounds need no type AST.
func TestErasedThrowableViewSkipsIrrelevantGenericThrowsWithinChargedLinearWork(t *testing.T) {
	for _, sig := range []string{
		"<T:Ljava/lang/Object;>()V",
		"<Float:Ljava/lang/Object;Double:Ljava/lang/Object;java:Ljava/lang/Object;>(Z)D",
		"<T:Ljava/lang/Object;>([Ljava/util/List<+[TT;>;)Ljava/util/List<TT;>;",
		"<E:Ljava/lang/Throwable;>(TE;)V^Ljava/lang/Throwable;",
		"<E:Ljava/lang/Throwable;>(TE;)TE;",
		"<T::Ljava/io/Serializable;:Ljava/lang/Comparable<TT;>;>(TT;)TT;",
	} {
		t.Run(sig, func(t *testing.T) {
			n := int64(len(sig) + 1)
			work := workbudget.New(nil, workbudget.Limits{MaxGraphScans: n * 400})
			ctx := &class_context.ClassContext{CurrentMethodSig: sig, TypeParams: []string{"E", "T", "Float", "Double", "java"}, Work: work}
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.AssertionError"))
			for i := 0; i < 400; i++ {
				if _, ok := erasedThrowableTypeVariableView(ctx, ref); ok {
					t.Fatal("irrelevant signature licensed a cast")
				}
			}
			if work.Err() != nil {
				t.Fatal(work.Err())
			}
			if got := work.Used(workbudget.CounterGraphScans); got != n*400 {
				t.Fatalf("linear work %d want %d", got, n*400)
			}
		})
	}
	// A necessary lexical marker is not sufficient proof. Full parsing and
	// resource checks still reject malformed, multiply thrown and long inputs.
	for _, sig := range []string{"<E:Ljava/lang/Throwable;>()V^TE;garbage", "<E:Ljava/lang/Throwable;>()V^TE;^TE;", "<E:Ljava/lang/Throwable;>()V^TE;" + strings.Repeat("X", 4096)} {
		ctx := &class_context.ClassContext{CurrentMethodSig: sig, TypeParams: []string{"E"}}
		ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Throwable"))
		if _, ok := erasedThrowableTypeVariableView(ctx, ref); ok {
			t.Fatal("invalid signature licensed a cast")
		}
	}
}

func TestErasedThrowableRelevanceScanRetainsBudgetAndCancellationFailure(t *testing.T) {
	sig := "<T:Ljava/lang/Object;>(TT;)TT;"
	for _, variant := range []string{"budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			c := context.Background()
			limits := workbudget.Limits{MaxGraphScans: int64(len(sig))}
			if variant == "canceled" {
				var cancel context.CancelFunc
				c, cancel = context.WithCancel(c)
				cancel()
				limits = workbudget.Limits{}
			}
			work := workbudget.New(c, limits)
			ctx := &class_context.ClassContext{CurrentMethodSig: sig, TypeParams: []string{"T"}, Work: work}
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.AssertionError"))
			if _, ok := erasedThrowableTypeVariableView(ctx, ref); ok || work.Err() == nil {
				t.Fatal("lost charged relevance-scan failure")
			}
		})
	}
}

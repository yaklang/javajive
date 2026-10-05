package javaclassparser

import (
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialConstructorLoopInvariantJoinPreservesAliasAndScalarUpperBounds(t *testing.T) {
	known := func(n int32) constructorEffectValue {
		return constructorEffectValue{kind: 'I', knownInt: true, intWord: n}
	}
	for _, a := range []int32{-2147483648, -1, 0, 7, 2147483647} {
		for _, b := range []int32{-2147483648, -1, 0, 7, 2147483647} {
			x := &constructorEffectLoopFrame{initialized: true, locals: []constructorEffectValue{{kind: 'L', receiver: true}, known(a)}, stack: []constructorEffectValue{{kind: 'J'}}}
			y := []constructorEffectValue{{kind: 'L', receiver: true}, known(b)}
			before := append([]constructorEffectValue(nil), x.locals...)
			joined, changed, ok := constructorEffectLoopJoin(x, y, x.stack, true)
			if !ok || changed != (a != b) || joined.locals[0] != x.locals[0] || joined.locals[1].knownInt != (a == b) || a == b && joined.locals[1].intWord != a || !reflect.DeepEqual(before, x.locals) {
				t.Fatalf("lost inductive upper bound or mutated evidence: %d/%d", a, b)
			}
			reverse, _, ok := constructorEffectLoopJoin(&constructorEffectLoopFrame{initialized: true, locals: y, stack: x.stack}, x.locals, x.stack, true)
			if !ok || !reflect.DeepEqual(joined, reverse) {
				t.Fatal("scalar join depends on predecessor order")
			}
			again, change, ok := constructorEffectLoopJoin(joined, y, joined.stack, true)
			if !ok || change || !reflect.DeepEqual(joined, again) {
				t.Fatal("joined invariant did not contain original incoming state")
			}
		}
	}
	for _, variant := range []string{"receiver alias", "local category", "stack category", "local count", "stack count", "initialization", "original allocation", "noninteger constant"} {
		t.Run(variant, func(t *testing.T) {
			prior := &constructorEffectLoopFrame{initialized: true, locals: []constructorEffectValue{{kind: 'L', receiver: true}}, stack: []constructorEffectValue{{kind: 'I'}}}
			locals := append([]constructorEffectValue(nil), prior.locals...)
			stack := append([]constructorEffectValue(nil), prior.stack...)
			init := true
			switch variant {
			case "receiver alias":
				locals[0].receiver = false
			case "local category":
				locals[0].kind = 'J'
			case "stack category":
				stack[0].kind = 'J'
			case "local count":
				locals = nil
			case "stack count":
				stack = nil
			case "initialization":
				init = false
			case "original allocation":
				locals[0].allocation = 17
			case "noninteger constant":
				locals[0].knownInt = true
			}
			if _, _, ok := constructorEffectLoopJoin(prior, locals, stack, init); ok {
				t.Fatal("unsafe cyclic frame was certified")
			}
		})
	}
}
func TestAdversarialConstructorLoopEffectsRecheckEveryObservationPath(t *testing.T) {
	const source = `class InvariantSilent{int value;InvariantSilent(int n){for(int i=0;i<n;i++)value+=i;}}
 class InvariantRead{Object capture,observed;InvariantRead(int n){for(int i=0;i<n;i++)if(i==2)observed=capture;}}
 class InvariantPublish{static Object saved;InvariantPublish(int n){for(int i=0;i<n;i++)if(i==2)saved=this;}}
 class InvariantAlias{static Object saved;InvariantAlias(int n){Object alias=null;for(int i=0;i<n;i++)if(i==2)alias=this;saved=alias;}}
 class InvariantOverwrite{int capture;InvariantOverwrite(int n){for(int i=0;i<n;i++)capture=i;}}
 class InvariantDivision{int value;InvariantDivision(int n){for(int i=0;i<n;i++)value+=7/n;}}
 class InvariantWide{long value;InvariantWide(int n){long total=0;for(int i=0;i<n;i++)total+=i;value=total;}}
 `
	files := nativeCompileClasses(t, source)
	root, e := Parse(files["InvariantSilent.class"])
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		name, field string
		safe        bool
	}{{"InvariantSilent", "", true}, {"InvariantWide", "", true}, {"InvariantRead", "capture", false}, {"InvariantPublish", "", false}, {"InvariantAlias", "", false}, {"InvariantOverwrite", "capture", false}, {"InvariantDivision", "", false}} {
		t.Run(tc.name, func(t *testing.T) {
			d := &ClassObjectDumper{obj: root, foldSiblingResolver: func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }, FuncCtx: &class_context.ClassContext{}}
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			writes := map[string]bool{}
			if tc.field != "" {
				desc := "I"
				if tc.name == "InvariantRead" {
					desc = "Ljava/lang/Object;"
				}
				writes[tc.name+"\x00"+tc.field+"\x00"+desc] = true
			}
			remaining := 512
			got := d.constructorChainDoesNotObserve(tc.name, "(I)V", writes, map[string]bool{}, &remaining, 0)
			if got != tc.safe {
				t.Fatalf("effect invariant=%v, remaining=%d", got, remaining)
			}
		})
	}
	for _, variant := range []string{"chain work", "shared graph", "memory"} {
		t.Run(variant, func(t *testing.T) {
			remaining := 512
			d := &ClassObjectDumper{obj: root, foldSiblingResolver: func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }, FuncCtx: &class_context.ClassContext{}}
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			switch variant {
			case "chain work":
				remaining = 1
			case "shared graph":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			if d.constructorChainDoesNotObserve("InvariantSilent", "(I)V", map[string]bool{}, map[string]bool{}, &remaining, 0) {
				t.Fatal("cyclic proof bypassed request resource bounds")
			}
		})
	}
}

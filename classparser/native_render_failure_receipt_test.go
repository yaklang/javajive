package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"testing"
)

func TestNativeRenderReceiptCacheAndMethodScope(t *testing.T) {
	files := nativeCompileClasses(t, `class ReceiptOwner { String good(){return "empty slot value";} }`)
	obj, e := Parse(append([]byte(nil), files["ReceiptOwner.class"]...))
	if e != nil {
		t.Fatal(e)
	}
	d := NewClassObjectDumper(obj)
	if _, e = d.DumpClass(); e != nil {
		t.Fatal(e)
	}
	parent := &class_context.SourceRenderState{}
	d.FuncCtx.SourceRendering = parent
	for _, mode := range []string{"valid literal", "incomplete cached body", "malformed cached body"} {
		t.Run(mode, func(t *testing.T) {
			*parent = class_context.SourceRenderState{}
			key := "name:good,desc:()Ljava/lang/String;"
			body := d.dumpedMethodsSet[key]
			if body == nil {
				t.Fatal("expected original cached body")
			}
			body.incompleteValue = mode == "incomplete cached body"
			body.malformedTry = mode == "malformed cached body"
			if _, e := d.DumpMethod("good", "()Ljava/lang/String;"); e != nil {
				t.Fatal(e)
			}
			if parent.IncompleteValue != (mode == "incomplete cached body") || parent.MalformedTry != (mode == "malformed cached body") {
				t.Fatalf("cached status lost: %+v", parent)
			}
			if d.FuncCtx.SourceRendering != parent {
				t.Fatal("cached call replaced caller scope")
			}
		})
	}
	delete(d.dumpedMethodsSet, "name:good,desc:()Ljava/lang/String;")
	*parent = class_context.SourceRenderState{}
	if body, e := d.DumpMethod("good", "()Ljava/lang/String;"); e != nil || body.incompleteValue || body.malformedTry || body.isStub {
		t.Fatalf("valid new rendering: %+v %v", body, e)
	}
	if d.FuncCtx.SourceRendering != parent || parent.IncompleteValue || parent.MalformedTry {
		t.Fatal("new method leaked a receipt")
	}
	if _, e := d.safeDumpMethod("absent", "()V"); e == nil || d.FuncCtx.SourceRendering != parent {
		t.Fatal("failed method did not restore caller scope")
	}
}

func TestNativeRenderReceiptRetryRollbackPreservesActiveIdentity(t *testing.T) {
	state := &class_context.SourceRenderState{}
	d := NewClassObjectDumper(&ClassObject{})
	d.FuncCtx = &class_context.ClassContext{SourceRendering: state}
	rollback := d.snapshotRetryState()
	state.IncompleteValue = true
	state.MalformedTry = true
	d.sourceDegraded = true
	rollback()
	if d.FuncCtx.SourceRendering != state || state.IncompleteValue || state.MalformedTry || d.sourceDegraded {
		t.Fatal("declined retry leaked status or replaced active receipt")
	}
	values.IncompleteSourceValue(d.FuncCtx)
	if !state.IncompleteValue {
		t.Fatal("later emitted failure became detached from method receipt")
	}
}

func TestNativeRenderReceiptProtectsNoCatchAndStubPublication(t *testing.T) {
	for _, text := range []string{values.EmptySlotValuePlaceholder, malformedTryNoCatchMarker, DecompileStubMarker} {
		if !canFlattenNoCatchTry(`return "`+text+`";`, false, &class_context.SourceRenderState{}) {
			t.Fatalf("literal data rejected: %q", text)
		}
	}
	for _, state := range []*class_context.SourceRenderState{{IncompleteValue: true}, {MalformedTry: true}} {
		if canFlattenNoCatchTry("return 7;", false, state) {
			t.Fatal("typed failure was hidden by ordinary source text")
		}
	}
	if canFlattenNoCatchTry("return 7;", true, nil) || canFlattenNoCatchTry("v = Exception;", false, nil) {
		t.Fatal("real invalid no-catch body accepted")
	}
	files := nativeCompileClasses(t, `class StubReceiptOwner { int number(){return 7;} }`)
	o, e := Parse(files["StubReceiptOwner.class"])
	if e != nil {
		t.Fatal(e)
	}
	d := NewClassObjectDumper(o)
	if _, e := d.DumpClass(); e != nil {
		t.Fatal(e)
	}
	var method *MemberInfo
	for _, m := range o.Methods {
		n, _ := sourceBridgeUTF8(o, m.NameIndex)
		if n == "number" {
			method = m
		}
	}
	stub := d.dumpStubMethod(method, "number", "()I", "forced failed reconstruction")
	if stub == nil || !stub.isStub || !d.sourceDegraded {
		t.Fatal("actual fallback did not record class/method failure")
	}
}

func TestNativeRenderReceiptDegradesTypedCachedFailure(t *testing.T) {
	files := nativeCompileClasses(t, `class CachedReceiptOwner{int number(){return 7;}}`)
	for _, mode := range []string{"incomplete value", "malformed try"} {
		t.Run(mode, func(t *testing.T) {
			o, e := Parse(append([]byte(nil), files["CachedReceiptOwner.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			d := NewClassObjectDumper(o)
			if _, e = d.DumpClass(); e != nil {
				t.Fatal(e)
			}
			key := "name:number,desc:()I"
			body := d.dumpedMethodsSet[key]
			if body == nil {
				t.Fatal("missing original body")
			}
			body.incompleteValue = mode == "incomplete value"
			body.malformedTry = mode == "malformed try"
			d.aggressiveRetried[key] = true
			methods, e := d.DumpMethods()
			if e != nil {
				t.Fatal(e)
			}
			found := false
			for _, m := range methods {
				if m.methodName == "number" {
					found = m.isStub
				}
			}
			if !found || !d.sourceDegraded {
				t.Fatal("typed failed body was accepted because its source contains no marker")
			}
		})
	}
}

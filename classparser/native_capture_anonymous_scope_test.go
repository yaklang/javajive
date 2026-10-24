package javaclassparser

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeCapturedAnonymousLexicalContextRequiresPhysicalBindings(t *testing.T) {
	files := nativeCompileClasses(t, capturedAnonymousSupertypeShape("class-variable"))
	for _, mode := range []string{"original", "renderer hint disabled", "missing visible formal", "raw source formal", "wrong current method", "wrong current descriptor", "missing group", "failed group", "foreign group owner", "missing producer", "foreign producer identity", "missing owner row", "missing enclosing method", "duplicate enclosing method", "different Signature", "missing reference", "foreign reference", "missing class Signature", "static cut", "changed original bound", "work", "memory", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			owner, _ := Parse(files["ProjectionOwner.class"])
			prepared := z.prepareNativeMemberFamily(owner, snapshotJDECEnv())
			if prepared == nil || prepared.family == nil || prepared.family.anonymous == nil {
				t.Fatal("original lexical family")
			}
			d := z.nativeMemberReader(owner)
			d.nativeMemberRoot, d.nativeAnonymousRoot = prepared.family, prepared.family.anonymous
			producer := d.nativeAnonymousRoot.children["ProjectionOwner$1"]
			if producer == nil {
				t.Fatal("original producer")
			}
			classSig, _ := nativeOriginalClassSignature(owner, nil)
			d.FuncCtx = &class_context.ClassContext{ClassName: owner.GetClassName(), FunctionName: "make", CurrentMethodDesc: "(Ljava/lang/CharSequence;)Ljava/util/Iterator;", ClassSig: classSig, TypeParams: []string{"E"}, ClassTypeParams: []string{"E"}}
			var method *MemberInfo
			for _, m := range owner.Methods {
				n, _ := sourceBridgeUTF8(owner, m.NameIndex)
				if n == "make" {
					method = m
				}
			}
			if method == nil {
				t.Fatal("original owning method")
			}
			d.FuncCtx.CurrentMethodSig, _ = nativeMethodLocalOriginalSignature(owner, method.Attributes, nil)
			signature, _ := nativeOriginalClassSignature(producer.object, nil)
			_, refs, _ := types.SignatureTypeVariableReferences(signature)
			switch mode {
			case "renderer hint disabled":
				d.FuncCtx.CurrentMethodSig = ""
			case "missing visible formal":
				d.FuncCtx.TypeParams = nil
			case "raw source formal":
				d.FuncCtx.RawEraseTypeVars = map[string]bool{"E": true}
			case "wrong current method":
				d.FuncCtx.FunctionName = "other"
			case "wrong current descriptor":
				d.FuncCtx.CurrentMethodDesc = "()Ljava/util/Iterator;"
			case "missing group":
				d.nativeAnonymousRoot = nil
			case "failed group":
				d.nativeAnonymousRoot.failed = true
			case "foreign group owner":
				d.nativeAnonymousRoot.owner = "Foreign"
			case "missing producer":
				delete(d.nativeAnonymousRoot.children, "ProjectionOwner$1")
			case "foreign producer identity":
				copy := *producer
				d.nativeAnonymousRoot.children["ProjectionOwner$1"] = &copy
			case "missing owner row":
				for _, a := range owner.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						var rows []*InnerClassInfo
						for _, row := range table.Classes {
							n, _ := sourceBridgeClassName(owner, row.InnerClassInfoIndex)
							if n != "ProjectionOwner$1" {
								rows = append(rows, row)
							}
						}
						table.Classes = rows
					}
				}
			case "missing enclosing method":
				var attrs []AttributeInfo
				for _, a := range producer.object.Attributes {
					if raw, ok := a.(*UnparsedAttribute); ok && raw.Name == "EnclosingMethod" {
						continue
					}
					attrs = append(attrs, a)
				}
				producer.object.Attributes = attrs
			case "duplicate enclosing method":
				for _, a := range producer.object.Attributes {
					if raw, ok := a.(*UnparsedAttribute); ok && raw.Name == "EnclosingMethod" {
						producer.object.Attributes = append(producer.object.Attributes, a)
						break
					}
				}
			case "different Signature":
				signature = "LProjectionCursor<Ljava/lang/String;>;"
			case "missing reference":
				refs = nil
			case "foreign reference":
				refs = []string{"X"}
			case "missing class Signature":
				var attrs []AttributeInfo
				for _, a := range owner.Attributes {
					if _, ok := a.(*SignatureAttribute); !ok {
						attrs = append(attrs, a)
					}
				}
				owner.Attributes = attrs
			case "static cut":
				method.AccessFlags |= 8
			case "changed original bound":
				for _, a := range owner.Attributes {
					if sig, ok := a.(*SignatureAttribute); ok {
						sig.SignatureIndex = sourceBridgePoolString(t, owner, "<E:Ljava/lang/Number;>Ljava/lang/Object;")
					}
				}
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := d.FuncCtx.CloneForRetry()
			got, known := d.nativeCapturedAnonymousLexicalContext(producer, signature, refs)
			good := mode == "original" || mode == "renderer hint disabled"
			if known != good {
				t.Fatalf("physical scope admitted=%v want=%v", known, good)
			}
			if good {
				if erased, k := values.SourceTypeErasure(types.NewJavaTypeVariable("E"), got); !k || erased != "Ljava/lang/CharSequence;" {
					t.Fatal("original bound lost or rebound", erased, k)
				}
				if got == d.FuncCtx {
					t.Fatal("live renderer used as proof context")
				}
			}
			if !reflect.DeepEqual(before, d.FuncCtx.CloneForRetry()) {
				t.Fatal("lexical proof mutated renderer state")
			}
		})
	}
}

// The source method's E shadows the class's Number-bound E. Optional renderer
// hints are absent; the physical method declaration still supplies CharSequence.
// A static method cuts the class scope while retaining its own formal binding.
func TestNativeCapturedAnonymousLexicalMethodShadowRetainsOriginalBound(t *testing.T) {
	for _, static := range []bool{false, true} {
		name := "instance"
		source := capturedAnonymousSupertypeShape("method-shadow")
		if static {
			name = "static"
			source = strings.Replace(source, "<E extends CharSequence> java.util.Iterator<E> make", "static <E extends CharSequence> java.util.Iterator<E> make", 1)
		}
		t.Run(name, func(t *testing.T) {
			files := nativeCompileClasses(t, source)
			z := nativeArchive(t, files)
			defer z.Close()
			owner, _ := Parse(files["ProjectionOwner.class"])
			prepared := z.prepareNativeMemberFamily(owner, snapshotJDECEnv())
			if prepared == nil || prepared.family == nil || prepared.family.anonymous == nil {
				t.Fatal("original lexical method family")
			}
			d := z.nativeMemberReader(owner)
			d.nativeMemberRoot, d.nativeAnonymousRoot = prepared.family, prepared.family.anonymous
			producer := d.nativeAnonymousRoot.children["ProjectionOwner$1"]
			if producer == nil {
				t.Fatal("original method-owned producer")
			}
			d.FuncCtx = &class_context.ClassContext{ClassName: owner.GetClassName(), FunctionName: "make", CurrentMethodDesc: "(Ljava/lang/CharSequence;)Ljava/util/Iterator;", TypeParams: []string{"E"}}
			d.FuncCtx.ClassSig, _ = nativeOriginalClassSignature(owner, nil)
			signature, _ := nativeOriginalClassSignature(producer.object, nil)
			_, refs, _ := types.SignatureTypeVariableReferences(signature)
			got, known := d.nativeCapturedAnonymousLexicalContext(producer, signature, refs)
			if !known {
				t.Fatal("original method binding refused")
			}
			if erased, known := values.SourceTypeErasure(types.NewJavaTypeVariable("E"), got); !known || erased != "Ljava/lang/CharSequence;" {
				t.Fatal("method's declaration borrowed the outer Number bound", erased, known)
			}
		})
	}
}

func TestNativeCapturedAnonymousOuterBoundKeepsItsDeclarationEnvironment(t *testing.T) {
	files := nativeCompileClasses(t, capturedAnonymousSupertypeShape("hidden-bound"))
	z := nativeArchive(t, files)
	defer z.Close()
	owner, _ := Parse(files["ProjectionOwner.class"])
	prepared := z.prepareNativeMemberFamily(owner, snapshotJDECEnv())
	if prepared == nil || prepared.family == nil || prepared.family.anonymous == nil {
		t.Fatal("original dependent lexical family")
	}
	d := z.nativeMemberReader(owner)
	d.nativeMemberRoot, d.nativeAnonymousRoot = prepared.family, prepared.family.anonymous
	producer := d.nativeAnonymousRoot.children["ProjectionOwner$1"]
	if producer == nil {
		t.Fatal("original dependent producer")
	}
	d.FuncCtx = &class_context.ClassContext{ClassName: owner.GetClassName(), FunctionName: "make", CurrentMethodDesc: "(Ljava/lang/CharSequence;)Ljava/util/Iterator;", TypeParams: []string{"K", "E"}}
	d.FuncCtx.ClassSig, _ = nativeOriginalClassSignature(owner, nil)
	signature, _ := nativeOriginalClassSignature(producer.object, nil)
	_, refs, _ := types.SignatureTypeVariableReferences(signature)
	got, known := d.nativeCapturedAnonymousLexicalContext(producer, signature, refs)
	if !known {
		t.Fatal("using E must not require redeclaring its hidden K bound")
	}
	if erased, known := values.SourceTypeErasure(types.NewJavaTypeVariable("E"), got); !known || erased != "Ljava/lang/CharSequence;" {
		t.Fatal("outer E borrowed the method's Number-bound K", erased, known)
	}
	if got.IsTypeParam("K") {
		t.Fatal("an unused bound dependency became a new source variable")
	}
}

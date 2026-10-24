package javaclassparser

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/jdecenv"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeRetainedCaptureSourceRequiresActualStoreSnapshotAndCompletedParent(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for original local-lifetime metadata")
	}
	files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, lambdaCaptureLifetimeMetadataFixture, "UncapturedLambdaNames", "none")
	for _, change := range []string{"original", "missing source", "missing frame", "incomplete frame", "frame request", "source method", "source code", "source PC", "missing factory flag", "missing capture", "different capture", "lost STORE", "changed STORE RHS", "same public STORE target", "shallow STORE target", "lost snapshot", "changed snapshot RHS", "missing body factory", "duplicate body factory", "opaque source leaf", "work", "allocation", "canceled"} {
		t.Run(change, func(t *testing.T) {
			object, err := Parse(bytes.Clone(files["UncapturedLambdaNames.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(object)
			d.options = DecompileOptions{Mode: Precision, SourceCompiler: NativeJavac8, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}}
			d.foldSiblingResolver = func(name string) ([]byte, bool) { data, known := files[name+".class"]; return bytes.Clone(data), known }
			var source string
			err = jdecenv.Run(map[string]string{}, func() error { source, err = d.DumpClass(); return err })
			if err != nil || strings.Contains(source, DecompileStubMarker) || len(d.lambdaMethodSourceBodies) != 1 {
				t.Fatal("original completed lifetime source", err, source)
			}
			var b *lambdaParameterBinding
			for _, receipt := range d.lambdaMethodSourceBodies {
				b = receipt.body.lambdaParameterBinding
			}
			if b == nil || !b.capturedNamesClosed(nil) || b.captureBody == nil || len(b.captured) != 1 {
				t.Fatal("actual retained local and completed factory")
			}
			read := b.metadata.captures[0].local
			if read == nil || nativeLambdaLocalCaptureValue(b.metadata.captures[0], b.captured[0], b.metadata.factory.pc, 0, b.captureContext, nil) {
				t.Fatal("fixture must require a retained STORE rather than historical Val")
			}
			capture, known := nativeMemberEnclosingUnpack(b.captured[0], nil)
			ref, typed := capture.(*values.JavaRef)
			if !known || !typed {
				t.Fatal("actual capture snapshot")
			}
			var store, snapshot *statements.AssignStatement
			var walk func([]statements.Statement)
			walk = func(body []statements.Statement) {
				for _, st := range body {
					if assign, ok := st.(*statements.AssignStatement); ok {
						pc, slot, sealed := assign.OriginalLocalStore()
						if sealed && pc == read.storePC && slot == read.slot {
							if store != nil {
								t.Fatal("duplicate actual STORE")
							}
							store = assign
						}
						if left, ok := assign.LeftValue.(*values.JavaRef); ok && left.OriginalDynamicOperandDeclarationOf(ref, assign.JavaValue) {
							if snapshot != nil {
								t.Fatal("duplicate actual snapshot")
							}
							snapshot = assign
						}
					}
					_, children, known := nativeSourceNameChildren(st)
					if !known {
						t.Fatal("unknown original source node")
					}
					for _, child := range children {
						walk(child)
					}
				}
			}
			walk(b.captureBody.body)
			if store == nil || snapshot == nil {
				t.Fatal("actual original definition/snapshot pair")
			}
			before := d.obj.Bytes()
			var work *workbudget.Budget
			switch change {
			case "missing source":
				b.captureBody = nil
			case "missing frame":
				b.captureEmission = nil
			case "incomplete frame":
				b.captureEmission.complete = false
			case "frame request":
				b.captureEmission.request.serial++
			case "source method":
				b.captureBody.method = &MemberInfo{}
			case "source code":
				copy := *b.captureBody.code
				b.captureBody.code = &copy
			case "source PC":
				b.captureBody.factoryPC++
			case "missing factory flag":
				b.captureBody.hasFactoryPC = false
			case "missing capture":
				b.captureBody.values = nil
			case "different capture":
				b.captureBody.values[0] = values.NewJavaRef(ref.Id, ref.Val, ref.Type())
			case "lost STORE":
				store.HasOriginPC = false
			case "changed STORE RHS":
				store.JavaValue = values.JavaNull
			case "same public STORE target":
				original := store.LeftValue.(*values.JavaRef)
				other := values.NewJavaRef(original.Id, original.Val, original.Type())
				other.VarUid = original.VarUid
				store.LeftValue = other
			case "shallow STORE target":
				copy := *store.LeftValue.(*values.JavaRef)
				store.LeftValue = &copy
			case "lost snapshot":
				snapshot.HasOriginPC = false
			case "changed snapshot RHS":
				snapshot.JavaValue = values.JavaNull
			case "missing body factory":
				b.captureBody.body = []statements.Statement{&statements.SourceAnchorStatement{}}
			case "duplicate body factory":
				b.captureBody.body = append(b.captureBody.body, b.captureBody.body...)
			case "opaque source leaf":
				b.captureBody.body = append(b.captureBody.body, &statements.CustomStatement{ThrownValue: values.JavaNull})
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if known := b.capturedNamesClosed(work); known != (change == "original") {
				t.Fatalf("changed retained source acquired capture binding=%v", known)
			}
			if !bytes.Equal(before, d.obj.Bytes()) {
				t.Fatal("source binding query changed original class bytes")
			}
			if work != nil && work.Err() == nil {
				t.Fatal("bounded refusal must be sticky")
			}
		})
	}
}

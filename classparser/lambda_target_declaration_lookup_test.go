package javaclassparser

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeLambdaTargetSignaturesUseExactPlatformDeclarationsWithoutOwningThem(t *testing.T) {
	files := nativeCompileDebugClasses(t, `interface PlatformSignatureProbe<T>{T produce();}`, "none")
	probe, err := Parse(files["PlatformSignatureProbe.class"])
	if err != nil {
		t.Fatal(err)
	}
	for _, release := range []int{8, 9, 11, 16, 17, 21} {
		for _, owner := range []string{"java/util/concurrent/Callable", "java/util/function/Supplier", "java/util/function/Function", "java/util/function/IntFunction", "java/util/Comparator"} {
			t.Run(fmt.Sprintf("%s/%d", owner, release), func(t *testing.T) {
				d := NewClassObjectDumper(probe)
				d.options.TargetSourceVersion = release
				if d.buildSiblingClassSig() != nil {
					t.Fatal("platform lambda evidence broadened the ordinary-call provider")
				}
				before, known := jdkConstructorClassBytes(owner, release)
				metadata, declared := jdkInvocationMetadata(owner, release)
				if !known || !declared {
					t.Fatal("exact original platform declarations")
				}
				class, methods, known := d.buildLambdaTargetClassSig()(owner)
				if !known || class != metadata.Signature {
					t.Fatal("class formal/erasure identity changed", class, metadata.Signature)
				}
				for _, method := range metadata.Methods {
					if method.Name != "<init>" && methods[class_context.MethodDescKey(method.Name, method.Desc)] != method.Signature {
						t.Fatalf("descriptor-specific generic signature changed: %s%s", method.Name, method.Desc)
					}
				}
				after, known := jdkConstructorClassBytes(owner, release)
				if !known || !bytes.Equal(before, after) || d.nativeMemberRoot != nil || d.nativeAnonymousRoot != nil {
					t.Fatal("declaration lookup changed original bytes or enrolled source ownership")
				}
			})
		}
	}
}

func TestNativeLambdaTargetDeclarationLookupKeepsProfileIdentityAndResolverPrecedence(t *testing.T) {
	files := nativeCompileDebugClasses(t, `interface PlatformSignatureProbe<T>{T produce();}`, "none")
	raw, known := jdkConstructorClassBytes("java/util/concurrent/Callable", 8)
	if !known {
		t.Fatal("pinned original declaration")
	}
	for _, change := range []string{"original", "implicit original profile", "own declaration", "unknown profile", "unknown class", "explicit renamed formal", "archive renamed formal", "sibling renamed formal", "wrong supplied identity", "truncated supplied bytes", "known empty declaration", "unclaimed supplied bytes", "work", "canceled"} {
		t.Run(change, func(t *testing.T) {
			probe, err := Parse(bytes.Clone(files["PlatformSignatureProbe.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(probe)
			d.options.TargetSourceVersion = 8
			owner := "java/util/concurrent/Callable"
			data := bytes.Clone(raw)
			if strings.Contains(change, "renamed formal") {
				object, err := Parse(data)
				if err != nil {
					t.Fatal(err)
				}
				for _, constant := range object.ConstantPool {
					if text, ok := constant.(*ConstantUtf8Info); ok {
						text.Value = strings.NewReplacer("<V:", "<Q:", "TV;", "TQ;").Replace(text.Value)
					}
				}
				data = object.Bytes()
			}
			resolver := func(string) ([]byte, bool) { return data, true }
			switch change {
			case "implicit original profile":
				d.options.TargetSourceVersion = 0
			case "own declaration":
				owner = "PlatformSignatureProbe"
				d.declarationResolver = func(string) ([]byte, bool) { return raw, true }
			case "unknown profile":
				d.options.TargetSourceVersion = 10
			case "unknown class":
				owner = "unknown/NoPlatformClass"
			case "explicit renamed formal":
				d.declarationResolver = resolver
			case "archive renamed formal":
				d.archiveDeclarationResolver = resolver
			case "sibling renamed formal":
				d.foldSiblingResolver = resolver
			case "wrong supplied identity":
				d.declarationResolver = func(string) ([]byte, bool) { return files["PlatformSignatureProbe.class"], true }
			case "truncated supplied bytes":
				d.declarationResolver = func(string) ([]byte, bool) { return data[:len(data)/2], true }
			case "known empty declaration":
				d.declarationResolver = func(string) ([]byte, bool) { return nil, true }
			case "unclaimed supplied bytes":
				d.declarationResolver = func(string) ([]byte, bool) { return data, false }
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			class, methods, valid := d.buildLambdaTargetClassSig()(owner)
			want := change == "original" || change == "implicit original profile" || change == "own declaration" || strings.Contains(change, "renamed formal")
			if valid != want {
				t.Fatalf("declaration known=%v want=%v", valid, want)
			}
			if strings.Contains(change, "renamed formal") && (class != "<Q:Ljava/lang/Object;>Ljava/lang/Object;" || methods[class_context.MethodDescKey("call", "()Ljava/lang/Object;")] != "()TQ;") {
				t.Fatal("explicit original signature was replaced by the platform default")
			}
			if change == "own declaration" && (class != "<T:Ljava/lang/Object;>Ljava/lang/Object;" || methods[class_context.MethodDescKey("produce", "()Ljava/lang/Object;")] != "()TT;") {
				t.Fatal("current class identity borrowed a dependency's same lookup")
			}
		})
	}
}

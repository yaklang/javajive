package javaclassparser

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialAnonymousLambdaBoundaryRequiresItsCompletedAllocation(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Skip("JAVA8_JAVAC required for original anonymous ownership")
	}
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, anonymousLambdaScopeFixture, "ArtifactLambdaOwner", "none", false)
	jar := filepath.Join(t.TempDir(), "original.jar")
	if err := os.WriteFile(jar, t23Zip(t, files), 0600); err != nil {
		t.Fatal(err)
	}
	artifact, err := NewJarSourceArtifactFromLocal(jar, 8, NativeJavac8, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer artifact.Close()
	root, err := Parse(bytes.Clone(files["ArtifactLambdaOwner.class"]))
	if err != nil {
		t.Fatal(err)
	}
	reader := artifact.jar.nativeMemberReader(root)
	family := reader.planNativeMemberFamily()
	if family == nil || !reader.planNativeMemberAnonymousScopes(family) || family.anonymous == nil {
		t.Fatal("original anonymous source family")
	}
	originalGroup := family.anonymous
	originalChild := originalGroup.children["ArtifactLambdaOwner$1"]
	if originalChild == nil {
		t.Fatal("original allocated class")
	}
	for _, variant := range []string{"control", "reference tokens", "argument comment", "argument quoted parentheses", "literal spoof", "literal only", "unrecorded", "foreign marker", "changed owner", "changed enclosing method", "detached child", "missing child", "failed group", "nil receipt", "unknown ordinal", "duplicate allocation", "ordinary block", "unobserved parent", "identifier lookalike", "prefix literal lookalike", "rollback", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			group, child := *originalGroup, *originalChild
			group.children = maps.Clone(originalGroup.children)
			group.children[child.object.GetClassName()] = &child
			d := NewClassObjectDumper(root)
			// Exercise the namespace projection only. The full renderer's body,
			// constructor and capture obligations are tested by the JVM MVP.
			if !d.recordLambdaAnonymousScope(&group, &child, "new Parent") {
				t.Fatal("original typed allocation boundary")
			}
			marker := fmt.Sprintf("/*jdec-owned-anonymous-ordinal:%d:%s*/", child.ordinal, group.owner)
			source := "LongUnaryOperator outer=(x)->x;return new Parent((y)->y,0L) " + marker + " {public void run(){LongUnaryOperator inner=(z)->z;}};"
			references := false
			wantKnown, wantTokens := true, 2
			switch variant {
			case "reference tokens":
				source = "LongUnaryOperator outer=Owner::first;return new Parent(Owner::second,0L) " + marker + " {public void run(){LongUnaryOperator inner=Owner::third;}};"
				references = true
			case "argument comment":
				source = strings.Replace(source, "(y)->y", "(y)->y /* ) -> ( */", 1)
			case "argument quoted parentheses":
				source = strings.Replace(source, "(y)->y", "(y)->{String text=\") new Parent(\";return y;}", 1)
			case "literal spoof":
				source = "String fake=\"new Parent() " + marker + " {x -> x;}\";" + source
			case "literal only":
				source = "String fake=\"new Parent() " + marker + " {x -> x;}\";LongUnaryOperator actual=(x)->x;"
				wantTokens = 1
			case "unrecorded":
				d.lambdaAnonymousScopes = nil
				wantTokens = 3
			case "foreign marker":
				source = strings.Replace(source, marker, "/*jdec-owned-anonymous-ordinal:1:Foreign*/", 1)
				wantTokens = 3
			case "changed owner":
				copy := *root
				d.obj = &copy
				wantKnown = false
			case "changed enclosing method":
				child.method += "different"
				wantKnown = false
			case "detached child":
				copy := child
				group.children[child.object.GetClassName()] = &copy
				wantKnown = false
			case "missing child":
				delete(group.children, child.object.GetClassName())
				wantKnown = false
			case "failed group":
				group.failed = true
				wantKnown = false
			case "nil receipt":
				d.lambdaAnonymousScopes[&child] = nil
				wantKnown = false
			case "unknown ordinal":
				source = strings.Replace(source, marker, "/*jdec-owned-anonymous-ordinal:99:ArtifactLambdaOwner*/", 1)
				wantKnown = false
			case "duplicate allocation":
				source += source
				wantKnown = false
			case "ordinary block":
				source = strings.Replace(source, "new Parent((y)->y,0L)", "if (ready())", 1)
				wantKnown = false
			case "unobserved parent":
				source = strings.Replace(source, "new Parent", "new Stranger", 1)
				wantKnown = false
			case "identifier lookalike":
				source = strings.Replace(source, "new Parent", "renew Parent", 1)
				wantKnown = false
			case "prefix literal lookalike":
				source = strings.Replace(source, "new Parent((y)->y,0L)", "method(\"new Parent((y)->y,0L)\")", 1)
				wantKnown = false
			case "rollback":
				before := d.lambdaAnonymousScopes[&child]
				rollback := d.snapshotRetryState()
				if !d.recordLambdaAnonymousScope(&group, &child, "new ArtifactLambdaOwner.Parent") || len(d.lambdaAnonymousScopes[&child].allocations) != 2 {
					t.Fatal("typed alternative spelling")
				}
				rollback()
				if d.lambdaAnonymousScopes[&child] != before || len(before.allocations) != 1 {
					t.Fatal("declined retry leaked mutable class scope state")
				}
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
				wantKnown = false
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				wantKnown = false
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
				wantKnown = false
			}
			got, known := d.lambdaOwnImplementationTokens(source, references)
			if known != wantKnown || known && len(got) != wantTokens {
				t.Fatalf("own class tokens=%v known=%v; want %d known=%v", got, known, wantTokens, wantKnown)
			}
			if known {
				all, original := nativeLambdaSourceImplementationTokens(source, nil, references)
				if !original || !reflect.DeepEqual(got, all[:wantTokens]) {
					t.Fatal("constructor argument positions or original source coordinates changed")
				}
			}
		})
	}
}

package javaclassparser

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

// The original parent executes a larger finite composition of already closed
// receiver operations. Neither the original parameter nor runtime loop counts
// are specialized. The original driver checks overflow, capture identity and
// all three abrupt exits before any candidate code is executed.
func TestAdversarialConstructorLargeClosedEffectsRoundTrip(t *testing.T) {
	for _, calls := range []int{16, 32, 64} {
		t.Run(fmt.Sprint(calls), func(t *testing.T) {
			f := strings.Replace(constructorUnknownAbruptCaptureFixture, "final int n;", "final int n;int checksum;private int step(int input){return input*31+7;}", 1)
			f = strings.Replace(f, "this.n=n;trace=trace*31+2;", "this.n=n;"+strings.Repeat("checksum+=step(n);", calls)+"trace=trace*31+2;", 1)
			f = strings.Replace(f, "AbruptParent.trace!=33", fmt.Sprintf("AbruptParent.trace!=33||c.checksum!=%d*(n*31+7)", calls), 1)
			f = strings.Replace(f, "{-3,-2,-1,0,7,70000}", "{-3,-2,-1,0,7,70000,Integer.MAX_VALUE}", 1)
			prefix := fmt.Sprintf("ClosedEffect%d", calls)
			f = strings.ReplaceAll(f, "Abrupt", prefix)
			const want = "-3:null:false\n-3:null:true\n-2:fresh:false\n-2:fresh:true\n-1:shared:false\n-1:shared:true\n0:ok:false\n0:ok:true\n7:ok:false\n7:ok:true\n70000:ok:false\n70000:ok:true\n2147483647:ok:false\n2147483647:ok:true\n"
			testConstructorUnknownInputAbruptCaptureRoundTrip(t, f, prefix, want)
		})
	}
}

func TestNativeConstructorLargeEffectProofRemainsBounded(t *testing.T) {
	files := nativeCompileClasses(t, `class BoundedEffectParent{BoundedEffectParent(){}}final class BoundedEffectChild extends BoundedEffectParent{}`)
	for _, variant := range []string{"small", "large original body", "graph budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			parentRaw := bytes.Clone(files["BoundedEffectParent.class"])
			if variant == "large original body" {
				parent, err := Parse(parentRaw)
				if err != nil {
					t.Fatal(err)
				}
				for _, method := range parent.Methods {
					name, _ := sourceBridgeUTF8(parent, method.NameIndex)
					if name != "<init>" {
						continue
					}
					for _, attr := range method.Attributes {
						if code, ok := attr.(*CodeAttribute); ok {
							// A valid finite straight-line original, unrelated to any
							// library or selected proof limit. The query must refuse
							// rather than treat exhausted analysis as a certificate.
							code.Code = append(append(bytes.Clone(code.Code[:len(code.Code)-1]), make([]byte, 20000)...), code.Code[len(code.Code)-1])
							code.Attributes = nil
							code.AttrLen = uint32(12 + len(code.Code))
						}
					}
				}
				parentRaw = parent.Bytes()
			}
			child, err := Parse(bytes.Clone(files["BoundedEffectChild.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(child)
			d.foldSiblingResolver = func(name string) ([]byte, bool) {
				if name == "BoundedEffectParent" {
					return bytes.Clone(parentRaw), true
				}
				raw, found := files[name+".class"]
				return bytes.Clone(raw), found
			}
			switch variant {
			case "graph budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if safe := d.constructorCaptureChainDoesNotObserve("BoundedEffectParent", "()V", map[string]bool{}); safe != (variant == "small") {
				t.Fatalf("certificate=%v", safe)
			}
		})
	}
}

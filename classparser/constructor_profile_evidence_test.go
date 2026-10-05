package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialConstructorProfileCertificateRechecksEveryOriginalProviderObservation(t *testing.T) {
	const source = `class ProfileParent {int sum;ProfileParent(int n){for(int i=0;i<n;i++)sum+=i;}}final class ProfileChild extends ProfileParent{ProfileChild(int n){super(n);}}`
	files := nativeCompileClasses(t, source)
	bad := nativeCompileClasses(t, `class ProfileParent {static Object saved;int sum;ProfileParent(int n){saved=this;for(int i=0;i<n;i++)sum+=i;}}final class ProfileChild extends ProfileParent{ProfileChild(int n){super(n);}}`)
	for _, variant := range []string{"stable", "changed last body", "missing last body", "graph budget", "read budget", "memory", "cancelled"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(files["ProfileChild.class"])
			if err != nil {
				t.Fatal(err)
			}
			reads := 0
			resolve := func(name string) ([]byte, bool) {
				if name == "ProfileParent" {
					reads++
					// Six retained profiles each observe the original parent for
					// finalization, then for its body. A cache must not suppress the
					// twelfth, load-bearing provider observation.
					if reads == 12 && variant == "changed last body" {
						return bad[name+".class"], true
					}
					if reads == 12 && variant == "missing last body" {
						return nil, false
					}
				}
				b, ok := files[name+".class"]
				return b, ok
			}
			d := &ClassObjectDumper{obj: obj, foldSiblingResolver: resolve}
			d.options.TargetSourceVersion = 8
			switch variant {
			case "graph budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "read budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxReadBytes: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "cancelled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := d.constructorCaptureChainDoesNotObserve("ProfileParent", "(I)V", map[string]bool{})
			if got != (variant == "stable") {
				t.Fatalf("certificate=%v reads=%d", got, reads)
			}
			if variant == "stable" && reads != 12 {
				t.Fatalf("original provider observation order changed: %d", reads)
			}
			if (variant == "changed last body" || variant == "missing last body") && reads != 12 {
				t.Fatalf("test did not reach the changed original lookup: %d", reads)
			}
		})
	}
}

func TestAdversarialConstructorProfileMissingRootRefuses(t *testing.T) {
	for _, d := range []*ClassObjectDumper{nil, {}} {
		if d.constructorCaptureChainDoesNotObserve("Parent", "()V", nil) {
			t.Fatal("missing original root obtained a movement certificate")
		}
	}
}

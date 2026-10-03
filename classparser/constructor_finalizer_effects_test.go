package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAdversarialFinalReceiverProofNeedsClosedOriginalFinalizerAncestry(t *testing.T) {
	javac, _ := t04Tools(t)
	source := finalReceiverEffectFixture + `
class NonFinalEffectsOwner {class Child extends FinalEffectsParent {Child(FinalEffectsBox box,int n){super(box,n);}}}
class ObservingEffectsParent extends FinalEffectsParent {ObservingEffectsParent(FinalEffectsBox box,int n){super(box,n);}protected void finalize(){throw new AssertionError("observer");}}
class InheritedFinalizerOwner {final class Child extends ObservingEffectsParent {Child(FinalEffectsBox box,int n){super(box,n);}}}
class OwnFinalizerOwner {final class Child extends FinalEffectsParent {Child(FinalEffectsBox box,int n){super(box,n);}protected void finalize(){throw new AssertionError("observer");}}}
class PublishingEffectsParent extends FinalEffectsParent {static Object escaped;PublishingEffectsParent(FinalEffectsBox box,int n){super(box,n);escaped=this;}}
class FinalPublicationOwner {final class Child extends PublishingEffectsParent {Child(FinalEffectsBox box,int n){super(box,n);}}}
`
	for _, debug := range []string{"-g", "-g:none"} {
		dir := t.TempDir()
		path := filepath.Join(dir, "FinalEffectsDriver.java")
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", dir, path).CombinedOutput(); err != nil {
			t.Fatalf("original finalizer controls: %v\n%s", err, out)
		}
		resolve := func(name string) ([]byte, bool) {
			raw, err := os.ReadFile(filepath.Join(dir, name+".class"))
			return raw, err == nil
		}
		for _, tc := range []struct {
			name, parent string
			closed, safe bool
		}{
			{"FinalEffectsOwner$Child", "FinalEffectsParent", true, true},
			{"NonFinalEffectsOwner$Child", "FinalEffectsParent", false, false},
			{"InheritedFinalizerOwner$Child", "ObservingEffectsParent", false, false},
			{"OwnFinalizerOwner$Child", "FinalEffectsParent", false, false},
			{"FinalPublicationOwner$Child", "PublishingEffectsParent", true, false},
		} {
			raw, ok := resolve(tc.name)
			if !ok {
				t.Fatal("original control missing", tc.name)
			}
			obj, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			d := &ClassObjectDumper{obj: obj, foldSiblingResolver: resolve}
			d.options.TargetSourceVersion = 8
			remaining := 512
			if got := d.constructorReceiverCannotObserveFinalization(&remaining); got != tc.closed {
				t.Fatalf("%s/%s: finalizer proof %v want %v", debug, tc.name, got, tc.closed)
			}
			writes := map[string]bool{tc.name + "\x00this$0\x00L" + tc.name[:len(tc.name)-len("$Child")] + ";": true}
			if got := d.constructorCaptureChainDoesNotObserve(tc.parent, "(LFinalEffectsBox;I)V", writes); got != tc.safe {
				t.Fatalf("%s/%s: capture motion %v want %v", debug, tc.name, got, tc.safe)
			}
			if tc.safe {
				d.foldSiblingResolver = func(name string) ([]byte, bool) {
					if name == tc.parent {
						return []byte{0, 1}, true
					}
					return resolve(name)
				}
				remaining = 512
				if d.constructorReceiverCannotObserveFinalization(&remaining) || d.constructorCaptureChainDoesNotObserve(tc.parent, "(LFinalEffectsBox;I)V", writes) {
					t.Fatal("invalid original ancestry was concealed")
				}
			}
		}
	}
}

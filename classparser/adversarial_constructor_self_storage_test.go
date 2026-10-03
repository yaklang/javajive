package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

// A self-reference retained inside the unpublished receiver is not a heap
// publication. Reading that field later must not manufacture a receiver-free
// alias; the conservative whole-chain proof rejects that combination.
func TestAdversarialConstructorSelfStorageAliasBoundaries(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	source := `class SelfStore {Object self;SelfStore(){self=this;}}
class SelfAlias {Object self;SelfAlias(){Object alias=this;self=alias;}}
class SelfRead extends SelfStore {static Object escaped;SelfRead(){escaped=self;}}
class SelfCall extends SelfStore {SelfCall(){System.identityHashCode(self);}}
class SelfCopied extends SelfStore {Object copy;SelfCopied(){copy=self;}}
class SelfNumeric extends SelfStore {int number;SelfNumeric(int n){number=(n<<2)^7;}}
class SelfBranches {Object self;SelfBranches(int n){if(n<0)self=this;}}
class SelfAcrossBranches {Object self;SelfAcrossBranches(int n){if(n<0)self=this;else System.identityHashCode(self);}}
class SelfVirtual {Object self;SelfVirtual(){self=this;observe();}void observe(){}}
class SelfDirectPublication {static Object escaped;SelfDirectPublication(){escaped=this;}}
class SelfVolatile {volatile Object self;SelfVolatile(){self=this;}}
`
	path := filepath.Join(dir, "SelfStorage.java")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	for _, debug := range []string{"-g", "-g:none"} {
		if out, err := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", dir, path).CombinedOutput(); err != nil {
			t.Fatalf("compile: %v\n%s", err, out)
		}
		resolve := func(name string) ([]byte, bool) {
			raw, err := os.ReadFile(filepath.Join(dir, name+".class"))
			return raw, err == nil
		}
		raw, _ := resolve("SelfStore")
		obj, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		d := &ClassObjectDumper{obj: obj, foldSiblingResolver: resolve, FuncCtx: &class_context.ClassContext{}}
		d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
		for _, tc := range []struct {
			name, desc string
			want       bool
		}{
			{"SelfStore", "()V", true}, {"SelfAlias", "()V", true}, {"SelfRead", "()V", false},
			{"SelfCall", "()V", false}, {"SelfCopied", "()V", false}, {"SelfNumeric", "(I)V", true},
			{"SelfBranches", "(I)V", true}, {"SelfAcrossBranches", "(I)V", false},
			{"SelfVirtual", "()V", false}, {"SelfDirectPublication", "()V", false}, {"SelfVolatile", "()V", false},
		} {
			t.Run(debug+"/"+tc.name, func(t *testing.T) {
				remaining := 512
				if got := d.constructorChainDoesNotObserve(tc.name, tc.desc, map[string]bool{}, map[string]bool{}, &remaining, 0); got != tc.want {
					t.Fatalf("safe=%v want=%v", got, tc.want)
				}
			})
		}
	}
}

func TestAdversarialConstructorSelfStoragePreservesLockAndCaptureIdentity(t *testing.T) {
	roundTripGenericFlowUnitsClasspath(t, "SelfStorageDriver", `
class SelfStorageBase {final Object lock;SelfStorageBase(){lock=this;}Object capture(){return null;}boolean locked(){return lock==this;}}
class SelfStorageParent extends SelfStorageBase {final long number;SelfStorageParent(long n){number=(n<<3)^19L;}}
class SelfStorageOwner {
 SelfStorageBase plain(final Object token){return new SelfStorageBase(){Object capture(){return token;}};}
 SelfStorageParent wide(final Object token,long n){return new SelfStorageParent(n){Object capture(){return token;}};}
}
class SelfStorageOracle {static void run(){SelfStorageOwner owner=new SelfStorageOwner();Object token=new Object();
 for(Object t:new Object[]{null,token})for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){
  SelfStorageBase plain=owner.plain(t);SelfStorageParent wide=owner.wide(t,n);
  if(!plain.locked()||!wide.locked()||plain.capture()!=t||wide.capture()!=t||wide.number!=((n<<3)^19L))throw new AssertionError("self-storage/capture identity");
  System.out.println(n+":"+(t==token)+":"+wide.number);
 }
}}
public class SelfStorageDriver {public static void main(String[]args){SelfStorageOracle.run();}}
`, nil, []string{"SelfStorageOwner", "SelfStorageOwner$1", "SelfStorageOwner$2"}, true, Precision, Compatibility, "legacy")
}

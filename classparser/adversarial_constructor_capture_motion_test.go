package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

const captureMotionFixture = `
class CaptureMotionBase {
 final int number;CaptureMotionBase(int value){number=value;}
 Object capture(){return null;}Object owner(){return null;}long wide(){return 0;}
}
class CaptureMotionObserver {
 final Object seen;CaptureMotionObserver(){seen=capture();}Object capture(){return null;}
}
class CaptureMotionSilent {CaptureMotionSilent(){}Object capture(){return null;}}
class CaptureMotionOwner {
 CaptureMotionBase make(final Object token,final long wide,int n){return new CaptureMotionBase(n){Object capture(){return token;}Object owner(){return CaptureMotionOwner.this;}long wide(){return wide;}};}
 CaptureMotionObserver observed(final Object token){return new CaptureMotionObserver(){Object capture(){return token;}};}
 CaptureMotionSilent plain(final Object token){return new CaptureMotionSilent(){Object capture(){return token;}};}
}
class CaptureMotionOracle {
 static void run(){CaptureMotionOwner owner=new CaptureMotionOwner();Object token=new Object();
  for(Object value:new Object[]{null,token})for(long wide:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE})for(int n:new int[]{-2,0,2}){
   CaptureMotionBase result=owner.make(value,wide,n);
   if(result.capture()!=value||result.owner()!=owner||result.wide()!=wide||result.number!=n)throw new AssertionError("capture identity/value");
   System.out.println(n+":"+wide+":"+(value==token));
  }
  if(owner.observed(token).seen!=token)throw new AssertionError("original super observes early capture");
  if(owner.plain(token).capture()!=token||owner.plain(null).capture()!=null)throw new AssertionError("silent no-arg capture");
 }
}
public class CaptureMotionDriver {public static void main(String[]args){CaptureMotionOracle.run();}}
`

func TestAdversarialConstructorCaptureMotionAcrossSilentChain(t *testing.T) {
	roundTripGenericFlowUnits(t, "CaptureMotionDriver", captureMotionFixture, nil, []string{"CaptureMotionOwner$1", "CaptureMotionOwner$3"}, Precision, Compatibility, "legacy")
}

func TestConstructorCaptureMotionRejectsObservingSuperclass(t *testing.T) {
	javac, java := t04Tools(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "CaptureMotionDriver.java")
	if err := os.WriteFile(file, []byte(captureMotionFixture), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-d", dir, file).CombinedOutput(); err != nil {
		t.Fatalf("original fixture: %v\n%s", err, out)
	}
	_ = t04RunJava(t, java, dir, "CaptureMotionDriver")
	raw, err := os.ReadFile(filepath.Join(dir, "CaptureMotionOwner$2.class"))
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(name string) ([]byte, bool) {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)+".class"))
		return b, err == nil
	}
	result, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision, TargetSourceVersion: 8, Resolve: resolve})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Source, DecompileStubMarker) {
		t.Fatal("unproved movement across virtual this observation was accepted", result.Source)
	}
	// Missing original superclass bytes cannot be replaced by declaration-only
	// optimism, even for the otherwise silent positive constructor.
	raw, err = os.ReadFile(filepath.Join(dir, "CaptureMotionOwner$1.class"))
	if err != nil {
		t.Fatal(err)
	}
	missing := func(name string) ([]byte, bool) {
		if name == "CaptureMotionBase" {
			return nil, false
		}
		return resolve(name)
	}
	result, err = DecompileWithOptions(raw, DecompileOptions{Mode: Precision, TargetSourceVersion: 8, Resolve: missing})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Source, DecompileStubMarker) {
		t.Fatal("missing superclass code accepted as silent", result.Source)
	}
	// Run the proof independently on the same original constructor bytes,
	// including a wrong identity provider and a resource/cycle rejection.
	child, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	dumper := &ClassObjectDumper{obj: child, ConstantPool: child.ConstantPool, foldSiblingResolver: resolve, FuncCtx: &class_context.ClassContext{}}
	dumper.FuncCtx.InvocationMetadata = dumper.buildInvocationMetadata()
	for _, kind := range []string{"silent", "observes", "missing", "wrong identity", "cycle", "budget", "depth"} {
		t.Run(kind, func(t *testing.T) {
			owner, desc := "CaptureMotionBase", "(I)V"
			remaining := 512
			depth := 0
			active := map[string]bool{}
			prior := dumper.foldSiblingResolver
			switch kind {
			case "observes":
				owner = "CaptureMotionObserver"
				desc = "()V"
			case "missing":
				owner = "AbsentConstructorOwner"
			case "wrong identity":
				dumper.foldSiblingResolver = func(string) ([]byte, bool) { return raw, true }
			case "cycle":
				active[owner+"\x00"+desc] = true
			case "budget":
				remaining = 0
			case "depth":
				depth = 17
			}
			defer func() { dumper.foldSiblingResolver = prior }()
			if got := dumper.constructorChainDoesNotObserve(owner, desc, map[string]bool{}, active, &remaining, depth); got != (kind == "silent") {
				t.Fatalf("receiver silence=%v", got)
			}
		})
	}
}

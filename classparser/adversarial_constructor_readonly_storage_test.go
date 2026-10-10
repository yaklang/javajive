package javaclassparser

import (
	"bytes"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Returning an independent original actual is different from reading a
// receiver field, even when both results have computational category L.
// The parent/driver remain original and check self, input and capture identity.
func TestAdversarialConstructorReadOnlyIndependentReturnAfterSelfStorage(t *testing.T) {
	for _, rename := range []string{"Storage", "Packet"} {
		for _, method := range []struct{ name, body string }{{"parameter", "return token;"}, {"null literal", "return null;"}} {
			t.Run(rename+"/"+method.name, func(t *testing.T) {
				source := `class StorageParent {final Object self,reference;StorageParent(Object token){self=this;reference=transport(-0.0,Long.MIN_VALUE,token);}private Object transport(double ignored,long gap,Object token){RETURN_BODY}Object capture(){return null;}boolean locked(){return self==this;}}
class StorageOwner {final Object captured;StorageOwner(Object t){captured=t;}final class Child extends StorageParent {Child(Object token){super(token);}Object capture(){return StorageOwner.this.captured;}}StorageParent make(Object token){return new Child(token);}}
class StorageOracle{static void run(){Object token=new Object(),capture=new Object();int rows=0;for(Object c:new Object[]{null,capture})for(Object value:new Object[]{null,token}){StorageParent p=new StorageOwner(c).make(value);if(!p.locked()||p.capture()!=c||p.reference!=EXPECTED_RETURN)throw new AssertionError("self/return/capture identity");rows++;}System.out.println(rows);}}
public class StorageDriver{public static void main(String[]args){StorageOracle.run();}}`
				expected := "value"
				if method.name == "null literal" {
					expected = "null"
				}
				source = strings.ReplaceAll(source, "RETURN_BODY", method.body)
				source = strings.ReplaceAll(source, "EXPECTED_RETURN", expected)
				source = strings.ReplaceAll(source, "Storage", rename)
				roundTripGenericFlowUnitsClasspath(t, rename+"Driver", source, nil, []string{rename + "Owner", rename + "Owner$Child"}, true, Precision, Compatibility, "legacy")
			})
		}
	}
}

// A real original constructor publishes a self-reference read through an own
// private getter. Proven field reads must still close the monotone alias gate;
// the JVM is the publication oracle, not the classifier's returned category.
func TestAdversarialConstructorReadOnlyHeapReadStillRejectsPublication(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"HeapReturnDriver.java": `final class HeapReturn {static Object saved;Object self;HeapReturn(){self=this;saved=read();}private Object read(){return self;}}public class HeapReturnDriver{public static void main(String[]args){HeapReturn p=new HeapReturn();if(HeapReturn.saved!=p)throw new AssertionError("original heap alias not published");System.out.println("published:true");}}`}, "none", "8")
	_, java := t04Tools(t)
	dir := t.TempDir()
	for name, b := range files {
		if e := os.WriteFile(filepath.Join(dir, name), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	out, e := exec.Command(java, "-Xverify:all", "-cp", dir, "HeapReturnDriver").CombinedOutput()
	if e != nil || string(out) != "published:true\n" {
		t.Fatalf("original JVM publication %v %q", e, out)
	}
	obj, e := Parse(bytes.Clone(files["HeapReturn.class"]))
	if e != nil {
		t.Fatal(e)
	}
	resolve := func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }
	d := &ClassObjectDumper{obj: obj, foldSiblingResolver: resolve}
	d.options.TargetSourceVersion = 8
	if d.constructorCaptureChainDoesNotObserve(obj.GetClassName(), "()V", map[string]bool{}) {
		t.Fatal("receiver heap read turned retained THIS into a receiver-free publication")
	}
}

func TestAdversarialConstructorReadOnlyStorageSourceBoundaries(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		files := nativeCompileSourceReleaseClasses(t, map[string]string{"StorageEvidence.java": `class StorageEvidence{Object self;private Object same(double ignored,long gap,Object token){return token;}private Object empty(){return null;}private Object read(){return self;}}`}, debug, "8")
		for _, variant := range []string{"parameter after self store", "null after self store", "field without self store", "field after self store", "moved field", "receiver actual", "uninitialized actual"} {
			t.Run(debug+"/"+variant, func(t *testing.T) {
				obj, e := Parse(bytes.Clone(files["StorageEvidence.class"]))
				if e != nil {
					t.Fatal(e)
				}
				name, desc := "same", "(DJLjava/lang/Object;)Ljava/lang/Object;"
				args := []constructorEffectValue{{kind: 'D'}, {kind: 'J'}, {kind: 'L'}}
				storage := &constructorSelfStorageProof{selfStored: true}
				writes := map[string]bool{}
				wantRead, wantAccepted := false, true
				switch variant {
				case "null after self store":
					name, desc, args = "empty", "()Ljava/lang/Object;", nil
				case "field without self store", "field after self store", "moved field":
					name, desc, args = "read", "()Ljava/lang/Object;", nil
					wantRead = true
					if variant == "field without self store" {
						storage.selfStored = false
					}
					if variant == "moved field" {
						writes[obj.GetClassName()+"\x00self\x00Ljava/lang/Object;"] = true
						wantRead = false
						wantAccepted = false
					}
				case "receiver actual":
					args[2].receiver = true
					wantAccepted = false
				case "uninitialized actual":
					args[2].allocation = 3
					wantAccepted = false
				}
				remaining := 512
				d := &ClassObjectDumper{obj: obj}
				value, accepted := d.constructorReceiverReadOnlyMethodWithStorage(obj, &values.JavaClassMember{Name: obj.GetClassName(), Member: name, Description: desc}, core.OP_INVOKEVIRTUAL, writes, &remaining, storage, args...)
				if accepted != wantAccepted || accepted && value.kind != 'L' || storage.referenceRead != wantRead {
					t.Fatalf("accepted=%v value=%+v storage=%+v want=%v/read%v", accepted, value, storage, wantAccepted, wantRead)
				}
				if storage.closed() == (storage.selfStored && wantRead) {
					t.Fatalf("self-store/read composition lost: %+v", storage)
				}
			})
		}
	}
}

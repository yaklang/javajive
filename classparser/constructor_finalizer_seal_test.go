package javaclassparser

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// An inherited final method seals dispatch even when the allocated class is
// extensible. Conversely, an empty override in an extensible class does not.
// The nonempty ancestor also distinguishes selecting the nearest override from
// requiring every same-signature method in the hierarchy to be empty.
const finalizerSealFixture = `
class SealState {static String trace="";}
class SealBox {final int value;SealBox(int n){value=n;}}
class SealOpaque {protected void finalize(){SealState.trace+="F";}}
class SealParent extends SealOpaque {
 int value;
 SealParent(SealBox box,int divisor){SealState.trace+="P";value=box.value/divisor;SealState.trace+="Q";}
 Object captured(){return null;}
 void finish(){finalize();}
}
class SealFinalMethodParent extends SealParent {
 SealFinalMethodParent(SealBox box,int divisor){super(box,divisor);}
 protected final void finalize(){}
}
class SealInheritedOwner {
 final Object token;SealInheritedOwner(Object t){token=t;}
 class Child extends SealFinalMethodParent {Child(SealBox box,int n){super(box,n);}Object captured(){return SealInheritedOwner.this.token;}}
 class Grandchild extends Child {Grandchild(SealBox box,int n){super(box,n);}}
 SealParent make(SealBox box,int n){return new Grandchild(box,n);}
}
class SealClassOwner {
 final Object token;SealClassOwner(Object t){token=t;}
 final class Child extends SealParent {Child(SealBox box,int n){super(box,n);}protected void finalize(){}Object captured(){return SealClassOwner.this.token;}}
 SealParent make(SealBox box,int n){return new Child(box,n);}
}
class SealOwnMethodOwner {
 final Object token;SealOwnMethodOwner(Object t){token=t;}
 class Child extends SealParent {Child(SealBox box,int n){super(box,n);}protected final void finalize(){}Object captured(){return SealOwnMethodOwner.this.token;}}
 SealParent make(SealBox box,int n){return new Child(box,n);}
}
class SealOpenOwner {class Child extends SealParent {Child(SealBox box,int n){super(box,n);}protected void finalize(){}}}
class SealObserverOwner {final class Child extends SealParent {Child(SealBox box,int n){super(box,n);}}}
class SealOverloadOwner {class Child extends SealParent {Child(SealBox box,int n){super(box,n);}protected final void finalize(int unused){}}}
class SealDriver {
 public static void main(String[]args)throws Exception{int rows=0;Object token=new Object();
  for(Object t:new Object[]{null,token})for(int word:new int[]{Integer.MIN_VALUE,-7,-1,0,1,7,Integer.MAX_VALUE})for(int divisor:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(boolean absent:new boolean[]{false,true})for(int kind=0;kind<3;kind++){
   SealState.trace="";SealBox box=absent?null:new SealBox(word);
   Object enclosing=kind==0?new SealInheritedOwner(t):kind==1?new SealClassOwner(t):new SealOwnMethodOwner(t);
   try{SealParent result=kind==0?((SealInheritedOwner)enclosing).make(box,divisor):kind==1?((SealClassOwner)enclosing).make(box,divisor):((SealOwnMethodOwner)enclosing).make(box,divisor);
    if(absent||divisor==0||result.value!=word/divisor||result.captured()!=t||!SealState.trace.equals("PQ"))throw new AssertionError("capture/value/order");
    for(Class<?> layer:kind==0?new Class<?>[]{result.getClass(),result.getClass().getSuperclass()}:new Class<?>[]{result.getClass()}){
     java.lang.reflect.Field capture=layer.getDeclaredField("this$0");capture.setAccessible(true);
     if(capture.get(result)!=enclosing||capture.getType()!=enclosing.getClass())throw new AssertionError("physical enclosing identity");
    }
    result.finish();if(!SealState.trace.equals("PQ"))throw new AssertionError("selected finalizer");
   }catch(NullPointerException ex){if(!absent||!SealState.trace.equals("P"))throw new AssertionError("null boundary",ex);}
   catch(ArithmeticException ex){if(absent||divisor!=0||!SealState.trace.equals("P"))throw new AssertionError("division boundary",ex);}
   rows++;
  }System.out.println(rows+":sealed:finalizer:capture:abrupt");
 }
}`

func TestConstructorFinalizerSealUsesNearestOriginalDeclaration(t *testing.T) {
	files := nativeCompileClasses(t, finalizerSealFixture)
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"SealInheritedOwner$Child", true},
		{"SealInheritedOwner$Grandchild", true},
		{"SealClassOwner$Child", true},
		{"SealOwnMethodOwner$Child", true},
		{"SealOpenOwner$Child", false},
		{"SealObserverOwner$Child", false},
		{"SealOverloadOwner$Child", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj, err := Parse(bytes.Clone(files[tc.name+".class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := &ClassObjectDumper{obj: obj, foldSiblingResolver: func(name string) ([]byte, bool) { r, ok := files[name+".class"]; return bytes.Clone(r), ok }}
			d.options.TargetSourceVersion = 8
			remaining := 512
			if got := d.constructorReceiverCannotObserveFinalization(&remaining); got != tc.want {
				t.Fatalf("original finalizer dispatch closed=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestConstructorFinalizerSealRejectsCounterfeitAndResourceEvidence(t *testing.T) {
	files := nativeCompileClasses(t, finalizerSealFixture)
	for _, variant := range []string{"original", "open dispatch", "synchronized", "static", "private", "native", "abstract", "observable body", "duplicate method", "missing code", "duplicate code", "handler", "missing ancestry", "invalid ancestry", "illegal final override", "budget", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			input := map[string][]byte{}
			for name, raw := range files {
				input[name] = bytes.Clone(raw)
			}
			parent, err := Parse(input["SealFinalMethodParent.class"])
			if err != nil {
				t.Fatal(err)
			}
			var method *MemberInfo
			var code *CodeAttribute
			for _, m := range parent.Methods {
				name, _ := sourceBridgeUTF8(parent, m.NameIndex)
				if name == "finalize" {
					method = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if method == nil || code == nil {
				t.Fatal("original finalizer absent")
			}
			remaining := 512
			var work *workbudget.Budget
			switch variant {
			case "open dispatch":
				method.AccessFlags &^= 0x0010
			case "synchronized":
				method.AccessFlags |= 0x0020
			case "static":
				method.AccessFlags |= 0x0008
			case "private":
				method.AccessFlags = 0x0012
			case "native":
				method.AccessFlags |= 0x0100
			case "abstract":
				method.AccessFlags |= 0x0400
			case "observable body":
				code.Code = []byte{core.OP_ACONST_NULL, core.OP_ATHROW}
				code.MaxStack = 1
			case "duplicate method":
				parent.Methods = append(parent.Methods, method)
			case "missing code":
				method.Attributes = nil
			case "duplicate code":
				method.Attributes = append(method.Attributes, code)
			case "handler":
				code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{})
			case "missing ancestry":
				delete(input, "SealParent.class")
			case "invalid ancestry":
				input["SealParent.class"] = []byte{0, 1}
			case "illegal final override":
				ancestor, e := Parse(input["SealOpaque.class"])
				if e != nil {
					t.Fatal(e)
				}
				for _, m := range ancestor.Methods {
					name, _ := sourceBridgeUTF8(ancestor, m.NameIndex)
					if name == "finalize" {
						m.AccessFlags |= 0x0010
					}
				}
				input["SealOpaque.class"] = ancestor.Bytes()
			case "budget":
				remaining = 0
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				nativeProofWork(work, 1)
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			input["SealFinalMethodParent.class"] = parent.Bytes()
			obj, e := Parse(bytes.Clone(input["SealInheritedOwner$Child.class"]))
			if e != nil {
				t.Fatal(e)
			}
			d := &ClassObjectDumper{obj: obj, Work: work, foldSiblingResolver: func(name string) ([]byte, bool) { r, ok := input[name+".class"]; return bytes.Clone(r), ok }}
			d.options.TargetSourceVersion = 8
			if got := d.constructorReceiverCannotObserveFinalization(&remaining); got != (variant == "original") {
				t.Fatalf("finalizer seal accepted=%v", got)
			}
		})
	}
}

func TestAdversarialSealedFinalizerCaptureMotionRetainsAbruptBoundaries(t *testing.T) {
	for _, renamed := range []bool{false, true} {
		fixture, prefix := finalizerSealFixture, "Seal"
		if renamed {
			fixture = strings.ReplaceAll(fixture, prefix, "Quiet")
			prefix = "Quiet"
		}
		t.Run(prefix, func(t *testing.T) {
			testIndependentFlatFinalizerFamily(t, fixture, prefix,
				[]string{prefix + "InheritedOwner", prefix + "InheritedOwner$Child", prefix + "InheritedOwner$Grandchild", prefix + "ClassOwner", prefix + "ClassOwner$Child", prefix + "OwnMethodOwner", prefix + "OwnMethodOwner$Child"})
		})
	}
}

// Exercise the flattened constructor boundary directly. Keep the driver and
// all parent implementations original, so rewriting an oracle or silently
// replacing the allocation factory cannot conceal a wrong capture or failure.
// Native-family metadata reconstruction has its own independent ABI tests;
// this test checks the fallback's behavior and complete physical class set.
func testIndependentFlatFinalizerFamily(t *testing.T, fixture, prefix string, owners []string) {
	t.Helper()
	javac, java := t04Tools(t)
	owned := map[string]bool{}
	for _, owner := range owners {
		owned[owner+".class"] = true
	}
	write := func(root, name string, raw []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, fixture, debug)
			original := t.TempDir()
			var names []string
			for name, raw := range files {
				write(original, name, raw)
				names = append(names, name)
			}
			slices.Sort(names)
			want := t04RunJava(t, java, original, prefix+"Driver")
			if want != "420:sealed:finalizer:capture:abrupt\n" {
				t.Fatalf("original oracle %q", want)
			}
			resolve := func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return bytes.Clone(raw), ok }
			for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(policy, func(t *testing.T) {
					if policy == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if policy == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
						t.Run(string(mode), func(t *testing.T) {
							out := t.TempDir()
							var paths []string
							for _, name := range names {
								if !owned[name] {
									write(out, name, files[name])
									continue
								}
								var source string
								var err error
								if mode == "legacy" {
									source, err = DecompileWithResolver(bytes.Clone(files[name]), resolve)
								} else {
									var result DecompileResult
									result, err = DecompileWithOptions(bytes.Clone(files[name]), DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
									source = result.Source
								}
								if err != nil || strings.Contains(source, DecompileStubMarker) {
									t.Fatalf("complete flat constructor %s: %v\n%s", name, err, source)
								}
								path := strings.TrimSuffix(name, ".class") + ".java"
								write(out, path, []byte(source))
								paths = append(paths, filepath.Join(out, path))
							}
							args := append([]string{"-proc:none", "--release", "8", "-cp", out, "-d", out}, paths...)
							if log, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
								t.Fatalf("flat rebuild: %v\n%s", err, log)
							}
							for _, name := range names {
								if owned[name] {
									if _, err := os.ReadFile(filepath.Join(out, name)); err != nil {
										t.Fatal("lost physical class", name, err)
									}
								}
							}
							if got := t04RunJava(t, java, out, prefix+"Driver"); got != want {
								t.Fatalf("unchanged oracle: %q want %q", got, want)
							}
						})
					}
				})
			}
		})
	}
}

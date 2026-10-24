package javaclassparser

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

const constructorStaticReadFixture = `import java.util.*;
class StaticReadValue{static final Object TOKEN=new Object();static volatile long WORD=31;}
class StaticReadParent{final Object value;final long word;StaticReadParent(){value=StaticReadValue.TOKEN;word=StaticReadValue.WORD;}}
class StaticReadPublishingParent{static Object exposed;StaticReadPublishingParent(){exposed=this;}}
class StaticReadScope{Object token;StaticReadScope(Object token){this.token=token;}Object make(){return new StaticReadParent(){protected final void finalize(){}Object capture(){return StaticReadScope.this.token;}};}}
class StaticReadDriver{public static void main(String[]args)throws Exception{int rows=0;for(Object token:new Object[]{null,new Object(),new String("identity"),new String[]{"array"}}){Object result=new StaticReadScope(token).make();java.lang.reflect.Method capture=result.getClass().getDeclaredMethod("capture");capture.setAccessible(true);if(capture.invoke(result)!=token)throw new AssertionError("captured identity");if(result instanceof StaticReadParent){StaticReadParent parent=(StaticReadParent)result;if(parent.value!=StaticReadValue.TOKEN||parent.word!=31)throw new AssertionError("static snapshot");}if(result instanceof List){((List)result).add(token);if(((List)result).get(0)!=token)throw new AssertionError("list identity");}if(!result.getClass().isAnonymousClass())throw new AssertionError("original scope");rows++;}System.out.println(rows+":static:identity:prefix:order");}}`

func TestAdversarialConstructorStaticReadsPreserveOriginalCaptureMotion(t *testing.T) {
	for _, shape := range []string{"reference-wide-static", "own-volatile", "array-list", "vector"} {
		t.Run(shape, func(t *testing.T) {
			fixture := constructorStaticReadFixture
			parent, desc := "StaticReadParent", "()V"
			if shape == "own-volatile" {
				fixture = strings.Replace(fixture, "final long word;", "volatile long word;", 1)
				fixture = strings.Replace(fixture, "word=StaticReadValue.WORD;", "word=StaticReadValue.WORD;word=word^17;word=word^17;", 1)
			}
			if shape == "array-list" {
				parent = "java/util/ArrayList"
				fixture = strings.Replace(fixture, "new StaticReadParent(){", "new ArrayList<Object>(){", 1)
			}
			if shape == "vector" {
				parent = "java/util/Vector"
				fixture = strings.Replace(fixture, "new StaticReadParent(){", "new Vector<Object>(){", 1)
			}
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					files := nativeCompileDebugClasses(t, fixture, debug)
					obj, err := Parse(files["StaticReadScope$1.class"])
					if err != nil {
						t.Fatal(err)
					}
					d := NewClassObjectDumper(obj)
					d.options.TargetSourceVersion = 8
					d.foldSiblingResolver = func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return raw, ok }
					d.FuncCtx = &class_context.ClassContext{}
					d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
					writes := map[string]bool{obj.GetClassName() + "\x00this$0\x00LStaticReadScope;": true}
					if !d.constructorCaptureChainDoesNotObserve(parent, desc, writes) {
						t.Fatal("original receiver-free static read lacks movement proof")
					}
					// Exercise the production flattened-constructor boundary as well as the
					// effect certificate. This check grants no source ABI normalization.
					source, err := d.DumpClass()
					if err != nil || strings.Contains(source, DecompileStubMarker) {
						t.Fatalf("production constructor source: %v\n%s", err, source)
					}
					var code *CodeAttribute
					for _, method := range obj.Methods {
						name, _ := sourceBridgeUTF8(obj, method.NameIndex)
						if name == "<init>" {
							for _, attribute := range method.Attributes {
								if c, ok := attribute.(*CodeAttribute); ok {
									code = c
								}
							}
						}
					}
					if code == nil || len(code.ExceptionTable) != 0 {
						t.Fatal("original straight-line constructor")
					}
					decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
					if decoder.ParseOpcode() != nil {
						t.Fatal("original constructor decode")
					}
					ops := constructorMotionOps(decoder)
					if len(ops) != 6 || ops[2].Instr.OpCode != core.OP_PUTFIELD || ops[4].Instr.OpCode != core.OP_INVOKESPECIAL || ops[5].Instr.OpCode != core.OP_RETURN {
						t.Fatal("original one capture and delegation packet")
					}
					member := constructorMotionMember(obj, ops[2], core.OP_PUTFIELD)
					call := constructorMotionMember(obj, ops[4], core.OP_INVOKESPECIAL)
					if member == nil || member.Name != obj.GetClassName() || member.Member != "this$0" || call == nil || call.Name != parent || call.Description != desc {
						t.Fatal("original packet ownership")
					}
					split, end := int(ops[3].CurrentOffset), int(ops[5].CurrentOffset)
					original := append([]byte(nil), code.Code...)
					moved := append([]byte(nil), original[split:end]...)
					moved = append(moved, original[:split]...)
					moved = append(moved, original[end:]...)
					code.Code = moved
					for _, side := range []string{"original", "moved"} {
						out := t.TempDir()
						for name, raw := range files {
							if side == "moved" && name == "StaticReadScope$1.class" {
								raw = obj.Bytes()
							}
							if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
								t.Fatal(err)
							}
						}
						_, java := t04Tools(t)
						if got := t04RunJava(t, java, out, "StaticReadDriver"); got != "4:static:identity:prefix:order\n" {
							t.Fatalf("%s original JVM=%q", side, got)
						}
					}
				})
			}
		})
	}
}

func TestAdversarialConstructorStaticReadCannotHideReceiverPublicationOrFinalization(t *testing.T) {
	for _, shape := range []string{"published-receiver", "observing-finalizer", "open-volatile-finalizer"} {
		t.Run(shape, func(t *testing.T) {
			fixture := constructorStaticReadFixture
			parent := "StaticReadParent"
			if shape == "published-receiver" {
				parent = "StaticReadPublishingParent"
				fixture = strings.Replace(fixture, "new StaticReadParent(){", "new StaticReadPublishingParent(){", 1)
			} else if shape == "open-volatile-finalizer" {
				fixture = strings.Replace(fixture, "final long word;", "volatile long word;", 1)
				fixture = strings.Replace(fixture, "protected final void finalize(){}", "", 1)
			} else {
				fixture = strings.Replace(fixture, "protected final void finalize(){}", "protected void finalize(){StaticReadPublishingParent.exposed=this;}", 1)
			}
			files := nativeCompileClasses(t, fixture)
			obj, err := Parse(files["StaticReadScope$1.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(obj)
			d.options.TargetSourceVersion = 8
			d.foldSiblingResolver = func(n string) ([]byte, bool) { raw, ok := files[n+".class"]; return raw, ok }
			d.FuncCtx = &class_context.ClassContext{}
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			writes := map[string]bool{obj.GetClassName() + "\x00this$0\x00LStaticReadScope;": true}
			if d.constructorCaptureChainDoesNotObserve(parent, "()V", writes) {
				t.Fatal("capture movement concealed an observable receiver")
			}
		})
	}
}

// The candidate classpath masks every rebuilt owner/child, so a retained original
// implementation cannot conceal a failed constructor rewrite.
const constructorOwnVolatileFixture = `
class VolatileParent {volatile Object value;volatile long word;volatile double floating;volatile int narrow;final Object snapshot;final long wideSnapshot;final double floatSnapshot;final int intSnapshot;VolatileParent(Object v,long w,double f,int n){value=v;word=w;floating=f;narrow=n;snapshot=value;wideSnapshot=word;floatSnapshot=floating;intSnapshot=narrow;}}
class VolatileOwner {final Object token;VolatileOwner(Object t){token=t;}final class Child extends VolatileParent {Child(Object v,long w,double f,int n){super(v,w,f,n);}Object capture(){return VolatileOwner.this.token;}}VolatileParent make(Object v,long w,double f,int n){return new Child(v,w,f,n);}}
class VolatileOracle {static void run(){int rows=0;Object identity=new Object();for(Object t:new Object[]{null,identity})for(Object v:new Object[]{null,identity,new String("value")})for(long w:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(double f:new double[]{-0.0,Double.NaN,Double.POSITIVE_INFINITY}){int n=(int)w;VolatileParent p=new VolatileOwner(t).make(v,w,f,n);if(((VolatileOwner.Child)p).capture()!=t||p.value!=v||p.snapshot!=v||p.word!=w||p.wideSnapshot!=w||Double.doubleToLongBits(p.floating)!=Double.doubleToLongBits(f)||Double.doubleToLongBits(p.floatSnapshot)!=Double.doubleToLongBits(f)||p.narrow!=n||p.intSnapshot!=n)throw new AssertionError("volatile identity/width/order");rows++;}System.out.println(rows);}}
public class VolatileDriver {public static void main(String[]args){VolatileOracle.run();}}
`

func TestAdversarialConstructorUnpublishedVolatileStorageRoundTrip(t *testing.T) {
	roundTripGenericFlowUnitsClasspath(t, "VolatileDriver", constructorOwnVolatileFixture, nil, []string{"VolatileOwner", "VolatileOwner$Child"}, true, Precision, Compatibility, "legacy")
}

func TestAdversarialConstructorVolatileStorageKeepsReceiverGuards(t *testing.T) {
	for _, shape := range []string{"open-finalizer", "observing-finalizer", "published-alias", "heap-alias-read", "moved-storage", "final-volatile", "static-storage", "duplicate-storage", "missing-original"} {
		t.Run(shape, func(t *testing.T) {
			source := strings.Replace(constructorOwnVolatileFixture, "public class VolatileDriver", "class VolatileDriver", 1)
			switch shape {
			case "open-finalizer":
				source = strings.Replace(source, "final class Child", "class Child", 1)
			case "observing-finalizer":
				source = strings.Replace(source, "Object capture(){", "protected void finalize(){System.identityHashCode(this);}Object capture(){", 1)
			case "published-alias":
				source = strings.Replace(source, "volatile Object value;", "static Object exposed;volatile Object value;", 1)
				source = strings.Replace(source, "value=v;", "Object alias=this;exposed=alias;value=v;", 1)
			case "heap-alias-read":
				source = strings.Replace(source, "value=v;", "value=this;", 1)
			}
			files := nativeCompileClasses(t, source)
			object, err := Parse(files["VolatileOwner$Child.class"])
			if err != nil {
				t.Fatal(err)
			}
			parent, err := Parse(files["VolatileParent.class"])
			if err != nil {
				t.Fatal(err)
			}
			for _, field := range parent.Fields {
				name, _ := sourceBridgeUTF8(parent, field.NameIndex)
				if name != "word" {
					continue
				}
				switch shape {
				case "final-volatile":
					field.AccessFlags |= 0x0010
				case "static-storage":
					field.AccessFlags |= 0x0008
				case "duplicate-storage":
					parent.Fields = append(parent.Fields, field)
				}
				break
			}
			files["VolatileParent.class"] = parent.Bytes()
			d := NewClassObjectDumper(object)
			d.options.TargetSourceVersion = 8
			d.foldSiblingResolver = func(name string) ([]byte, bool) {
				if shape == "missing-original" && name == "VolatileParent" {
					return nil, false
				}
				raw, ok := files[name+".class"]
				return raw, ok
			}
			d.FuncCtx = &class_context.ClassContext{}
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			writes := map[string]bool{object.GetClassName() + "\x00this$0\x00LVolatileOwner;": true}
			if shape == "moved-storage" {
				writes["VolatileParent\x00word\x00J"] = true
			}
			if d.constructorCaptureChainDoesNotObserve("VolatileParent", "(Ljava/lang/Object;JDI)V", writes) {
				t.Fatal("volatile storage bypassed original receiver/field/failure evidence")
			}
		})
	}
}

func TestAdversarialConstructorVolatileOracleRejectsCompiledSemanticMutants(t *testing.T) {
	_, java := t04Tools(t)
	original := nativeCompileSourceReleaseClasses(t, map[string]string{"VolatileDriver.java": constructorOwnVolatileFixture}, "none", "8")
	for _, mutation := range []struct{ name, before, after string }{
		{"original", "", ""},
		{"lost-capture", "return VolatileOwner.this.token;", "return null;"},
		{"lost-wide-argument", "super(v,w,f,n);", "super(v,0,f,n);"},
		{"lost-signed-zero", "super(v,w,f,n);", "super(v,w,0.0,n);"},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			files := original
			if mutation.name != "original" {
				if strings.Count(constructorOwnVolatileFixture, mutation.before) != 1 {
					t.Fatal("ambiguous semantic mutation")
				}
				source := strings.Replace(constructorOwnVolatileFixture, mutation.before, mutation.after, 1)
				files = nativeCompileSourceReleaseClasses(t, map[string]string{"VolatileDriver.java": source}, "none", "8")
			}
			out := t.TempDir()
			for name, raw := range original {
				if name == "VolatileOwner.class" || name == "VolatileOwner$Child.class" {
					var ok bool
					raw, ok = files[name]
					if !ok {
						t.Fatal("mutant lost original target", name)
					}
				}
				if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, assertions := range []string{"-ea", "-da"} {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				output, err := exec.CommandContext(ctx, java, "-Xverify:all", "-Xmx128m", assertions, "-cp", out, "VolatileDriver").CombinedOutput()
				interrupted := ctx.Err() != nil
				cancel()
				if mutation.name == "original" {
					if interrupted || err != nil || string(output) != "90\n" {
						t.Fatalf("original %s: %v %s", assertions, err, output)
					}
				} else if interrupted || err == nil || !strings.Contains(string(output), "java.lang.AssertionError: volatile identity/width/order") || strings.Contains(string(output), "VerifyError") || strings.Contains(string(output), "NoClassDefFoundError") {
					t.Fatalf("compiled mutation escaped independent semantic oracle: %v %s", err, output)
				}
			}
		})
	}
}

func TestAdversarialConstructorVolatileMethodMemoCannotBorrowFreshReceiverScope(t *testing.T) {
	source := strings.Replace(closedMethodEvidenceFixture, "int word;", "volatile int word;", 1)
	if source == closedMethodEvidenceFixture {
		t.Fatal("original volatile declaration not selected")
	}
	files := nativeCompileClasses(t, source+"class ClosedBodyMemoChild extends ClosedMethodEvidence{}")
	c, object := closedBodyMemoDomain(t, files)
	member := &values.JavaClassMember{Name: object.GetClassName(), Member: "change", Description: "(I)I"}
	invoke := func(fresh bool) bool {
		remaining := 512
		_, accepted := c.constructorReceiverClosedMethod(object, member, core.OP_INVOKESPECIAL, map[string]bool{}, map[string]bool{}, &remaining, 0, &constructorSelfStorageProof{freshReceiver: fresh}, constructorEffectValue{kind: 'I'})
		return accepted
	}
	if invoke(false) {
		t.Fatal("method-only cold proof granted fresh receiver permission")
	}
	if !invoke(true) || len(c.constructorProfileEvidence.closedBodies) != 1 {
		t.Fatal("original fresh-receiver body did not establish a certificate")
	}
	if invoke(false) {
		t.Fatal("method-only query borrowed constructor nonpublication evidence")
	}
	if !invoke(true) {
		t.Fatal("same original constructor scope lost its reusable certificate")
	}
}

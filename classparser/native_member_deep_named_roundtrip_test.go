package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const nativeMemberDeepNamedFixture = `class DeepEffects{static Object published;static boolean fail;static String trace="";static final IllegalArgumentException failure=new IllegalArgumentException("same");static long arg(long n){trace+="A";return n;}}
abstract class DeepParent{final Object observed;final long parentN;DeepParent(long n){DeepEffects.trace+="P";DeepEffects.published=this;observed=origin();parentN=n;if(DeepEffects.fail)throw DeepEffects.failure;}abstract Object origin();}
class DeepOwner{final Object rootValue;DeepOwner(Object value){rootValue=value;}class Layer{final Object layerValue;Layer(Object value){layerValue=value;}class Leaf extends DeepParent{final Object value;final long n;Leaf(Object value,long n){super(n);this.value=value;this.n=n;DeepEffects.trace+="C";}Object origin(){return Layer.this;}Object root(){return DeepOwner.this;}Object rootToken(){return DeepOwner.this.rootValue;}Object layerToken(){return Layer.this.layerValue;}}Leaf make(Object value,long n){return new Leaf(value,DeepEffects.arg(n));}}Layer layer(Object value){return new Layer(value);}}
class DeepDriver{public static void main(String[]args)throws Exception{Object token=new Object();DeepOwner root=new DeepOwner(token);DeepOwner.Layer layer=root.layer(token);int rows=0;for(boolean fail:new boolean[]{true,false})for(Object value:new Object[]{null,token,"text"})for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){DeepEffects.fail=fail;DeepEffects.trace="";DeepEffects.published=null;try{DeepOwner.Layer.Leaf child=layer.make(value,n);if(fail||child.observed!=layer||child.origin()!=layer||child.root()!=root||child.rootToken()!=token||child.layerToken()!=token||child.value!=value||child.n!=n||child.parentN!=n||child!=DeepEffects.published||!DeepEffects.trace.equals("APC"))throw new AssertionError("success");}catch(IllegalArgumentException e){DeepOwner.Layer.Leaf child=(DeepOwner.Layer.Leaf)DeepEffects.published;if(!fail||e!=DeepEffects.failure||child==null||child.observed!=layer||child.root()!=root||child.value!=null||child.n!=0||child.parentN!=n||!DeepEffects.trace.equals("AP"))throw new AssertionError("failure",e);}rows++;}java.lang.reflect.Constructor<?>ctor=DeepOwner.Layer.Leaf.class.getDeclaredConstructor(DeepOwner.Layer.class,Object.class,long.class);ctor.setAccessible(true);for(boolean fail:new boolean[]{true,false})for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){DeepEffects.fail=fail;DeepEffects.trace="";DeepEffects.published=null;try{DeepOwner.Layer.Leaf child=(DeepOwner.Layer.Leaf)ctor.newInstance(null,token,n);if(fail||child.observed!=null||child.origin()!=null||child.value!=token||child.n!=n||!DeepEffects.trace.equals("PC"))throw new AssertionError("null outer");try{child.root();throw new AssertionError("missing root dereference NPE");}catch(NullPointerException expected){}}catch(java.lang.reflect.InvocationTargetException e){DeepOwner.Layer.Leaf child=(DeepOwner.Layer.Leaf)DeepEffects.published;if(!fail||e.getCause()!=DeepEffects.failure||child==null||child.observed!=null||child.origin()!=null||child.value!=null||child.n!=0||child.parentN!=n||!DeepEffects.trace.equals("P"))throw new AssertionError("null outer failure",e);}rows++;}System.out.println(rows+":"+DeepEffects.trace);}}`

func TestNativeMemberDeepNamedRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	for _, bounds := range []string{"plain", "dollar-root", "depth3", "depth4", "static-boundary", "generic", "generic-shadow", "generic-bounds", "generic-inherited", "private-fields", "private-fields-volatile", "private-fields-wide", "private-fields-array", "private-fields-generic", "private-fields-shadow", "private-fields-sideeffect", "private-fields-null", "external"} {
		fixture := nativeMemberDeepNamedFixture
		rootName := "DeepOwner"
		wantOracle := "24:PC\n"
		owned := []string{"DeepOwner$Layer", "DeepOwner$Layer$Leaf"}
		if bounds == "depth3" || bounds == "depth4" {
			fixture = strings.Replace(fixture, "class Leaf extends", "class Middle{final Object middleValue;Middle(Object value){middleValue=value;}class Leaf extends", 1)
			fixture = strings.Replace(fixture, ";}}Layer layer", ";}}Middle middle(Object value){return new Middle(value);}}Layer layer", 1)
			fixture = strings.Replace(fixture, "Object origin(){return Layer.this;}", "Object origin(){return Middle.this;}", 1)
			fixture = strings.ReplaceAll(fixture, "DeepOwner.Layer.Leaf", "DeepOwner.Layer.Middle.Leaf")
			fixture = strings.Replace(fixture, "layer=root.layer(token);int rows", "layer=root.layer(token);DeepOwner.Layer.Middle middle=layer.middle(token);int rows", 1)
			fixture = strings.ReplaceAll(fixture, "layer.make(value,n)", "middle.make(value,n)")
			fixture = strings.ReplaceAll(fixture, "child.observed!=layer||child.origin()!=layer", "child.observed!=middle||child.origin()!=middle")
			fixture = strings.ReplaceAll(fixture, "child.observed!=layer||child.root()", "child.observed!=middle||child.root()")
			fixture = strings.Replace(fixture, "getDeclaredConstructor(DeepOwner.Layer.class", "getDeclaredConstructor(DeepOwner.Layer.Middle.class", 1)
			owned = []string{"DeepOwner$Layer", "DeepOwner$Layer$Middle", "DeepOwner$Layer$Middle$Leaf"}
		}
		if bounds == "depth4" {
			fixture = strings.Replace(fixture, "class Leaf extends", "class Middle2{final Object middle2Value;Middle2(Object value){middle2Value=value;}class Leaf extends", 1)
			fixture = strings.Replace(fixture, ";}}Middle middle", ";}}Middle2 middle2(Object value){return new Middle2(value);}}Middle middle", 1)
			fixture = strings.Replace(fixture, "Object origin(){return Middle.this;}", "Object origin(){return Middle2.this;}", 1)
			fixture = strings.ReplaceAll(fixture, "DeepOwner.Layer.Middle.Leaf", "DeepOwner.Layer.Middle.Middle2.Leaf")
			fixture = strings.Replace(fixture, "middle=layer.middle(token);int rows", "middle=layer.middle(token);DeepOwner.Layer.Middle.Middle2 middle2=middle.middle2(token);int rows", 1)
			fixture = strings.ReplaceAll(fixture, "middle.make(value,n)", "middle2.make(value,n)")
			fixture = strings.ReplaceAll(fixture, "child.observed!=middle||child.origin()!=middle", "child.observed!=middle2||child.origin()!=middle2")
			fixture = strings.ReplaceAll(fixture, "child.observed!=middle||child.root()", "child.observed!=middle2||child.root()")
			fixture = strings.Replace(fixture, "getDeclaredConstructor(DeepOwner.Layer.Middle.class", "getDeclaredConstructor(DeepOwner.Layer.Middle.Middle2.class", 1)
			owned = []string{"DeepOwner$Layer", "DeepOwner$Layer$Middle", "DeepOwner$Layer$Middle$Middle2", "DeepOwner$Layer$Middle$Middle2$Leaf"}
		}
		if bounds == "static-boundary" {
			fixture = strings.Replace(fixture, "class Layer{final Object layerValue;Layer(Object value){layerValue=value;}", "static class Layer{final DeepOwner root;final Object layerValue;Layer(DeepOwner owner,Object value){root=owner;layerValue=value;}", 1)
			fixture = strings.ReplaceAll(fixture, "DeepOwner.this.rootValue", "Layer.this.root.rootValue")
			fixture = strings.ReplaceAll(fixture, "return DeepOwner.this;", "return Layer.this.root;")
			fixture = strings.Replace(fixture, "return new Layer(value);", "return new Layer(this,value);", 1)
		}
		if bounds == "dollar-root" {
			rootName = "Dollar$Deep"
			fixture = strings.ReplaceAll(fixture, "DeepOwner", rootName)
		}
		if bounds == "generic" || bounds == "generic-bounds" || bounds == "generic-shadow" || bounds == "private-fields-generic" || bounds == "private-fields-shadow" {
			fixture = strings.Replace(fixture, "class DeepOwner{final Object rootValue;DeepOwner(Object value)", "class DeepOwner<T>{final T rootValue;DeepOwner(T value)", 1)
			fixture = strings.Replace(fixture, "class Layer{final Object layerValue;Layer(Object value)", "class Layer<U>{final U layerValue;Layer(U value)", 1)
			fixture = strings.Replace(fixture, "class Leaf extends DeepParent{final Object value;final long n;Leaf(Object value", "class Leaf<V> extends DeepParent{final V value;final long n;Leaf(V value", 1)
			fixture = strings.Replace(fixture, "Leaf make(Object value,long n){return new Leaf(value", "Leaf<U> make(U value,long n){return new Leaf<U>(value", 1)
			fixture = strings.Replace(fixture, "Layer layer(Object value){return new Layer(value)", "Layer<T> layer(T value){return new Layer<T>(value)", 1)
			fixture = strings.Replace(fixture, "DeepOwner root=new DeepOwner(token)", "DeepOwner<Object> root=new DeepOwner<Object>(token)", 1)
			fixture = strings.Replace(fixture, "DeepOwner.Layer layer=root.layer(token)", "DeepOwner<Object>.Layer<Object> layer=root.layer(token)", 1)
			fixture = strings.ReplaceAll(fixture, "DeepOwner.Layer.Leaf child", "DeepOwner<Object>.Layer<Object>.Leaf<Object> child")
			fixture = strings.ReplaceAll(fixture, "(DeepOwner.Layer.Leaf)", "(DeepOwner<Object>.Layer<Object>.Leaf<Object>)")
			if bounds == "generic-shadow" || bounds == "private-fields-shadow" {
				fixture = strings.ReplaceAll(fixture, "<U>", "<T>")
				fixture = strings.ReplaceAll(fixture, "<V>", "<T>")
				fixture = strings.ReplaceAll(fixture, "final U layerValue;Layer(U value)", "final T layerValue;Layer(T value)")
				fixture = strings.ReplaceAll(fixture, "final V value;final long n;Leaf(V value", "final T value;final long n;Leaf(T value")
				fixture = strings.Replace(fixture, "make(U value", "make(T value", 1)
			}
		}
		if bounds == "generic-bounds" {
			fixture = strings.Replace(fixture, "class DeepOwner<T>", "class DeepOwner<T extends Number>", 1)
			fixture = strings.Replace(fixture, "class Layer<U>", "class Layer<U extends T>", 1)
			fixture = strings.Replace(fixture, "class Leaf<V>", "class Leaf<V extends U>", 1)
			fixture = strings.Replace(fixture, "Object token=new Object()", "Number token=Long.valueOf(Long.MIN_VALUE)", 1)
			fixture = strings.ReplaceAll(fixture, "new Object[]{null,token,\"text\"}", "new Number[]{null,token,Integer.valueOf(7)}")
			fixture = strings.Replace(fixture, "for(Object value:", "for(Number value:", 1)
			fixture = strings.ReplaceAll(fixture, "DeepOwner<Object>", "DeepOwner<Number>")
			fixture = strings.ReplaceAll(fixture, "Layer<Object>", "Layer<Number>")
			fixture = strings.ReplaceAll(fixture, "Leaf<Object>", "Leaf<Number>")
			fixture = strings.Replace(fixture, "getDeclaredConstructor(DeepOwner.Layer.class,Object.class", "getDeclaredConstructor(DeepOwner.Layer.class,Number.class", 1)
		}
		if bounds == "generic-inherited" {
			fixture = strings.Replace(fixture, "class DeepOwner{final Object rootValue;DeepOwner(Object value)", "class DeepOwner<T>{final T rootValue;DeepOwner(T value)", 1)
			fixture = strings.Replace(fixture, "class Layer{final Object layerValue;Layer(Object value)", "class Layer{final T layerValue;Layer(T value)", 1)
			fixture = strings.Replace(fixture, "class Leaf extends DeepParent{final Object value;final long n;Leaf(Object value", "class Leaf extends DeepParent{final T value;final long n;Leaf(T value", 1)
			fixture = strings.Replace(fixture, "Leaf make(Object value", "Leaf make(T value", 1)
			fixture = strings.Replace(fixture, "Layer layer(Object value", "Layer layer(T value", 1)
			fixture = strings.Replace(fixture, "DeepOwner root=new DeepOwner(token)", "DeepOwner<Object> root=new DeepOwner<Object>(token)", 1)
			fixture = strings.Replace(fixture, "DeepOwner.Layer layer", "DeepOwner<Object>.Layer layer", 1)
			fixture = strings.ReplaceAll(fixture, "DeepOwner.Layer.Leaf child", "DeepOwner<Object>.Layer.Leaf child")
			fixture = strings.ReplaceAll(fixture, "(DeepOwner.Layer.Leaf)", "(DeepOwner<Object>.Layer.Leaf)")
		}
		if strings.HasPrefix(bounds, "private-fields") {
			fixture = strings.Replace(fixture, "{final Object rootValue", "{private final Object rootValue", 1)
			fixture = strings.Replace(fixture, "{final Object layerValue", "{private final Object layerValue", 1)
			fixture = strings.Replace(fixture, "{final T rootValue", "{private final T rootValue", 1)
			fixture = strings.Replace(fixture, "{final U layerValue", "{private final U layerValue", 1)
			fixture = strings.Replace(fixture, "{final T layerValue", "{private final T layerValue", 1)
		}
		switch bounds {
		case "private-fields-volatile":
			fixture = strings.ReplaceAll(fixture, "private final Object", "private volatile Object")
		case "private-fields-wide":
			fixture = strings.Replace(fixture, "private final Object rootValue;", "private final Object rootValue;private long wide=Long.MIN_VALUE;", 1)
			fixture = strings.Replace(fixture, "Object layerToken()", "long rootWide(){return DeepOwner.this.wide;}Object layerToken()", 1)
			fixture = strings.ReplaceAll(fixture, "child.rootToken()!=token", "child.rootWide()!=Long.MIN_VALUE||child.rootToken()!=token")
		case "private-fields-array":
			fixture = strings.ReplaceAll(fixture, "private final Object ", "private final Object[] ")
			fixture = strings.ReplaceAll(fixture, "rootValue=value", "rootValue=(Object[])value")
			fixture = strings.ReplaceAll(fixture, "layerValue=value", "layerValue=(Object[])value")
			fixture = strings.Replace(fixture, "Object token=new Object()", `Object token=new Object[]{null,"value"}`, 1)
		case "private-fields-sideeffect":
			fixture = strings.Replace(fixture, "Object rootToken(){return DeepOwner.this.rootValue;}", `DeepOwner selectRoot(){DeepEffects.trace+="R";return DeepOwner.this;}Object rootToken(){return selectRoot().rootValue;}`, 1)
			fixture = strings.Replace(fixture, "Object layerToken(){return Layer.this.layerValue;}", `Layer selectLayer(){DeepEffects.trace+="L";return Layer.this;}Object layerToken(){return selectLayer().layerValue;}`, 1)
			fixture = strings.ReplaceAll(fixture, `equals("APC")`, `equals("APCRL")`)
		case "private-fields-null":
			fixture = strings.Replace(fixture, "Object layerToken()", "Object nullToken(){return ((DeepOwner)null).rootValue;}Object layerToken()", 1)
			fixture = strings.Replace(fixture, `throw new AssertionError("success");`, `throw new AssertionError("success");try{child.nullToken();throw new AssertionError("null getter receiver");}catch(NullPointerException expected){}`, 1)
		}
		if bounds == "external" {
			fixture += `class DeepExternal{static DeepOwner.Layer.Leaf pick(DeepOwner.Layer layer,Object value,long n){return layer.new Leaf(value,DeepEffects.arg(n));}}`
			fixture = strings.Replace(fixture, "for(Object value:new Object[]{null,token,\"text\"})", "for(boolean external:new boolean[]{false,true})for(Object value:new Object[]{null,token,\"text\"})", 1)
			fixture = strings.Replace(fixture, "child=layer.make(value,n)", "child=external?DeepExternal.pick(layer,value,n):layer.make(value,n)", 1)
			fixture = strings.Replace(fixture, "System.out.println(rows+\":\"+DeepEffects.trace);", `DeepEffects.trace="";DeepEffects.published=null;try{DeepExternal.pick(null,token,7);throw new AssertionError("missing external NPE");}catch(NullPointerException expected){if(!DeepEffects.trace.equals("")||DeepEffects.published!=null)throw new AssertionError("external check-before-argument");}System.out.println(rows+":PC");`, 1)
			wantOracle = "42:PC\n"
		}
		debugModes := []string{"none", "source,lines,vars"}
		policies := []string{"normal", "no-source-rewrites", "no-core-cleanups"}
		for _, debug := range debugModes {
			t.Run(bounds+"/"+debug, func(t *testing.T) {
				files := nativeCompileDebugClasses(t, fixture, debug)
				original := t.TempDir()
				for n, raw := range files {
					if err := os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
				oracle := t04RunJava(t, java, original, "DeepDriver")
				if oracle != wantOracle {
					t.Fatalf("original oracle %q", oracle)
				}
				for _, policy := range policies {
					t.Run(policy, func(t *testing.T) {
						if policy == "no-source-rewrites" {
							t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
						}
						if policy == "no-core-cleanups" {
							t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
						}
						z := nativeArchive(t, files)
						defer z.Close()
						for _, n := range owned {
							n = strings.Replace(n, "DeepOwner", rootName, 1)
							raw, err := z.ReadFile(n + ".class")
							if err != nil || !strings.Contains(string(raw), "body owned by") {
								t.Fatalf("original joint ownership %s %v\n%s", n, err, raw)
							}
						}
						src, err := z.ReadFile(rootName + ".class")
						if err != nil || strings.Contains(string(src), DecompileStubMarker) {
							t.Fatalf("source %v\n%s", err, src)
						}
						output := t.TempDir()
						if bounds == "external" {
							ext, err := z.ReadFile("DeepExternal.class")
							if err != nil || strings.Contains(string(ext), DecompileStubMarker) {
								t.Fatalf("external source %v\n%s", err, ext)
							}
							if err := os.WriteFile(filepath.Join(output, "DeepExternal.java"), ext, 0600); err != nil {
								t.Fatal(err)
							}
						}
						for n, raw := range files {
							if strings.HasPrefix(n, rootName) {
								continue
							}
							if err := os.WriteFile(filepath.Join(output, n), raw, 0600); err != nil {
								t.Fatal(err)
							}
						}
						file := filepath.Join(output, rootName+".java")
						if err := os.WriteFile(file, src, 0600); err != nil {
							t.Fatal(err)
						}
						inputs := []string{"-proc:none", "--release", "8", "-cp", output, "-d", output, file}
						if bounds == "external" {
							inputs = append(inputs, filepath.Join(output, "DeepExternal.java"))
						}
						if out, err := exec.Command(javac, inputs...).CombinedOutput(); err != nil {
							t.Fatalf("rebuilt %v %s\n%s", err, out, src)
						}
						if got := t04RunJava(t, java, output, "DeepDriver"); got != oracle {
							t.Fatalf("JVM mismatch %q != %q", got, oracle)
						}
						for n, want := range files {
							if !strings.HasPrefix(n, rootName) {
								continue
							}
							raw, err := os.ReadFile(filepath.Join(output, n))
							if err != nil {
								t.Fatal(err)
							}
							if got := nativeBinaryShape(t, raw); got != nativeBinaryShape(t, want) {
								t.Fatalf("ABI changed %s\n%s\n%s", n, nativeBinaryShape(t, want), got)
							}
						}
					})
				}
			})
		}
	}
}

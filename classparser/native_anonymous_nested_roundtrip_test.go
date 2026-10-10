package javaclassparser

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const nativeAnonymousNestedFixture = `class NestedEffects{static Object published;static boolean fail;static String trace="";static final RuntimeException failure=new RuntimeException("same");static long arg(long n){trace+="A";return n;}}
abstract class NestedFactory{abstract NestedPhase build(long n);}
abstract class NestedPhase{final NestedFactory factory;final Object observed;final long parentN;NestedPhase(NestedFactory factory,long n){NestedEffects.trace+="P";NestedEffects.published=this;this.factory=factory;observed=origin();parentN=n;if(NestedEffects.fail)throw NestedEffects.failure;}abstract Object origin();abstract long calc(long n);}
class NestedOwner{static NestedFactory make(final Object token,final long seed){return new NestedFactory(){NestedPhase build(long n){return new NestedPhase(this,NestedEffects.arg(n)){Object origin(){return token;}long calc(long n){return n^seed;}};}};}}
class NestedDriver{public static void main(String[]args)throws Exception{Object identity=new Object();int rows=0;for(boolean fail:new boolean[]{false,true})for(Object token:new Object[]{null,identity,"text"})for(long seed:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){NestedFactory factory=NestedOwner.make(token,seed);for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){NestedEffects.fail=fail;NestedEffects.trace="";NestedEffects.published=null;try{NestedPhase phase=factory.build(n);if(fail||phase.factory!=factory||phase.observed!=token||phase.origin()!=token||phase.parentN!=n||phase.calc(n)!=(n^seed)||NestedEffects.published!=phase||!NestedEffects.trace.equals("AP"))throw new AssertionError("normal");}catch(RuntimeException e){NestedPhase phase=(NestedPhase)NestedEffects.published;if(!fail||e!=NestedEffects.failure||phase==null||phase.factory!=factory||phase.observed!=token||phase.origin()!=token||phase.parentN!=n||phase.calc(n)!=(n^seed)||!NestedEffects.trace.equals("AP"))throw new AssertionError("failure",e);}rows++;}}NestedFactory factory=NestedOwner.make(identity,7);Class<?>child=Class.forName("NestedOwner$1$1");java.lang.reflect.Constructor<?>ctor=child.getDeclaredConstructor(factory.getClass(),NestedFactory.class,long.class);ctor.setAccessible(true);for(boolean nullOuter:new boolean[]{false,true})for(boolean fail:new boolean[]{false,true}){NestedEffects.fail=fail;NestedEffects.trace="";NestedEffects.published=null;try{NestedPhase phase=(NestedPhase)ctor.newInstance(nullOuter?null:factory,factory,Long.MIN_VALUE);if(fail||nullOuter||phase.factory!=factory||phase.observed!=identity||phase.calc(Long.MIN_VALUE)!=(Long.MIN_VALUE^7)||!NestedEffects.trace.equals("P"))throw new AssertionError("reflective success");}catch(java.lang.reflect.InvocationTargetException e){NestedPhase phase=(NestedPhase)NestedEffects.published;if(phase==null||phase.factory!=factory||!NestedEffects.trace.equals("P"))throw new AssertionError("publication order",e);if(nullOuter){if(!(e.getCause() instanceof NullPointerException)||phase.observed!=null||phase.parentN!=0)throw new AssertionError("null outer callback order",e);try{phase.origin();throw new AssertionError("missing ancestor dereference");}catch(NullPointerException expected){}}else if(!fail||e.getCause()!=NestedEffects.failure||phase.observed!=identity||phase.origin()!=identity||phase.parentN!=Long.MIN_VALUE)throw new AssertionError("reflective parent failure",e);}rows++;}System.out.println(rows+":"+NestedEffects.trace);}}`

func TestNativeAnonymousNestedOriginalCaptureRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	for _, scope := range []string{"static-root-interface-default", "depth3-interface-default", "member-private-context-interface-default", "member-private-context-depth-interface-default", "static-root-interface-major51", "static-root-interface", "depth3-interface", "member-private-context-interface", "member-private-context-depth-interface", "member-instance-constants", "member-depth-instance-constants", "member-private-context-constants", "member-private-context-depth-constants", "static-root", "dollar-root", "instance-root", "static-local", "dollar-local", "instance-local", "depth3", "depth4", "depth3-instance", "depth3-shadow", "depth3-major51", "depth3-multiple", "member-static", "member-instance", "member-depth-static", "member-depth-instance", "member-context", "member-context-depth", "member-private-context", "member-private-context-depth", "member-private-context-args", "member-private-context-depth-args"} {
		fixture := nativeAnonymousNestedFixture
		if strings.HasPrefix(scope, "depth") {
			depth := 3
			if scope == "depth4" {
				depth = 4
			}
			fixture = nativeAnonymousNestedDepthFixture(depth)
			if scope == "depth3-instance" {
				fixture = strings.Replace(fixture, "static NestedHub1 make(", "NestedHub1 make(", 1)
				fixture = strings.ReplaceAll(fixture, "NestedOwner.make(", "new NestedOwner().make(")
			}
			if scope == "depth3-multiple" {
				body := `return new NestedFactory(){NestedPhase build(long n){return new NestedPhase(this,NestedEffects.arg(n)){Object origin(){return token;}long calc(long n){return n^seed;}};}};`
				fixture = strings.Replace(fixture, "abstract NestedFactory next();", "abstract NestedFactory next();abstract NestedFactory next2();", 1)
				fixture = strings.Replace(fixture, "NestedFactory next(){"+body+"}", "NestedFactory next(){"+body+"}NestedFactory next2(){"+body+"}", 1)
				fixture = strings.Replace(fixture, "class NestedDriver{public static", `class NestedDriver{static NestedFactory choose(NestedHub1 hub,long seed){return seed<0?hub.next():hub.next2();}public static`, 1)
				fixture = strings.ReplaceAll(fixture, "NestedOwner.make(token,seed).next()", "NestedDriver.choose(NestedOwner.make(token,seed),seed)")
				fixture = strings.ReplaceAll(fixture, "NestedOwner.make(identity,7).next()", "NestedDriver.choose(NestedOwner.make(identity,7),7)")
				fixture = strings.Replace(fixture, `Class.forName("NestedOwner$1$1$1")`, `Class.forName(factory.getClass().getName()+"$1")`, 1)
			}
			if scope == "depth3-shadow" {
				fixture = strings.ReplaceAll(fixture, "abstract long calc(long n);", "abstract long calc(long n);abstract Object echo(Object token);")
				fixture = strings.Replace(fixture, "long calc(long n){return n^seed;}", "long calc(long n){return n^seed;}Object echo(Object token){return token;}", 1)
				fixture = strings.ReplaceAll(fixture, "phase.origin()!=token", "phase.origin()!=token||phase.echo(identity)!=identity")
			}
		}
		if strings.HasPrefix(scope, "member-") {
			fixtureScope := scope
			if strings.Contains(scope, "context") {
				fixtureScope = "member-instance"
				if strings.Contains(scope, "depth") {
					fixtureScope = "member-depth-instance"
				}
			}
			fixture = nativeAnonymousNestedMemberFixture(fixtureScope)
			if strings.Contains(scope, "context") {
				fixture = nativeAnonymousNestedContextFixture(scope)
				if strings.Contains(scope, "private") {
					fixture = strings.Replace(fixture, "final Object token;final long seed;", "private final Object token;private final long seed;", 1)
					if strings.Contains(scope, "args") {
						fixture = nativeAnonymousNestedPrivateArgsFixture(fixture, scope)
					}
				}
			}
		}
		if strings.Contains(scope, "-interface") {
			fixture = strings.Replace(fixture, "class NestedOwner{", `class NestedOwner{interface Contract<T extends Number>{T echo(T n);}static class Implementation implements Contract<Long>{public Long echo(Long n){return n;}}`, 1)
			fixture = strings.Replace(fixture, "int rows=0;", `int rows=0;if(new NestedOwner.Implementation().echo(Long.valueOf(7)).longValue()!=7||!NestedOwner.Contract.class.isMemberClass()||NestedOwner.Contract.class.getDeclaringClass()!=NestedOwner.class||!java.lang.reflect.Modifier.isStatic(NestedOwner.Contract.class.getModifiers()))throw new AssertionError("interface scope");`, 1)
		}
		if strings.Contains(scope, "interface-default") {
			fixture = strings.Replace(fixture, "T echo(T n);", `Object identity=new Object();T echo(T n);default long mix(long n){return n^Long.MAX_VALUE;}static long sum(long a,long b){return a+b;}`, 1)
			fixture = strings.Replace(fixture, `if(new NestedOwner.Implementation()`, `if(NestedOwner.Contract.identity==null||new NestedOwner.Implementation().mix(Long.MIN_VALUE)!=-1||NestedOwner.Contract.sum(Long.MAX_VALUE,1)!=Long.MIN_VALUE||new NestedOwner.Implementation()`, 1)
		}
		if strings.Contains(scope, "constants") {
			const declarations = `private static final byte tiny=-128;protected static final short shorty=-32768;public static final char letter='\uffff';static final boolean enabled=true;static final int count=Integer.MIN_VALUE;private static final long wide=Long.MIN_VALUE;static final float fraction=-0.0f;static final double precise=Double.POSITIVE_INFINITY;static final String text="\u03bb:\"\n";`
			fixture = strings.Replace(fixture, "class Layer{", "class Layer{"+declarations, 1)
			const probe = `Class<?>constants=Class.forName("NestedOwner$Layer");java.lang.reflect.Field[]constantFields=constants.getDeclaredFields();int constantCount=0;for(java.lang.reflect.Field f:constantFields){if(!java.lang.reflect.Modifier.isStatic(f.getModifiers()))continue;if(!java.lang.reflect.Modifier.isFinal(f.getModifiers()))throw new AssertionError("constant flags");f.setAccessible(true);Object v=f.get(null);String name=f.getName();if(name.equals("tiny")&&!v.equals(Byte.valueOf((byte)-128))||name.equals("shorty")&&!v.equals(Short.valueOf((short)-32768))||name.equals("letter")&&!v.equals(Character.valueOf('\uffff'))||name.equals("enabled")&&!v.equals(Boolean.TRUE)||name.equals("count")&&!v.equals(Integer.valueOf(Integer.MIN_VALUE))||name.equals("wide")&&!v.equals(Long.valueOf(Long.MIN_VALUE))||name.equals("fraction")&&Float.floatToRawIntBits(((Float)v).floatValue())!=0x80000000||name.equals("precise")&&!v.equals(Double.valueOf(Double.POSITIVE_INFINITY))||name.equals("text")&&!v.equals("\u03bb:\"\n"))throw new AssertionError("constant value "+name);constantCount++;}if(constantCount!=9)throw new AssertionError("constant layout");`
			fixture = strings.Replace(fixture, "int rows=0;", "int rows=0;"+probe, 1)
		}
		rootName := "NestedOwner"
		if strings.HasPrefix(scope, "dollar-") {
			rootName = "Dollar$Nested"
			fixture = strings.ReplaceAll(fixture, "NestedOwner", rootName)
		}
		if strings.HasPrefix(scope, "instance-") {
			fixture = strings.Replace(fixture, "static NestedFactory make(", "NestedFactory make(", 1)
			fixture = strings.ReplaceAll(fixture, "NestedOwner.make(", "new NestedOwner().make(")
		}
		wantOracle := "58:P\n"
		if strings.Contains(scope, "context") {
			wantOracle = "60:AP\n"
			if strings.Contains(scope, "args") {
				wantOracle = "60:\n"
			}
		}
		if strings.HasSuffix(scope, "-local") {
			fixture = strings.Replace(fixture, "NestedPhase build(long n){return new NestedPhase", "NestedPhase build(long n){final Object captured=token;final long copiedSeed=seed;return new NestedPhase", 1)
			fixture = strings.Replace(fixture, "Object origin(){return token;}long calc(long n){return n^seed;}", "Object origin(){return captured;}long calc(long n){return n^copiedSeed;}", 1)
			fixture = strings.Replace(fixture, "getDeclaredConstructor(factory.getClass(),NestedFactory.class,long.class)", "getDeclaredConstructor(factory.getClass(),NestedFactory.class,long.class,Object.class,long.class)", 1)
			fixture = strings.Replace(fixture, "ctor.newInstance(nullOuter?null:factory,factory,Long.MIN_VALUE)", "ctor.newInstance(nullOuter?null:factory,factory,Long.MIN_VALUE,identity,7L)", 1)
			fixture = strings.Replace(fixture, "if(fail||nullOuter||phase.factory", "if(fail||phase.factory", 1)
			fixture = strings.Replace(fixture, `if(nullOuter){if(!(e.getCause() instanceof NullPointerException)||phase.observed!=null||phase.parentN!=0)throw new AssertionError("null outer callback order",e);try{phase.origin();throw new AssertionError("missing ancestor dereference");}catch(NullPointerException expected){}}else if(!fail`, `if(!fail`, 1)
		}
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(scope+"/"+debug, func(t *testing.T) {
				files := nativeCompileDebugClasses(t, fixture, debug)
				if scope == "depth3-major51" || scope == "static-root-interface-major51" {
					for n, raw := range files {
						if strings.HasPrefix(n, rootName) {
							copy := append([]byte(nil), raw...)
							copy[6] = 0
							copy[7] = 51
							files[n] = copy
						}
					}
				}
				original := t.TempDir()
				for n, raw := range files {
					if e := os.WriteFile(filepath.Join(original, n), raw, 0600); e != nil {
						t.Fatal(e)
					}
				}
				oracle := t04RunJava(t, java, original, "NestedDriver")
				if oracle != wantOracle {
					t.Fatalf("original %q", oracle)
				}
				for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
					t.Run(policy, func(t *testing.T) {
						if policy == "no-source-rewrites" {
							t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
						}
						if policy == "no-core-cleanups" {
							t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
						}
						z := nativeArchive(t, files)
						defer z.Close()
						for n := range files {
							if !strings.HasPrefix(n, rootName+"$") {
								continue
							}
							n = strings.TrimSuffix(n, ".class")
							raw, e := z.ReadFile(n + ".class")
							if e != nil || !strings.Contains(string(raw), "body owned by") {
								t.Fatalf("ownership %s %v\n%s", n, e, raw)
							}
						}
						src, e := z.ReadFile(rootName + ".class")
						if e != nil || strings.Contains(string(src), DecompileStubMarker) {
							t.Fatalf("source %v\n%s", e, src)
						}
						output := t.TempDir()
						for n, raw := range files {
							if strings.HasPrefix(n, rootName) {
								continue
							}
							if e := os.WriteFile(filepath.Join(output, n), raw, 0600); e != nil {
								t.Fatal(e)
							}
						}
						file := filepath.Join(output, rootName+".java")
						if e := os.WriteFile(file, src, 0600); e != nil {
							t.Fatal(e)
						}
						if out, e := exec.Command(javac, "-proc:none", "--release", "8", "-cp", output, "-d", output, file).CombinedOutput(); e != nil {
							t.Fatalf("rebuilt %v %s\n%s", e, out, src)
						}
						if got := t04RunJava(t, java, output, "NestedDriver"); got != oracle {
							t.Fatalf("JVM %s != %s", got, oracle)
						}
						for n, want := range files {
							if !strings.HasPrefix(n, rootName) {
								continue
							}
							raw, e := os.ReadFile(filepath.Join(output, n))
							if e != nil {
								t.Fatal(e)
							}
							if got := nativeBinaryShape(t, raw); got != nativeBinaryShape(t, want) {
								t.Fatalf("ABI %s\n%s\n%s", n, nativeBinaryShape(t, want), got)
							}
							if got := nativeOriginalConstantValueShape(t, raw); got != nativeOriginalConstantValueShape(t, want) {
								t.Fatalf("constant status/bits %s\n%s\n%s", n, nativeOriginalConstantValueShape(t, want), got)
							}
							if got := nativeAnonymousAccessorShape(t, raw); got != nativeAnonymousAccessorShape(t, want) {
								t.Fatalf("accessor ABI %s\n%s\n%s", n, nativeAnonymousAccessorShape(t, want), got)
							}
						}
					})
				}
			})
		}
	}
}

func nativeAnonymousNestedDepthFixture(depth int) string {
	fixture := nativeAnonymousNestedFixture
	declarations := ""
	prefix := ""
	suffix := ""
	calls := ""
	child := "NestedOwner"
	for i := 1; i <= depth-2; i++ {
		next := "NestedFactory"
		if i < depth-2 {
			next = "NestedHub" + strconv.Itoa(i+1)
		}
		hub := "NestedHub" + strconv.Itoa(i)
		declarations += "abstract class " + hub + "{abstract " + next + " next();}"
		prefix += "return new " + hub + "(){" + next + " next(){"
		suffix += "}};"
		calls += ".next()"
	}
	for i := 0; i < depth; i++ {
		child += "$1"
	}
	fixture = strings.Replace(fixture, "class NestedOwner{static NestedFactory make(", declarations+"class NestedOwner{static NestedHub1 make(", 1)
	fixture = strings.Replace(fixture, "{return new NestedFactory(){NestedPhase build(long n)", "{"+prefix+"return new NestedFactory(){NestedPhase build(long n)", 1)
	fixture = strings.Replace(fixture, "}};}}\nclass NestedDriver", "}};"+suffix+"}}\nclass NestedDriver", 1)
	fixture = strings.ReplaceAll(fixture, "NestedOwner.make(token,seed)", "NestedOwner.make(token,seed)"+calls)
	fixture = strings.ReplaceAll(fixture, "NestedOwner.make(identity,7)", "NestedOwner.make(identity,7)"+calls)
	fixture = strings.ReplaceAll(fixture, "NestedOwner$1$1", child)
	return fixture
}

func nativeAnonymousNestedMemberFixture(scope string) string {
	fixture := nativeAnonymousNestedFixture
	static := "static "
	receiver := "Layer.make(token,seed)"
	if strings.Contains(scope, "instance") {
		static = ""
		receiver = "new Layer().make(token,seed)"
	}
	fixture = strings.Replace(fixture, "class NestedOwner{static NestedFactory make(", "class NestedOwner{"+static+"NestedFactory make(Object token,long seed){return "+receiver+";}"+static+"class Layer{"+static+"NestedFactory make(", 1)
	fixture = strings.Replace(fixture, "}};}}\nclass NestedDriver", "}};}}}\nclass NestedDriver", 1)
	if strings.Contains(scope, "depth") {
		receiver := "Middle.make(token,seed)"
		if static == "" {
			receiver = "new Middle().make(token,seed)"
		}
		fixture = strings.Replace(fixture, static+"class Layer{"+static+"NestedFactory make(", static+"class Layer{"+static+"NestedFactory make(Object token,long seed){return "+receiver+";}"+static+"class Middle{"+static+"NestedFactory make(", 1)
		fixture = strings.Replace(fixture, "}};}}}\nclass NestedDriver", "}};}}}}\nclass NestedDriver", 1)
	}
	fixture = strings.Replace(fixture, `Class.forName("NestedOwner$1$1")`, `Class.forName(factory.getClass().getName()+"$1")`, 1)
	if strings.Contains(scope, "instance") {
		fixture = strings.ReplaceAll(fixture, "NestedOwner.make(", "new NestedOwner().make(")
	}
	return fixture
}

func nativeAnonymousNestedContextFixture(scope string) string {
	fixtureScope, owner := "member-instance", "Layer"
	if strings.Contains(scope, "depth") {
		fixtureScope, owner = "member-depth-instance", "Middle"
	}
	fixture := nativeAnonymousNestedMemberFixture(fixtureScope)
	fixture = strings.Replace(fixture, "class "+owner+"{NestedFactory make(", "class "+owner+"{final Object token;final long seed;"+owner+"(Object token,long seed){this.token=token;this.seed=seed;}NestedFactory make(", 1)
	fixture = strings.ReplaceAll(fixture, "new "+owner+"().make(token,seed)", "new "+owner+"(token,seed).make(token,seed)")
	fixture = strings.Replace(fixture, "Object origin(){return token;}long calc(long n){return n^seed;}", "Object origin(){return "+owner+".this.token;}long calc(long n){return n^"+owner+".this.seed;}", 1)
	checks := `java.lang.reflect.Constructor<?>factoryCtor=factory.getClass().getDeclaredConstructor(Class.forName(factory.getClass().getName().substring(0,factory.getClass().getName().lastIndexOf('$'))));factoryCtor.setAccessible(true);NestedFactory nullNamed=(NestedFactory)factoryCtor.newInstance(new Object[]{null});for(boolean fail:new boolean[]{false,true}){NestedEffects.fail=fail;NestedEffects.trace="";NestedEffects.published=null;try{nullNamed.build(Long.MIN_VALUE);throw new AssertionError("missing named ancestor dereference");}catch(NullPointerException expected){NestedPhase phase=(NestedPhase)NestedEffects.published;if(phase==null||phase.factory!=nullNamed||phase.observed!=null||phase.parentN!=0||!NestedEffects.trace.equals("AP"))throw new AssertionError("named ancestor callback order");try{phase.origin();throw new AssertionError("missing named ancestor dereference");}catch(NullPointerException wanted){}}rows++;}`
	return strings.Replace(fixture, "System.out.println(rows+", checks+"System.out.println(rows+", 1)
}

func nativeAnonymousAccessorShape(t *testing.T, raw []byte) string {
	t.Helper()
	object, e := Parse(raw)
	if e != nil {
		t.Fatal(e)
	}
	rows := []string{}
	for _, m := range object.Methods {
		n, _ := sourceBridgeUTF8(object, m.NameIndex)
		d, _ := sourceBridgeUTF8(object, m.DescriptorIndex)
		if strings.HasPrefix(n, "access$") {
			rows = append(rows, n+d+":"+strconv.Itoa(int(m.AccessFlags)))
		}
	}
	sort.Strings(rows)
	return strings.Join(rows, "\n")
}

func nativeAnonymousNestedPrivateArgsFixture(fixture, scope string) string {
	owner := "Layer"
	if strings.Contains(scope, "depth") {
		owner = "Middle"
	}
	fixture = strings.Replace(fixture, "NestedEffects.arg(n)", "NestedEffects.arg(n^"+owner+".this.seed)", 1)
	fixture = strings.ReplaceAll(fixture, "phase.parentN!=n||", "phase.parentN!=(n^seed)||")
	old := `if(phase==null||phase.factory!=nullNamed||phase.observed!=null||phase.parentN!=0||!NestedEffects.trace.equals("AP"))throw new AssertionError("named ancestor callback order");try{phase.origin();throw new AssertionError("missing named ancestor dereference");}catch(NullPointerException wanted){}`
	next := `if(phase!=null||!NestedEffects.trace.equals(""))throw new AssertionError("named ancestor argument before constructor");`
	return strings.Replace(fixture, old, next, 1)
}

// This records physical ConstantValue tags/values, independently of the
// source proof. The JVM fixture separately checks reflection and raw FP bits.
func nativeOriginalConstantValueShape(t *testing.T, raw []byte) string {
	t.Helper()
	object, e := Parse(raw)
	if e != nil {
		t.Fatal(e)
	}
	rows := []string{}
	for _, field := range object.Fields {
		name, _ := sourceBridgeUTF8(object, field.NameIndex)
		descriptor, _ := sourceBridgeUTF8(object, field.DescriptorIndex)
		for _, attribute := range field.Attributes {
			a, ok := attribute.(*ConstantValueAttribute)
			if !ok {
				continue
			}
			value, e := object.getConstantInfo(a.ConstantValueIndex)
			if e != nil {
				t.Fatal(e)
			}
			payload := ""
			switch n := value.(type) {
			case *ConstantIntegerInfo:
				payload = fmt.Sprintf("I:%d", n.Value)
			case *ConstantLongInfo:
				payload = fmt.Sprintf("J:%d", n.Value)
			case *ConstantFloatInfo:
				payload = fmt.Sprintf("F:%08x", math.Float32bits(n.Value))
			case *ConstantDoubleInfo:
				payload = fmt.Sprintf("D:%016x", math.Float64bits(n.Value))
			case *ConstantStringInfo:
				s, known := sourceBridgeUTF8(object, n.StringIndex)
				if !known {
					t.Fatal("string constant")
				}
				payload = fmt.Sprintf("S:%q", s)
			default:
				t.Fatalf("original constant tag %T", value)
			}
			rows = append(rows, fmt.Sprintf("%s:%s:%x:%d:%s", name, descriptor, field.AccessFlags, a.AttrLen, payload))
		}
	}
	sort.Strings(rows)
	return strings.Join(rows, "\n")
}

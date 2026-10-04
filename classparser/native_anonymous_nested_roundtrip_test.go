package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
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
	for _, scope := range []string{"static-root", "dollar-root", "instance-root", "static-local", "dollar-local", "instance-local", "depth3", "depth4", "depth3-instance", "depth3-shadow", "depth3-major51", "depth3-multiple", "member-static", "member-instance", "member-depth-static", "member-depth-instance", "member-context", "member-context-depth"} {
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
			}
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
				if scope == "depth3-major51" {
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

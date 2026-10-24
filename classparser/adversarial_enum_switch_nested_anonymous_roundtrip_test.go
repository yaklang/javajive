package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func nativeEnumTableNestedAnonymousFixture(twoTables bool) string {
	f := enumConstructorMarkerFixture
	a := strings.Index(f, " SwitchBridgeOp make(")
	if a < 0 {
		panic("missing authored factory")
	}
	b := strings.Index(f[a:], "\n}\nclass SwitchBridgeDriver")
	if b < 0 {
		panic("missing authored class boundary")
	}
	b += a
	method := f[a:b]
	f = f[:a] + " final class Maker {\n" + method + "\n }\n SwitchBridgeOp make(final long seed,final Object token){return new Maker().make(seed,token);}\n" + f[b:]
	if twoTables {
		f = "enum ExtraBridgeMode {FIRST,LAST}\n" + f
		f = strings.Replace(f, " long apply(SwitchBridgeMode", " static int extra(ExtraBridgeMode mode){switch(mode){case FIRST:return 17;default:return -9;}}\n long apply(SwitchBridgeMode", 1)
		f = strings.Replace(f, "SwitchBridgeEffects.fail=false;", "if(SwitchBridgeOwner.extra(ExtraBridgeMode.FIRST)!=17||SwitchBridgeOwner.extra(ExtraBridgeMode.LAST)!=-9)throw new AssertionError(\"second table\");SwitchBridgeEffects.fail=false;", 1)
	}
	return f
}

// The root owns a switch cache/private-constructor marker, while its named
// member owns the executable anonymous body. The unchanged binary caller
// observes pre-super capture, private binding, overflow and abrupt publication.
func TestAdversarialEnumSwitchArtifactPreservesNestedAnonymousOwnership(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("actual javac8 required")
	}
	for _, compiler := range []struct {
		name, javac string
		profile     SourceCompilerProfile
	}{{"native8", native, NativeJavac8}, {"modern8", modern, ModernJavac}} {
		for _, rename := range []string{"SwitchBridge", "ChangedBridge"} {
			for _, twoTables := range []bool{false, true} {
				shape := "one-table"
				if twoTables {
					shape = "two-tables"
				}
				t.Run(compiler.name+"/"+rename+"/"+shape, func(t *testing.T) {
					fixture := strings.ReplaceAll(nativeEnumTableNestedAnonymousFixture(twoTables), "SwitchBridge", rename)
					compile := func(debug string) map[string][]byte {
						if compiler.profile == NativeJavac8 {
							return nativePrivateEnumCompile(t, fixture, rename+"Owner", debug)
						}
						return nativeCompileDebugClasses(t, fixture, debug)
					}
					testNativeIndependentCompilerFamilyFixture(t, compile, compiler.profile, compiler.javac, []string{rename + "Owner"}, rename+"Driver", "900:switch:constructor:shared-marker:overflow:identity:effects\n", nil, nativeLexicalExactSignatures)
				})
			}
		}
	}
}

const enumArtifactAssertionFixture = `enum ArtifactMode{ADD,SUB,XOR}
abstract class ArtifactParent{final Object seen;ArtifactParent(){seen=identity();}abstract Object identity();abstract long read(long n);void base(long n){assert n>=0:"parent";}}
class ArtifactOwner{
 private static final class Cell{final long word;private Cell(long word){this.word=word;}}
 static long apply(ArtifactMode mode,long a,long b){switch(mode){case ADD:return a+b;case SUB:return a-b;default:return a^b;}}
 class Maker{ArtifactParent make(final long delta,final Object token,final ArtifactMode mode){final Cell cell=new Cell(delta);return new ArtifactParent(){Object identity(){return token;}long read(long n){assert n>=0:"input";return apply(mode,cell.word,n);}};}}
 ArtifactParent make(long delta,Object token,ArtifactMode mode){return new Maker().make(delta,token,mode);}
}
class ArtifactScenario{static int run(boolean enabled){int rows=0;Object identity=new Object();for(long delta:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(ArtifactMode mode:ArtifactMode.values())for(Object token:new Object[]{null,identity}){ArtifactParent value=new ArtifactOwner().make(delta,token,mode);if(value.seen!=token||value.identity()!=token)throw new AssertionError("pre-super capture");try{long got=value.read(n);if(enabled&&n<0)throw new IllegalStateException("missing assertion");long expected=mode==ArtifactMode.ADD?java.math.BigInteger.valueOf(delta).add(java.math.BigInteger.valueOf(n)).longValue():mode==ArtifactMode.SUB?java.math.BigInteger.valueOf(delta).subtract(java.math.BigInteger.valueOf(n)).longValue():delta^n;if(got!=expected)throw new IllegalStateException("arithmetic");}catch(AssertionError failure){if(!enabled||n>=0||!"input".equals(failure.getMessage()))throw new IllegalStateException("assertion scope/message",failure);}rows++;}try{new ArtifactOwner().make(0,null,ArtifactMode.ADD).base(-1);if(!enabled)throw new IllegalStateException("missing parent assertion");}catch(AssertionError failure){if(enabled||!"parent".equals(failure.getMessage()))throw new IllegalStateException("parent scope/message",failure);}return rows;}}
class ArtifactDriver{public static void main(String[]args)throws Exception{int rows=0;java.net.URL url=ArtifactDriver.class.getProtectionDomain().getCodeSource().getLocation();for(boolean enabled:new boolean[]{false,true}){try(java.net.URLClassLoader loader=new java.net.URLClassLoader(new java.net.URL[]{url},null)){loader.setDefaultAssertionStatus(false);loader.setClassAssertionStatus("ArtifactOwner",enabled);loader.setClassAssertionStatus("ArtifactOwner$Maker$1",!enabled);loader.setClassAssertionStatus("ArtifactParent",!enabled);Class<?> scenario=Class.forName("ArtifactScenario",true,loader);java.lang.reflect.Method run=scenario.getDeclaredMethod("run",boolean.class);run.setAccessible(true);rows+=(Integer)run.invoke(null,enabled);}}System.out.println(rows+":artifact:lexical:assertions:loader:overflow:pre-super");}}
`

// Independent class loaders make the outer owner's assertion policy opposite
// to both the anonymous class and its parent. Flattening an assert into the
// wrong owner cannot accidentally pass merely because the whole VM uses -ea.
func TestAdversarialEnumSwitchNestedAnonymousPreservesAssertionOwner(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("actual javac8 required")
	}
	for _, compiler := range []struct {
		name, javac string
		profile     SourceCompilerProfile
	}{{"native8", native, NativeJavac8}, {"modern8", modern, ModernJavac}} {
		for _, rename := range []string{"Artifact", "RenamedArtifact"} {
			t.Run(compiler.name+"/"+rename, func(t *testing.T) {
				fixture := strings.ReplaceAll(enumArtifactAssertionFixture, "Artifact", rename)
				compile := func(debug string) map[string][]byte {
					if compiler.profile == NativeJavac8 {
						return nativePrivateEnumCompile(t, fixture, rename+"Owner", debug)
					}
					return nativeCompileDebugClasses(t, fixture, debug)
				}
				testNativeIndependentCompilerFamilyFixture(t, compile, compiler.profile, compiler.javac, []string{rename + "Owner"}, rename+"Driver", "300:artifact:lexical:assertions:loader:overflow:pre-super\n", nil, nativeLexicalExactSignatures)
			})
		}
	}
}

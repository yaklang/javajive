package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// The unchanged caller observes the compiler flag during a reentrant static
// producer, then both assertion paths and initializer failure/retry behavior.
// Reconstructing assertions must retain the original compiler prefix before
// every ordinary static effect, even when the initializer has a nonempty tail.
const standaloneAssertionMixedFixture = `class MixedAssertionEffects {
 static String trace="";static final RuntimeException token=new IllegalStateException("producer");
 static int produce()throws Exception{trace+="P";java.lang.reflect.Field f=MixedAssertionOwner.class.getDeclaredField("$assertionsDisabled");f.setAccessible(true);if(!f.isSynthetic()||f.getModifiers()!=0x1018||f.getBoolean(null)==ENABLED)throw new AssertionError("flag prefix/metadata");if(FAIL)throw token;return 7;}
 static boolean condition(int x){trace+="C";return x>=0;}
 static String message(){trace+="M";return "condition";}
}
class MixedAssertionOwner {
 static final int value;
 static{try{value=MixedAssertionEffects.produce();}catch(RuntimeException e){throw e;}catch(Exception e){throw new RuntimeException(e);}MixedAssertionEffects.trace+="B";assert MixedAssertionEffects.condition(value):MixedAssertionEffects.message();MixedAssertionEffects.trace+="E";}
 static int check(int x){assert MixedAssertionEffects.condition(x):MixedAssertionEffects.message();return value+x;}
}
class MixedAssertionDriver{public static void main(String[]a)throws Exception{
 ClassLoader.getSystemClassLoader().setClassAssertionStatus("MixedAssertionOwner",ENABLED);MixedAssertionEffects.trace="";
 if(FAIL){try{MixedAssertionOwner.check(1);throw new AssertionError("missing init failure");}catch(ExceptionInInitializerError e){if(e.getCause()!=MixedAssertionEffects.token)throw new AssertionError("original failure identity",e);}if(!MixedAssertionEffects.trace.equals("P"))throw new AssertionError("init failure effects");try{MixedAssertionOwner.check(1);throw new AssertionError("missing erroneous class");}catch(NoClassDefFoundError expected){}if(!MixedAssertionEffects.trace.equals("P"))throw new AssertionError("initializer repeated");}
 else{if(MixedAssertionOwner.check(1)!=8||!MixedAssertionEffects.trace.equals(ENABLED?"PBCEC":"PBE"))throw new AssertionError("init and method order:"+MixedAssertionEffects.trace);MixedAssertionEffects.trace="";try{if(MixedAssertionOwner.check(-1)!=6||ENABLED)throw new AssertionError("missing assertion");}catch(AssertionError e){if(!ENABLED||!"condition".equals(e.getMessage()))throw e;}if(!MixedAssertionEffects.trace.equals(ENABLED?"CM":""))throw new AssertionError("disabled condition/message effects");}
 System.out.println("mixed-assertions:prefix:tail:metadata:effects:failure-retry");}}
`

func TestAdversarialStandaloneAssertionMixedInitializationRoundTrip(t *testing.T) {
	for _, renamed := range []bool{false, true} {
		t.Run(fmt.Sprintf("renamed=%t", renamed), func(t *testing.T) {
			for _, enabled := range []bool{false, true} {
				for _, fail := range []bool{false, true} {
					t.Run(fmt.Sprintf("enabled=%t/fail=%t", enabled, fail), func(t *testing.T) {
						fixture := strings.NewReplacer("ENABLED", fmt.Sprint(enabled), "FAIL", fmt.Sprint(fail)).Replace(standaloneAssertionMixedFixture)
						owner := "MixedAssertionOwner"
						if renamed {
							fixture = strings.ReplaceAll(fixture, owner, "RenamedInitializerScope")
							owner = "RenamedInitializerScope"
						}
						testNativePrivateSetterFixture(t, fixture, owner, "MixedAssertionDriver", "mixed-assertions:prefix:tail:metadata:effects:failure-retry\n")
					})
				}
			}
		})
	}
}

func TestAdversarialStandaloneAssertionMixedInitializationNativeCompiler(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Fatal("JAVA8_JAVAC required for independent native assertion input")
	}
	for _, enabled := range []bool{false, true} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("enabled=%t/fail=%t", enabled, fail), func(t *testing.T) {
				fixture := strings.NewReplacer("ENABLED", fmt.Sprint(enabled), "FAIL", fmt.Sprint(fail)).Replace(standaloneAssertionMixedFixture)
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return nativePrivateEnumCompile(t, fixture, "MixedAssertionOwner", debug)
				}, NativeJavac8, javac, []string{"MixedAssertionOwner"}, "MixedAssertionDriver", "mixed-assertions:prefix:tail:metadata:effects:failure-retry\n", nil, nativeLexicalExactSignatures)
			})
		}
	}
}

func TestNativeStandaloneAssertionMixedInitializerClearsFailedRecheck(t *testing.T) {
	fixture := strings.NewReplacer("ENABLED", "true", "FAIL", "false").Replace(standaloneAssertionMixedFixture)
	files := nativeCompileDebugClasses(t, fixture, "none")
	obj, err := Parse(files["MixedAssertionOwner.class"])
	if err != nil {
		t.Fatal(err)
	}
	d := NewClassObjectDumper(obj)
	source, err := d.DumpClass()
	if err != nil || d.nativeStandaloneAssertion == nil || !d.nativeStandaloneAssertionClosed(source) {
		t.Fatal("original complete mixed initialization", err)
	}
	// A failed second projection must not borrow the earlier successful
	// initializer occurrence, even when all method reads remain consumed.
	if _, err := d.prepareNativeAssertions("<clinit>", "()V", nil); err == nil {
		t.Fatal("missing original initializer source admitted")
	}
	if d.nativeStandaloneAssertionClosed(source) {
		t.Fatal("failed initializer recheck retained earlier source certificate")
	}
}

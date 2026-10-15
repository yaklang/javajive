package javaclassparser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const nativeEnumDelegationFixture = `class DelegateEffects {static String trace="";}
enum DelegateMode {
 FIRST, SECOND(7), THIRD(5,11L);
 final int code; final long salt;
 DelegateMode(){this(13);}
 DelegateMode(int code){this(code,19L);}
 DelegateMode(int code,long salt){this.code=code;this.salt=salt;DelegateEffects.trace+=code+":"+salt+";";}
}
abstract class DelegateParent {
 final Object seen; final long number;
 DelegateParent(Object input,long number){this.number=number;this.seen=read();if(this.seen!=input)throw new AssertionError("capture before parent");}
 abstract Object read(); abstract int choose();
}
class DelegateSwitchOwner {
 DelegateParent make(final DelegateMode mode,final Object token){return new DelegateParent(token,mode.salt){Object read(){return token;}int choose(){switch(mode){case THIRD:return 31;case FIRST:return 17;default:return 23;}}};}
}
class DelegateDriver {
 public static void main(String[]args)throws Exception {
  DelegateMode[] modes=DelegateMode.values();
  if(!DelegateEffects.trace.equals("13:19;7:19;5:11;"))throw new AssertionError("delegation effects");
  int[] codes={13,7,5};long[] salts={19,19,11};int[] picks={17,23,31};int rows=0;
  DelegateSwitchOwner owner=new DelegateSwitchOwner();Object object=new Object();
  for(int i=0;i<modes.length;i++)for(Object token:new Object[]{null,object,new int[]{3}}){
   DelegateMode mode=modes[i];DelegateParent value=owner.make(mode,token);
   if(mode.ordinal()!=i||DelegateMode.valueOf(mode.name())!=mode||mode.code!=codes[i]||mode.salt!=salts[i]||value.number!=salts[i]||value.seen!=token||value.read()!=token||value.choose()!=picks[i])throw new AssertionError("enum/capture identity");
   if(value.getClass().getEnclosingClass()!=DelegateSwitchOwner.class||!value.getClass().isAnonymousClass())throw new AssertionError("original ownership");
   rows++;
  }
  if(DelegateMode.class.getDeclaredConstructors().length!=3)throw new AssertionError("constructor ABI");
  System.out.println(rows+":enum:delegation:effects:external-switch:captures");
 }
}`

func TestNativeEnumConstructorDelegationExternalSwitchRoundTrip(t *testing.T) {
	javac, _ := t04Tools(t)
	testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
		return nativeCompileSourceReleaseClasses(t, map[string]string{"DelegateSwitchOwner.java": nativeEnumDelegationFixture}, debug, "8")
	}, ModernJavac, javac, []string{"DelegateMode", "DelegateSwitchOwner"}, "DelegateDriver", "9:enum:delegation:effects:external-switch:captures\n", nil, nativeLexicalExactSignatures)
}

func TestNativeEnumConstructorDelegationClosesGeneratedProtocol(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"DelegateSwitchOwner.java": nativeEnumDelegationFixture}, "none", "8")
	obj, err := Parse(files["DelegateMode.class"])
	if err != nil {
		t.Fatal(err)
	}
	if nativeMemberEnumSynthesisProof(obj, 0x4019, nil) == nil {
		t.Fatal("original this-constructor chain lost the enum generation certificate")
	}
}

func nativeEnumDelegationVariant(variant string) (string, []string) {
	source := nativeEnumDelegationFixture
	owners := []string{"DelegateMode", "DelegateSwitchOwner"}
	switch variant {
	case "renamed":
		source = strings.NewReplacer("DelegateMode", "ReboundChoice", "DelegateSwitchOwner", "ReboundScope", "choose()", "selectValue()", "make(", "create(").Replace(source)
		owners = []string{"ReboundChoice", "ReboundScope"}
	case "conditional":
		source = strings.NewReplacer(
			"DelegateMode(){this(13);}", "DelegateMode(){this(-13);}",
			"DelegateMode(int code){this(code,19L);}", "DelegateMode(int code){this(code,code<0?11L:19L);}",
			"13:19;7:19;5:11;", "-13:11;7:19;5:11;",
			"{13,7,5}", "{-13,7,5}", "{19,19,11}", "{11,19,11}",
		).Replace(source)
	case "varargs array":
		source = strings.Replace(source, "DelegateMode(int code){this(code,19L);}", "DelegateMode(int... codes){this(codes[0],19L);}", 1)
	case "fresh allocation":
		source = strings.NewReplacer(
			"SECOND(7)", "SECOND(new DelegateBox(7))",
			"DelegateMode(){this(13);}", "DelegateMode(){this(new DelegateBox(13));}",
			"DelegateMode(int code){this(code,19L);}", "DelegateMode(DelegateBox box){this(box.value,19L);}",
		).Replace(source) + "class DelegateBox{final int value;DelegateBox(int value){this.value=value;}}"
	case "null overload binding":
		source = strings.NewReplacer(
			"SECOND(7)", "SECOND((String)null)",
			"DelegateMode(){this(13);}", "DelegateMode(){this((Object)null);}",
			"DelegateMode(int code){this(code,19L);}", "DelegateMode(Object unused){this(13,19L);}DelegateMode(String unused){this(7,11L);}",
			"13:19;7:19;5:11;", "13:19;7:11;5:11;", "{19,19,11}", "{19,11,11}",
			"getDeclaredConstructors().length!=3", "getDeclaredConstructors().length!=4",
		).Replace(source)
	case "parameterized null binding":
		source = strings.NewReplacer(
			"SECOND(7)", "SECOND((java.util.ArrayList<String>)null)",
			"DelegateMode(){this(13);}", "DelegateMode(){this((java.util.List<String>)null);}",
			"DelegateMode(int code){this(code,19L);}", "DelegateMode(java.util.List<String> unused){this(13,19L);}DelegateMode(java.util.ArrayList<String> unused){this(7,11L);}",
			"13:19;7:19;5:11;", "13:19;7:11;5:11;", "{19,19,11}", "{19,11,11}",
			"getDeclaredConstructors().length!=3", "getDeclaredConstructors().length!=4",
		).Replace(source)
	case "double payload":
		source = strings.NewReplacer(
			"SECOND(7)", "SECOND(7.0)", "DelegateMode(){this(13);}", "DelegateMode(){this(13.0);}",
			"DelegateMode(int code){this(code,19L);}", "DelegateMode(double code){this((int)code,19L);}",
		).Replace(source)
	case "class literal":
		source = strings.NewReplacer(
			"SECOND(7)", "SECOND(DelegateMode.class)", "DelegateMode(){this(13);}", "DelegateMode(){this(String[].class);}",
			"DelegateMode(int code){this(code,19L);}", "DelegateMode(Class<?> type){this(type==String[].class?13:7,19L);}",
		).Replace(source)
	case "string payload":
		source = strings.NewReplacer(
			"SECOND(7)", "SECOND(\"xx\")", "DelegateMode(){this(13);}", "DelegateMode(){this(\"thirteen\");}",
			"DelegateMode(int code){this(code,19L);}", "DelegateMode(String word){this(word.length()+5,19L);}",
		).Replace(source)
	case "member enum":
		source = strings.Replace(source, "enum DelegateMode {", "class DelegateEnumContainer{enum DelegateMode {", 1)
		source = strings.Replace(source, "}\nabstract class DelegateParent", "}}\nabstract class DelegateParent", 1)
		start := strings.Index(source, "abstract class DelegateParent")
		source = source[:start] + strings.ReplaceAll(source[start:], "DelegateMode", "DelegateEnumContainer.DelegateMode")
		owners = []string{"DelegateEnumContainer", "DelegateSwitchOwner"}
	}
	return source, owners
}

func TestNativeEnumConstructorDelegationArgumentDomainsRoundTrip(t *testing.T) {
	javac, _ := t04Tools(t)
	for _, variant := range []string{"renamed", "conditional", "varargs array", "fresh allocation", "null overload binding", "parameterized null binding", "double payload", "class literal", "string payload", "member enum"} {
		t.Run(variant, func(t *testing.T) {
			source, owners := nativeEnumDelegationVariant(variant)
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, map[string]string{"DelegateSwitchOwner.java": source}, debug, "8")
			}, ModernJavac, javac, owners, "DelegateDriver", "9:enum:delegation:effects:external-switch:captures\n", nil, nativeLexicalExactSignatures)
		})
	}
}

// Use real Java5 and Java8 originals, rather than changing class-version words.
// Compiler metadata and the ordinary non-parameterized constructor tail still
// pass the same ABI/Signature and original-first runtime gates.
func TestNativeEnumConstructorDelegationOriginalCompilerRoundTrip(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Fatal("JAVA8_JAVAC required for the independent original compiler")
	}
	for _, level := range []string{"5", "8"} {
		for _, variant := range []string{"original", "varargs array"} {
			t.Run(level+"/"+variant, func(t *testing.T) {
				source, owners := nativeEnumDelegationVariant(variant)
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					if level == "5" {
						return nativeCompileJava56(t, source, debug, level)
					}
					files := localGetterCompileJavac8(t, map[string]string{"DelegateSwitchOwner.java": source}, debug)
					for _, raw := range files {
						obj, err := Parse(raw)
						if err != nil || obj.MajorVersion != 52 || obj.MinorVersion != 0 {
							t.Fatal("actual native Java8 class required", err)
						}
					}
					return files
				}, NativeJavac8, javac, owners, "DelegateDriver", "9:enum:delegation:effects:external-switch:captures\n", nil, nativeLexicalExactSignatures)
			})
		}
	}
}

func TestNativeEnumConstructorDelegationParameterMetadataFallbackRoundTrip(t *testing.T) {
	// These switches disable heuristic signature/overload paths. Exact original
	// declaration evidence must still protect a source enum's payload binding.
	t.Setenv("JDEC_CTOR_WILDCARD_CAST_OFF", "1")
	t.Setenv("JDEC_THIS_CTOR_OVERLOAD_CAST_OFF", "1")
	source, owners := nativeEnumDelegationVariant("parameterized null binding")
	javac, _ := t04Tools(t)
	testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
		return nativeCompileSourceReleaseClasses(t, map[string]string{"DelegateSwitchOwner.java": source}, debug, "8")
	}, ModernJavac, javac, owners, "DelegateDriver", "9:enum:delegation:effects:external-switch:captures\n", nil, nativeLexicalExactSignatures)
}

func TestNativeEnumConstructorDelegationKeepsMethodBoundInferenceUnproved(t *testing.T) {
	source, _ := nativeEnumDelegationVariant("null overload binding")
	source = strings.Replace(source, "DelegateMode(Object unused)", "<T> DelegateMode(T unused)", 1)
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"DelegateSwitchOwner.java": source}, "none", "8")
	original := t.TempDir()
	for name, raw := range files {
		if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	_, java := t04Tools(t)
	if got := t04RunJava(t, java, original, "DelegateDriver"); got != "9:enum:delegation:effects:external-switch:captures\n" {
		t.Fatalf("original generic-constructor counterexample: %q", got)
	}
	obj, err := Parse(files["DelegateMode.class"])
	if err != nil {
		t.Fatal(err)
	}
	if nativeMemberEnumSynthesisProof(obj, 0x4019, nil) != nil {
		t.Fatal("hidden-role identity does not prove constructor method-bound inference")
	}
}

package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// JVM blank-final stores are legal in separate terminal exception arms even
// when javac's definite-assignment rules require a joined local. Exercise all
// primitive widths against an unchanged binary caller, including raw NaN and
// signed-zero bits, handler side effects and erroneous-class reuse.
func TestAdversarialInitializerFinalPrimitiveArmsPreserveBitsAndFailures(t *testing.T) {
	modern, _ := t04Tools(t)
	types := []struct {
		name           string
		values, checks [3]string
	}{
		{"boolean", [3]string{"true", "false", "true"}, [3]string{"true", "false", "true"}},
		{"byte", [3]string{"(byte)127", "(byte)-128", "(byte)-1"}, [3]string{"127", "-128", "-1"}},
		{"char", [3]string{"(char)65535", "(char)0", "(char)55296"}, [3]string{"65535", "0", "55296"}},
		{"short", [3]string{"(short)32767", "(short)-32768", "(short)-1"}, [3]string{"32767", "-32768", "-1"}},
		{"int", [3]string{"Integer.MIN_VALUE", "Integer.MAX_VALUE", "-1"}, [3]string{"Integer.MIN_VALUE", "Integer.MAX_VALUE", "-1"}},
		{"long", [3]string{"Long.MIN_VALUE", "Long.MAX_VALUE", "-1L"}, [3]string{"Long.MIN_VALUE", "Long.MAX_VALUE", "-1L"}},
		{"float", [3]string{"Float.intBitsToFloat(0x80000000)", "Float.intBitsToFloat(0x7fc12345)", "Float.intBitsToFloat(1)"}, [3]string{"0x80000000", "0x7fc12345", "1"}},
		{"double", [3]string{"Double.longBitsToDouble(0x8000000000000000L)", "Double.longBitsToDouble(0x7ff8123456789abcL)", "Double.longBitsToDouble(1L)"}, [3]string{"0x8000000000000000L", "0x7ff8123456789abcL", "1L"}},
	}
	var source, helpers, driver, expected strings.Builder
	helpers.WriteString(`class FinalStoreProbe {static String trace=""; static final RuntimeException FAILURE=new IllegalStateException("same");`)
	driver.WriteString(`public class FinalStoreDriver {public static void main(String[] args){`)
	var owners []string
	for i, typ := range types {
		fmt.Fprintf(&helpers, `static %s pick%d(int mode){trace+="P%d:"+mode+";";if(mode==1)throw new IllegalArgumentException();if(mode==2)throw new SecurityException();if(mode==3)throw FAILURE;return %s;}
static %s recover%d(int mode){trace+="H%d:"+mode+";";return mode==1?%s:%s;}
`, typ.name, i, i, typ.values[0], typ.name, i, i, typ.values[1], typ.values[2])
		for mode := 0; mode < 4; mode++ {
			owner := fmt.Sprintf("FinalStoreCase%d_%d", i, mode)
			owners = append(owners, owner)
			fmt.Fprintf(&source, `class %s {static %s value;static{try{value=FinalStoreProbe.pick%d(%d);}catch(IllegalArgumentException unused){value=FinalStoreProbe.recover%d(1);}catch(SecurityException unused){value=FinalStoreProbe.recover%d(2);}}}
`, owner, typ.name, i, mode, i, i)
			fmt.Fprintf(&expected, "P%d:%d;", i, mode)
			if mode == 3 {
				fmt.Fprintf(&driver, `try{%s ignored=%s.value;throw new AssertionError("missing error");}catch(ExceptionInInitializerError failure){if(failure.getCause()!=FinalStoreProbe.FAILURE)throw new AssertionError("exception identity");}try{%s ignored=%s.value;throw new AssertionError("erroneous class reused");}catch(NoClassDefFoundError expected){}
`, typ.name, owner, typ.name, owner)
				continue
			}
			if mode > 0 {
				fmt.Fprintf(&expected, "H%d:%d;", i, mode)
			}
			value := owner + ".value"
			if typ.name == "float" {
				value = "Float.floatToRawIntBits(" + value + ")"
			}
			if typ.name == "double" {
				value = "Double.doubleToRawLongBits(" + value + ")"
			}
			fmt.Fprintf(&driver, `if(%s!=%s)throw new AssertionError("primitive bits %d/%d");`+"\n", value, typ.checks[mode], i, mode)
		}
	}
	helpers.WriteString("}\n")
	fmt.Fprintf(&driver, `if(!FinalStoreProbe.trace.equals("%s"))throw new AssertionError(FinalStoreProbe.trace);System.out.println("32:primitive-bits:handler-effects:error-identity");}}`, expected.String())
	fixture := source.String() + helpers.String() + driver.String()
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Fatal("native original compiler required")
				}
			}
			compile := func(debug string) map[string][]byte {
				files := compileLambdaParameterMetadataFixture(t, compiler, profile, fixture, "FinalStoreDriver", debug, false)
				for _, owner := range owners {
					obj := mustParseLowering(t, files[owner+".class"])
					if len(obj.Fields) != 1 {
						t.Fatal("authored field inventory")
					}
					obj.Fields[0].AccessFlags |= 0x10
					files[owner+".class"] = obj.Bytes()
				}
				return files
			}
			testNativeIndependentCompilerFamilyWithPolicies(t, compile, profile, compiler, owners, "FinalStoreDriver", "32:primitive-bits:handler-effects:error-identity\n", nil, nil, []string{"normal", "no-source-rewrites"})
		})
	}
}

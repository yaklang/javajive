package javaclassparser

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Promoted regression from frozen independent holdout seed87ecd9f218c331c7.
// Generic method scopes formerly exhausted the default request graph budget
// because each unrelated throw repeatedly parsed its entire method signature.
// Four branch/loop/finally skeletons preserve quiet-NaN raw words and effects.
// 100 numeric scenarios, not 7 names/debug/policy copies of each scenario.
func TestAdversarialRuntimeNaNControlFlowGenericNamespaces(t *testing.T) {
	seedText := "87ecd9f218c331c7"
	seed, err := strconv.ParseUint(seedText, 16, 64)
	if err != nil {
		t.Fatal(err)
	}
	javac, java := t04Tools(t)
	for _, namespace := range []string{"plain", "package_types", "package_root_type", "package_root_value_and_formal", "method_type_formals", "class_type_formals", "package_root_type_formals"} {
		t.Run(namespace, func(t *testing.T) {
			state := seed
			next := func() uint64 { state ^= state << 13; state ^= state >> 7; state ^= state << 17; return state }
			replacements := map[uint64]uint64{}
			var source, driver, want strings.Builder
			header := "class HeldoutWords{"
			methodPrefix, callPrefix, driverPrelude := "static ", "HeldoutWords.", ""
			switch namespace {
			case "method_type_formals":
				methodPrefix = "static <Float,Double,java> "
			case "class_type_formals":
				header = "class HeldoutWords<Float,Double,java>{"
				methodPrefix, callPrefix = "", "held."
				driverPrelude = "HeldoutWords<Object,Object,Object> held=new HeldoutWords<Object,Object,Object>();"
			case "package_root_type_formals":
				header = "class java{static class lang{static class Float{static float intBitsToFloat(int x){throw new AssertionError(\"wrong float owner\");}}static class Double{static double longBitsToDouble(long x){throw new AssertionError(\"wrong double owner\");}}}}class HeldoutWords{"
				methodPrefix = "static <Float,Double> "
			case "package_types":
				header = "class Float{}class Double{}class HeldoutWords{static int Float=17,Double=31;"
			case "package_root_type":
				header = "class java{}class HeldoutWords{static int Float=17,Double=31;"
			case "package_root_value_and_formal":
				header = "class HeldoutWords<Double>{static int java=29;"
			}
			source.WriteString(header + "static int trace;\n")
			driver.WriteString("import java.lang.Float;import java.lang.Double;class HeldoutDriver{public static void main(String[]args){\n" + driverPrelude)
			for i := 0; i < 100; i++ {
				for _, typ := range []string{"float", "double"} {
					name := fmt.Sprintf("m%s%d", typ, i)
					left, right := uint64(0x3fa00000+i*2), uint64(0x3fa00001+i*2)
					qleft, qright := uint64(0x7fc00000)|next()&0x003fffff, uint64(0x7fc00000)|next()&0x003fffff
					if next()&1 != 0 {
						qleft |= 0x80000000
					}
					if next()&1 != 0 {
						qright |= 0x80000000
					}
					leftText := strconv.FormatFloat(float64(math.Float32frombits(uint32(left))), 'x', -1, 32) + "F"
					rightText := strconv.FormatFloat(float64(math.Float32frombits(uint32(right))), 'x', -1, 32) + "F"
					if typ == "double" {
						left, right = uint64(0x3ff4000000000000)+uint64(i*2), uint64(0x3ff4000000000001)+uint64(i*2)
						qleft, qright = 0x7ff8000000000000|next()&0x0007ffffffffffff, 0x7ff8000000000000|next()&0x0007ffffffffffff
						if next()&1 != 0 {
							qleft |= 0x8000000000000000
						}
						if next()&1 != 0 {
							qright |= 0x8000000000000000
						}
						leftText = strconv.FormatFloat(math.Float64frombits(left), 'x', -1, 64) + "D"
						rightText = strconv.FormatFloat(math.Float64frombits(right), 'x', -1, 64) + "D"
					}
					replacements[left] = qleft
					replacements[right] = qright
					body := "return choose?" + leftText + ":" + rightText + ";"
					switch i % 4 {
					case 1:
						body = fmt.Sprintf("%s x;if(choose){x=%s;trace=1;}else{x=%s;trace=2;}return x;", typ, leftText, rightText)
					case 2:
						body = "try{if(choose)return " + leftText + ";return " + rightText + ";}finally{trace=3;}"
					case 3:
						body = fmt.Sprintf("%s[] a=new %s[]{%s,%s};for(int j=0;j<2;j++){if(j==(choose?0:1)){trace=trace*10+j+4;return a[j];}}throw new AssertionError(\"unreachable\");", typ, typ, leftText, rightText)
					}
					fmt.Fprintf(&source, "%s%s %s(boolean choose){%s}\n", methodPrefix, typ, name, body)
					raw, box := "floatToRawIntBits", "Float"
					if typ == "double" {
						raw, box = "doubleToRawLongBits", "Double"
					}
					integer := "Integer"
					if typ == "double" {
						integer = "Long"
					}
					for _, choose := range []bool{false, true} {
						trace := 0
						switch i % 4 {
						case 1:
							trace = 2
							if choose {
								trace = 1
							}
						case 2:
							trace = 3
						case 3:
							trace = 5
							if choose {
								trace = 4
							}
						}
						word := qright
						if choose {
							word = qleft
						}
						fmt.Fprintf(&driver, "HeldoutWords.trace=0;System.out.println(%s.toHexString(%s.%s(%s%s(%t)))+\":\"+HeldoutWords.trace);\n", integer, box, raw, callPrefix, name, choose)
						fmt.Fprintf(&want, "%x:%d\n", word, trace)
					}
				}
			}
			source.WriteString("}")
			driver.WriteString("}}")
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					files := nativeCompileSourceReleaseClasses(t, map[string]string{"HeldoutWords.java": source.String(), "HeldoutDriver.java": driver.String()}, debug, "8")
					obj, err := Parse(files["HeldoutWords.class"])
					if err != nil {
						t.Fatal(err)
					}
					replaced := 0
					for _, cp := range obj.ConstantPool {
						switch cp := cp.(type) {
						case *ConstantFloatInfo:
							if word, ok := replacements[uint64(math.Float32bits(cp.Value))]; ok {
								cp.Value = math.Float32frombits(uint32(word))
								replaced++
							}
						case *ConstantDoubleInfo:
							if word, ok := replacements[math.Float64bits(cp.Value)]; ok {
								cp.Value = math.Float64frombits(word)
								replaced++
							}
						}
					}
					if replaced != 400 {
						t.Fatalf("original constant bank changed %d, want400", replaced)
					}
					files["HeldoutWords.class"] = obj.Bytes()
					original := t.TempDir()
					for name, raw := range files {
						if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					if got := t04RunJava(t, java, original, "HeldoutDriver"); got != want.String() {
						t.Fatalf("original heldout oracle differs: %s", got)
					}
					for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
						t.Run(string(mode), func(t *testing.T) {
							resolve := func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return raw, ok }
							var text string
							var err error
							if mode == "legacy" {
								text, err = DecompileWithResolver(files["HeldoutWords.class"], resolve)
							} else {
								result, e := DecompileWithOptions(files["HeldoutWords.class"], DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
								text, err = result.Source, e
							}
							if err != nil {
								t.Fatal(err)
							}
							rebuilt := t.TempDir()
							for name, raw := range files {
								if name == "HeldoutWords.class" {
									continue
								}
								if err := os.WriteFile(filepath.Join(rebuilt, name), raw, 0600); err != nil {
									t.Fatal(err)
								}
							}
							path := filepath.Join(rebuilt, "HeldoutWords.java")
							if err := os.WriteFile(path, []byte(text), 0600); err != nil {
								t.Fatal(err)
							}
							if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", rebuilt, "-d", rebuilt, path).CombinedOutput(); err != nil {
								t.Fatalf("heldout candidate compile: %v %s", err, out)
							}
							if got := t04RunJava(t, java, rebuilt, "HeldoutDriver"); got != want.String() {
								t.Fatalf("heldout compiled source differs: %s", got)
							}
						})
					}
				})
			}
		})
	}
}

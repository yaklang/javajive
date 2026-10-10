package javaclassparser

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The expected raw words come from a finite bit-pattern bank, not from the
// renderer. Change only the original constant-pool words; run that class first
// against the unchanged driver. Quiet NaNs are used because signaling NaNs can
// be quieted by hardware during a copy on some supported JVM platforms.
func TestAdversarialQuietNaNConstantsKeepSignPayloadAndEffects(t *testing.T) {
	testQuietNaNConstantBank(t, false, false)
}

func TestAdversarialNaNIntrinsicOwnerAvoidsPackageRootShadow(t *testing.T) {
	testQuietNaNConstantBank(t, true, false)
}

func TestAdversarialNaNIntrinsicOwnerAvoidsValueNameShadow(t *testing.T) {
	testQuietNaNConstantBank(t, false, true)
}

func testQuietNaNConstantBank(t *testing.T, rootShadow, valueShadow bool) {
	javac, java := t04Tools(t)
	type replacement struct{ finite, quiet uint64 }
	var fs, ds []replacement
	var source, driver, want strings.Builder
	if rootShadow {
		source.WriteString("import java.lang.Float;import java.lang.Double;class java{}\n")
	} else if !valueShadow {
		source.WriteString("class Float{}class Double{}\n")
	}
	source.WriteString("class NaNConstantProbe {static int trace;static volatile float vf;static volatile double vd;static float mark(float x){trace=trace*10+1;return x;}static double mark(double x){trace=trace*10+1;return x;}\n")
	if valueShadow {
		source.WriteString("static int Float=0,Double=0;\n")
	}
	driver.WriteString("class NaNConstantDriver{public static void main(String[]args){\n")
	for i := 0; i < 64; i++ {
		f := uint32(0x7fc00000) | uint32((i*0x10531)&0x003fffff)
		d := uint64(0x7ff8000000000000) | uint64(i)*0x123456781
		if i&1 != 0 {
			f |= 0x80000000
			d |= 0x8000000000000000
		}
		ff, dd := uint32(0x3fa00000+i), uint64(0x3ff4000000000000)+uint64(i)
		fs = append(fs, replacement{uint64(ff), uint64(f)})
		ds = append(ds, replacement{dd, d})
		// One original constant is shared by different consumers; field writes,
		// array storage and boxing must retain the same value and effect order.
		for _, typ := range []string{"float", "double"} {
			literal, bits, wrapper, raw, array, field := strconv.FormatFloat(float64(math.Float32frombits(ff)), 'x', -1, 32)+"F", uint64(f), "java.lang.Float", "floatToRawIntBits", "float", "vf"
			if typ == "double" {
				literal = strconv.FormatFloat(math.Float64frombits(dd), 'x', -1, 64) + "D"
				bits, wrapper, raw, array, field = d, "java.lang.Double", "doubleToRawLongBits", "double", "vd"
			}
			if rootShadow {
				wrapper = strings.TrimPrefix(wrapper, "java.lang.")
			}
			for shape := 0; shape < 2; shape++ {
				name := fmt.Sprintf("m%s%d_%d", typ, i, shape)
				body := "return " + literal + ";"
				if shape == 1 {
					body = fmt.Sprintf("%s x=mark(%s);%s=x;Object box=%s.valueOf(x);%s[] a=new %s[]{x};trace=trace*10+2;if(((%s)box).%sValue()!=a[0]&&!%s.isNaN(a[0]))throw new AssertionError(\"box\");return a[0];", typ, literal, field, wrapper, array, array, wrapper, typ, wrapper)
				}
				fmt.Fprintf(&source, "static %s %s(){%s}\n", typ, name, body)
				fmt.Fprintf(&driver, "NaNConstantProbe.trace=0;System.out.println(%s.toHexString(%s.%s(NaNConstantProbe.%s()))+\":\"+NaNConstantProbe.trace);\n", map[string]string{"float": "Integer", "double": "Long"}[typ], wrapper, raw, name)
				if shape == 1 {
					fmt.Fprintf(&driver, "System.out.println(%s.toHexString(%s.%s(NaNConstantProbe.%s)));\n", map[string]string{"float": "Integer", "double": "Long"}[typ], wrapper, raw, field)
				}
				trace := 0
				if shape == 1 {
					trace = 12
				}
				fmt.Fprintf(&want, "%x:%d\n", bits, trace)
				if shape == 1 {
					fmt.Fprintf(&want, "%x\n", bits)
				}
			}
		}
	}
	source.WriteString("}\n")
	driver.WriteString("}}\n")
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileSourceReleaseClasses(t, map[string]string{"NaNConstantDriver.java": source.String() + driver.String()}, debug, "8")
			obj, err := Parse(files["NaNConstantProbe.class"])
			if err != nil {
				t.Fatal(err)
			}
			counts := [2]int{}
			for _, cp := range obj.ConstantPool {
				switch cp := cp.(type) {
				case *ConstantFloatInfo:
					for _, p := range fs {
						if uint64(math.Float32bits(cp.Value)) == p.finite {
							cp.Value = math.Float32frombits(uint32(p.quiet))
							counts[0]++
							break
						}
					}
				case *ConstantDoubleInfo:
					for _, p := range ds {
						if math.Float64bits(cp.Value) == p.finite {
							cp.Value = math.Float64frombits(p.quiet)
							counts[1]++
							break
						}
					}
				}
			}
			if counts != [2]int{64, 64} {
				t.Fatalf("original constant bank=%v", counts)
			}
			raw := obj.Bytes()
			// The class serializer's raw constant behavior is independently
			// checked before it can provide the original execution witness.
			for _, p := range fs {
				if !strings.Contains(string(raw), string(append([]byte{4}, binary.BigEndian.AppendUint32(nil, uint32(p.quiet))...))) {
					t.Fatal("original float constant lost its raw word")
				}
			}
			for _, p := range ds {
				if !strings.Contains(string(raw), string(append([]byte{6}, binary.BigEndian.AppendUint64(nil, p.quiet)...))) {
					t.Fatal("original double constant lost its raw word")
				}
			}
			original := t.TempDir()
			for name, bytes := range files {
				if name == "NaNConstantProbe.class" {
					bytes = raw
				}
				if err := os.WriteFile(filepath.Join(original, name), bytes, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got := t04RunJava(t, java, original, "NaNConstantDriver"); got != want.String() {
				t.Fatalf("original bit/effect oracle differs\n%s", got)
			}
			for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
				t.Run(string(mode), func(t *testing.T) {
					resolve := func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }
					var source string
					var err error
					if mode == "legacy" {
						source, err = DecompileWithResolver(raw, resolve)
					} else {
						result, e := DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
						source, err = result.Source, e
					}
					if err != nil {
						t.Fatal(err)
					}
					rebuilt := t.TempDir()
					// Only unchanged authored helpers accompany the candidate;
					// the original target implementation is absent from its path.
					for name, bytes := range files {
						if name == "NaNConstantProbe.class" {
							continue
						}
						if err := os.WriteFile(filepath.Join(rebuilt, name), bytes, 0600); err != nil {
							t.Fatal(err)
						}
					}
					path := filepath.Join(rebuilt, "NaNConstantProbe.java")
					if err := os.WriteFile(path, []byte(source), 0600); err != nil {
						t.Fatal(err)
					}
					if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", rebuilt, "-d", rebuilt, path).CombinedOutput(); err != nil {
						t.Fatalf("rebuilt javac: %v %s", err, out)
					}
					if got := t04RunJava(t, java, rebuilt, "NaNConstantDriver"); got != want.String() {
						t.Fatalf("compiled source changed raw words/effects\n%s", got)
					}
				})
			}
		})
	}
}

// A newly introduced intrinsic is absent from the original constant pool.
// Test type and value namespaces independently, without original wrapper calls
// accidentally populating the renderer's type-collision inventory.
func TestAdversarialNaNNewIntrinsicBindsWithoutOriginalWrapperReference(t *testing.T) {
	_, java := t04Tools(t)
	for _, profile := range []struct{ name, declarations string }{
		{"plain", ""},
		{"package_types", "class Float{}class Double{}"},
		{"wrapper_fields", ""},
		{"package_root_field", ""},
		{"package_root_type", "class java{}"},
		{"package_root_type_and_wrapper_fields", "class java{}"},
	} {
		t.Run(profile.name, func(t *testing.T) {
			fields := ""
			if strings.Contains(profile.name, "wrapper_fields") {
				fields = "static int Float=17,Double=31;"
			}
			if profile.name == "package_root_field" {
				fields = "static int java=29;"
			}
			source := profile.declarations + "class NaNBindingProbe{" + fields + "static float f(){return 1.25F;}static double d(){return 1.25D;}}"
			const driver = `import java.lang.Float;import java.lang.Double;class NaNBindingDriver{public static void main(String[]args){System.out.println(Integer.toHexString(Float.floatToRawIntBits(NaNBindingProbe.f())));System.out.println(Long.toHexString(Double.doubleToRawLongBits(NaNBindingProbe.d())));}}`
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					files := nativeCompileSourceReleaseClasses(t, map[string]string{"NaNBindingProbe.java": source, "NaNBindingDriver.java": driver}, debug, "8")
					obj, err := Parse(files["NaNBindingProbe.class"])
					if err != nil {
						t.Fatal(err)
					}
					changed := [2]int{}
					for _, constant := range obj.ConstantPool {
						switch c := constant.(type) {
						case *ConstantFloatInfo:
							if math.Float32bits(c.Value) == 0x3fa00000 {
								c.Value = math.Float32frombits(0xffc01234)
								changed[0]++
							}
						case *ConstantDoubleInfo:
							if math.Float64bits(c.Value) == 0x3ff4000000000000 {
								c.Value = math.Float64frombits(0xfff8000012345678)
								changed[1]++
							}
						case *ConstantClassInfo:
							name, _ := obj.getUtf8(c.NameIndex)
							if name == "java/lang/Float" || name == "java/lang/Double" {
								t.Fatal("target already names an intrinsic owner")
							}
						}
					}
					if changed != [2]int{1, 1} {
						t.Fatalf("original replacement bank=%v", changed)
					}
					files["NaNBindingProbe.class"] = obj.Bytes()
					original := t.TempDir()
					for name, raw := range files {
						if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					const want = "ffc01234\nfff8000012345678\n"
					if got := t04RunJava(t, java, original, "NaNBindingDriver"); got != want {
						t.Fatalf("original raw-bit oracle=%q", got)
					}
					for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
						t.Run(string(mode), func(t *testing.T) {
							resolve := func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return raw, ok }
							var output string
							if mode == "legacy" {
								output, err = DecompileWithResolver(files["NaNBindingProbe.class"], resolve)
							} else {
								var result DecompileResult
								result, err = DecompileWithOptions(files["NaNBindingProbe.class"], DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
								output = result.Source
							}
							if err != nil {
								t.Fatal(err)
							}
							rebuilt := t.TempDir()
							for name, raw := range files {
								if name == "NaNBindingProbe.class" {
									continue
								}
								if err := os.WriteFile(filepath.Join(rebuilt, name), raw, 0600); err != nil {
									t.Fatal(err)
								}
							}
							path := filepath.Join(rebuilt, "NaNBindingProbe.java")
							if err := os.WriteFile(path, []byte(output), 0600); err != nil {
								t.Fatal(err)
							}
							javac, _ := t04Tools(t)
							if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", rebuilt, "-d", rebuilt, path).CombinedOutput(); err != nil {
								t.Fatalf("candidate compile: %v %s\n%s", err, out, output)
							}
							if got := t04RunJava(t, java, rebuilt, "NaNBindingDriver"); got != want {
								t.Fatalf("candidate raw bits=%q", got)
							}
						})
					}
				})
			}
		})
	}
}

package javaclassparser

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The original class and rebuilt class run against the same unchanged numeric
// driver. The independent table evaluates IEEE relations in Go, including both
// unordered operands and signed zeros; each observation also includes call order.
func TestAdversarialFloatingPredicatesPreserveUnorderedTruthAndEffects(t *testing.T) {
	javac, java := requireJDK(t)
	fs := []uint32{0, 0x80000000, 1, 0x80000001, 0x007fffff, 0x00800000, 0x3f800000, 0xbf800000, 0x7f7fffff, 0xff7fffff, 0x7f800000, 0xff800000, 0x7fc00000, 0x7fc01234, 0xffc01234, 0x3f800001}
	ds := []uint64{0, 0x8000000000000000, 1, 0x8000000000000001, 0x000fffffffffffff, 0x0010000000000000, 0x3ff0000000000000, 0xbff0000000000000, 0x7fefffffffffffff, 0xffefffffffffffff, 0x7ff0000000000000, 0xfff0000000000000, 0x7ff8000000000000, 0x7ff8000000001234, 0xfff8000000001234, 0x3ff0000000000001}
	relations := []string{"<", "<=", ">", ">=", "==", "!="}
	relation := func(a, b float64, op string) bool {
		switch op {
		case "<":
			return a < b
		case "<=":
			return a <= b
		case ">":
			return a > b
		case ">=":
			return a >= b
		case "==":
			return a == b
		case "!=":
			return a != b
		}
		panic("unknown relation")
	}
	var probe, driver, want strings.Builder
	probe.WriteString("public class FloatingPredicateProbe{static int trace,fail;static final RuntimeException ERROR=new RuntimeException(\"predicate\");static float left(float v){trace=trace*10+1;if(fail==1)throw ERROR;return v;}static float right(float v){trace=trace*10+2;if(fail==2)throw ERROR;return v;}static double left(double v){trace=trace*10+1;if(fail==1)throw ERROR;return v;}static double right(double v){trace=trace*10+2;if(fail==2)throw ERROR;return v;}\n")
	driver.WriteString("public class FloatingPredicateDriver{public static void main(String[]args){\n")
	driver.WriteString("float[] fs=new float[]{")
	for i, bits := range fs {
		if i > 0 {
			driver.WriteByte(',')
		}
		fmt.Fprintf(&driver, "Float.intBitsToFloat(0x%08x)", bits)
	}
	driver.WriteString("};double[] ds=new double[]{")
	for i, bits := range ds {
		if i > 0 {
			driver.WriteByte(',')
		}
		fmt.Fprintf(&driver, "Double.longBitsToDouble(0x%016xL)", bits)
	}
	driver.WriteString("};\n")
	count := 0
	for _, typ := range []string{"float", "double"} {
		for _, op := range relations {
			for neg := 0; neg < 2; neg++ {
				for effect := 0; effect < 2; effect++ {
					for shape := 0; shape < 3; shape++ {
						name := fmt.Sprintf("m%d", count)
						count++
						left, right := "a", "b"
						if effect == 1 {
							left, right = "left(a)", "right(b)"
						}
						cond := left + op + right
						if neg == 1 {
							cond = "!(" + cond + ")"
						}
						body := ""
						switch shape {
						case 0:
							body = "return " + cond + ";"
						case 1:
							body = "if(" + cond + "){return 17;}else{return -19;}"
						case 2:
							body = "return " + cond + "?23:-29;"
						}
						ret := "int"
						if shape == 0 {
							ret = "boolean"
						}
						fmt.Fprintf(&probe, "public static %s %s(%s a,%s b){%s}\n", ret, name, typ, typ, body)
						table := "fs"
						if typ == "double" {
							table = "ds"
						}
						fmt.Fprintf(&driver, "for(int i=0;i<16;i++){for(int j=0;j<16;j++){for(int fail=0;fail<3;fail++){FloatingPredicateProbe.trace=0;FloatingPredicateProbe.fail=fail;String result;try{result=String.valueOf(FloatingPredicateProbe.%s(%s[i],%s[j]));}catch(RuntimeException e){result=e==FloatingPredicateProbe.ERROR?\"E\":\"wrong-error\";}System.out.println(\"%s:\"+i+\",\"+j+\",\"+fail+\":\"+result+\":\"+FloatingPredicateProbe.trace);}}}\n", name, table, table, name)
						for i := 0; i < len(fs); i++ {
							for j := 0; j < len(fs); j++ {
								a, b := float64(math.Float32frombits(fs[i])), float64(math.Float32frombits(fs[j]))
								if typ == "double" {
									a, b = math.Float64frombits(ds[i]), math.Float64frombits(ds[j])
								}
								truth := relation(a, b, op)
								if neg == 1 {
									truth = !truth
								}
								value := fmt.Sprint(truth)
								if shape == 1 {
									value = "-19"
									if truth {
										value = "17"
									}
								} else if shape == 2 {
									value = "-29"
									if truth {
										value = "23"
									}
								}
								for fail := 0; fail < 3; fail++ {
									trace := 0
									out := value
									if effect == 1 {
										trace = 12
										if fail > 0 {
											out = "E"
											if fail == 1 {
												trace = 1
											}
										}
									}
									fmt.Fprintf(&want, "%s:%d,%d,%d:%s:%d\n", name, i, j, fail, out, trace)
								}
							}
						}
					}
				}
			}
		}
	}
	probe.WriteString("}\n")
	driver.WriteString("}}\n")
	t.Logf("methods=%d observations=%d", count, count*len(fs)*len(fs)*3)
	for _, debug := range []string{"-g", "-g:none"} {
		t.Run(debug, func(t *testing.T) {
			original := t.TempDir()
			p := filepath.Join(original, "FloatingPredicateProbe.java")
			d := filepath.Join(original, "FloatingPredicateDriver.java")
			if e := os.WriteFile(p, []byte(probe.String()), 0600); e != nil {
				t.Fatal(e)
			}
			if e := os.WriteFile(d, []byte(driver.String()), 0600); e != nil {
				t.Fatal(e)
			}
			if out, e := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", original, p, d).CombinedOutput(); e != nil {
				t.Fatalf("original compile: %v %s", e, out)
			}
			expected := want.String()
			check := func(t *testing.T, label, got string) {
				t.Helper()
				if got == expected {
					return
				}
				gs, ws := strings.Split(got, "\n"), strings.Split(expected, "\n")
				bad := 0
				first := ""
				for i, w := range ws {
					g := "<missing>"
					if i < len(gs) {
						g = gs[i]
					}
					if w != g {
						bad++
						if first == "" {
							first = fmt.Sprintf("row%d got %q want %q", i, g, w)
						}
					}
				}
				t.Fatalf("%s %d mismatches; %s", label, bad, first)
			}
			check(t, "original", t04RunJava(t, java, original, "FloatingPredicateDriver"))
			raw, e := os.ReadFile(filepath.Join(original, "FloatingPredicateProbe.class"))
			if e != nil {
				t.Fatal(e)
			}
			dr, e := os.ReadFile(filepath.Join(original, "FloatingPredicateDriver.class"))
			if e != nil {
				t.Fatal(e)
			}
			for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
				t.Run(string(mode), func(t *testing.T) {
					var result DecompileResult
					var e error
					if mode == "legacy" {
						result.Source, e = Decompile(raw)
					} else {
						result, e = DecompileWithOptions(raw, DecompileOptions{Mode: mode})
					}
					if e != nil {
						t.Fatal(e)
					}
					rebuilt := t.TempDir()
					source := filepath.Join(rebuilt, "FloatingPredicateProbe.java")
					if e = os.WriteFile(source, []byte(result.Source), 0600); e != nil {
						t.Fatal(e)
					}
					if out, e := exec.Command(javac, "-proc:none", "--release", "8", "-cp", rebuilt, "-d", rebuilt, source).CombinedOutput(); e != nil {
						t.Fatalf("candidate compile: %v %s", e, out)
					}
					if e = os.WriteFile(filepath.Join(rebuilt, "FloatingPredicateDriver.class"), dr, 0600); e != nil {
						t.Fatal(e)
					}
					check(t, "candidate", t04RunJava(t, java, rebuilt, "FloatingPredicateDriver"))
				})
			}
		})
	}
}

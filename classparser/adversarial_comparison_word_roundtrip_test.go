package javaclassparser

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// javac has no source operator returning an fcmp/dcmp/lcmp word. Start from a
// verified authored sum/cast boundary and replace only that two-byte producer
// with cmp/nop. Descriptors, frames, handlers, local uses and the unchanged
// driver stay intact. The original JVM must pass the independent numeric and
// effect oracle before the generated class is run in a classpath of its own.
func TestAdversarialComparisonWordsPreserveNumericCategoryAndEffects(t *testing.T) {
	javac, java := requireJDK(t)
	fs := []uint32{0, 0x80000000, 1, 0x80000001, 0x007fffff, 0x00800000, 0x3f800000, 0xbf800000, 0x7f7fffff, 0xff7fffff, 0x7f800000, 0xff800000, 0x7fc00000, 0x7fc01234, 0xffc01234, 0x3f800001}
	ds := []uint64{0, 0x8000000000000000, 1, 0x8000000000000001, 0x000fffffffffffff, 0x0010000000000000, 0x3ff0000000000000, 0xbff0000000000000, 0x7fefffffffffffff, 0xffefffffffffffff, 0x7ff0000000000000, 0xfff0000000000000, 0x7ff8000000000000, 0x7ff8000000001234, 0xfff8000000001234, 0x3ff0000000000001}
	ls := []int64{0, 1, -1, 2, -2, math.MaxInt64, math.MinInt64, math.MaxInt64 - 1, math.MinInt64 + 1, 1 << 32, -1 << 32, 1 << 53, 1<<53 + 1, -1 << 53, -1<<53 - 1, 1 << 62}
	type spec struct {
		name   string
		opcode byte
		pair   []byte
	}
	var specs []spec
	var probe, driver, want strings.Builder
	probe.WriteString("public class ComparisonWordProbe{static int trace,fail;static final RuntimeException ERROR=new RuntimeException(\"comparison\");static volatile float vf;static volatile double vd;static volatile long vl;\n")
	for _, typ := range []string{"float", "double", "long"} {
		field := map[string]string{"float": "vf", "double": "vd", "long": "vl"}[typ]
		fmt.Fprintf(&probe, "static %s left(%s v){trace=trace*10+1;if(fail==1)throw ERROR;return v;}static %s right(%s v){trace=trace*10+2;%s=99;if(fail==2)throw ERROR;return v;}\n", typ, typ, typ, typ, field)
	}
	driver.WriteString("public class ComparisonWordDriver{public static void main(String[]args){float[]fs=new float[]{")
	for i, b := range fs {
		if i > 0 {
			driver.WriteByte(',')
		}
		fmt.Fprintf(&driver, "Float.intBitsToFloat(0x%08x)", b)
	}
	driver.WriteString("};double[]ds=new double[]{")
	for i, b := range ds {
		if i > 0 {
			driver.WriteByte(',')
		}
		fmt.Fprintf(&driver, "Double.longBitsToDouble(0x%016xL)", b)
	}
	driver.WriteString("};long[]ls=new long[]{")
	for i, b := range ls {
		if i > 0 {
			driver.WriteByte(',')
		}
		fmt.Fprintf(&driver, "0x%016xL", uint64(b))
	}
	driver.WriteString("};\n")
	for _, kind := range []string{"fl", "fg", "dl", "dg", "l"} {
		typ, table, field, opcode, pair := "float", "fs", "vf", byte(0x95), []byte{0x62, 0x8b}
		if kind[0] == 'd' {
			typ, table, field, opcode, pair = "double", "ds", "vd", 0x97, []byte{0x63, 0x8e}
		}
		if kind == "l" {
			typ, table, field, opcode, pair = "long", "ls", "vl", 0x94, []byte{0x61, 0x88}
		}
		if kind == "fg" || kind == "dg" {
			opcode++
		}
		for _, layout := range []string{"direct", "stored", "reused", "dup", "nonzero", "branch", "zero", "caught", "finally", "volatile", "handler-boundary"} {
			name := fmt.Sprintf("m%d", len(specs))
			specs = append(specs, spec{name, opcode, pair})
			word := "(int)(left(a)+right(b))"
			body := ""
			switch layout {
			case "direct":
				body = "return " + word + ";"
			case "stored":
				body = "int x=" + word + ";return x*7+3;"
			case "reused":
				body = "int x=" + word + ";return x*100+x*10+x;"
			case "dup":
				body = "int x;return (x=" + word + ")*10+x;"
			case "nonzero":
				body = "int x=" + word + ";if(x==1)return 71;if(x==-1)return 72;return 73;"
			case "branch":
				body = "int x=" + word + ";return x<0?41:(x==0?42:43);"
			case "zero":
				body = "if(" + word + "<0)return 91;return 97;"
			case "handler-boundary":
				body = typ + " x=left(a);try{return(int)(x+right(b));}catch(RuntimeException e){trace=trace*10+3;return e==ERROR?107:109;}"
			case "caught":
				body = "try{return " + word + ";}catch(RuntimeException e){trace=trace*10+3;return e==ERROR?107:109;}"
			case "finally":
				body = "try{return " + word + ";}finally{trace=trace*10+4;}"
			case "volatile":
				body = "return(int)(" + field + "+right(b));"
			}
			fmt.Fprintf(&probe, "public static int %s(%s a,%s b){%s}\n", name, typ, typ, body)
			stateWord := "Integer.toHexString(Float.floatToRawIntBits(ComparisonWordProbe.vf))"
			if typ == "double" {
				stateWord = "Long.toHexString(Double.doubleToRawLongBits(ComparisonWordProbe.vd))"
			} else if typ == "long" {
				stateWord = "Long.toHexString(ComparisonWordProbe.vl)"
			}
			fmt.Fprintf(&driver, "for(int i=0;i<16;i++){for(int j=0;j<16;j++){for(int fail=0;fail<3;fail++){ComparisonWordProbe.%s=%s[i];ComparisonWordProbe.trace=0;ComparisonWordProbe.fail=fail;String result;try{result=String.valueOf(ComparisonWordProbe.%s(%s[i],%s[j]));}catch(RuntimeException e){result=e==ComparisonWordProbe.ERROR?\"E\":\"wrong-error\";}System.out.println(\"%s:\"+i+\",\"+j+\",\"+fail+\":\"+result+\":\"+ComparisonWordProbe.trace+\":\"+%s);}}}\n", field, table, name, table, table, name, stateWord)
			for i := 0; i < 16; i++ {
				for j := 0; j < 16; j++ {
					result := 0
					if typ == "long" {
						if ls[i] < ls[j] {
							result = -1
						} else if ls[i] > ls[j] {
							result = 1
						}
					} else {
						a, b := float64(math.Float32frombits(fs[i])), float64(math.Float32frombits(fs[j]))
						if typ == "double" {
							a, b = math.Float64frombits(ds[i]), math.Float64frombits(ds[j])
						}
						if math.IsNaN(a) || math.IsNaN(b) {
							result = -1
							if kind == "fg" || kind == "dg" {
								result = 1
							}
						} else if a < b {
							result = -1
						} else if a > b {
							result = 1
						}
					}
					output := result
					switch layout {
					case "stored":
						output = result*7 + 3
					case "reused":
						output = result * 111
					case "dup":
						output = result * 11
					case "nonzero":
						output = 73
						if result == 1 {
							output = 71
						} else if result == -1 {
							output = 72
						}
					case "zero":
						output = 97
						if result < 0 {
							output = 91
						}
					case "branch":
						output = 42
						if result < 0 {
							output = 41
						} else if result > 0 {
							output = 43
						}
					}
					for fail := 0; fail < 3; fail++ {
						trace := 12
						out := fmt.Sprint(output)
						abrupt := fail > 0
						if fail == 1 {
							trace = 1
						}
						if layout == "volatile" {
							trace = 2
							abrupt = fail == 2
						}
						if abrupt {
							out = "E"
							if layout == "caught" || layout == "handler-boundary" && fail == 2 {
								out = "107"
								trace = trace*10 + 3
							}
						}
						if layout == "finally" {
							trace = trace*10 + 4
						}
						heap := fmt.Sprintf("%x", math.Float32bits(99))
						if typ == "double" {
							heap = fmt.Sprintf("%x", math.Float64bits(99))
						} else if typ == "long" {
							heap = "63"
						}
						if fail == 1 && layout != "volatile" {
							heap = fmt.Sprintf("%x", fs[i])
							if typ == "double" {
								heap = fmt.Sprintf("%x", ds[i])
							} else if typ == "long" {
								heap = fmt.Sprintf("%x", uint64(ls[i]))
							}
						}
						fmt.Fprintf(&want, "%s:%d,%d,%d:%s:%d:%s\n", name, i, j, fail, out, trace, heap)
					}
				}
			}
		}
	}
	probe.WriteString("}\n")
	driver.WriteString("}}\n")
	t.Logf("methods=%d observations=%d", len(specs), len(specs)*16*16*3)
	check := func(t *testing.T, label, got string) {
		t.Helper()
		if got == want.String() {
			return
		}
		gs, ws := strings.Split(got, "\n"), strings.Split(want.String(), "\n")
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
	for _, debug := range []string{"-g", "-g:none"} {
		t.Run(debug, func(t *testing.T) {
			dir := t.TempDir()
			p, d := filepath.Join(dir, "ComparisonWordProbe.java"), filepath.Join(dir, "ComparisonWordDriver.java")
			for file, s := range map[string]string{p: probe.String(), d: driver.String()} {
				if e := os.WriteFile(file, []byte(s), 0600); e != nil {
					t.Fatal(e)
				}
			}
			if out, e := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", dir, p, d).CombinedOutput(); e != nil {
				t.Fatalf("original compile %v %s", e, out)
			}
			raw, e := os.ReadFile(filepath.Join(dir, "ComparisonWordProbe.class"))
			if e != nil {
				t.Fatal(e)
			}
			obj, e := Parse(raw)
			if e != nil {
				t.Fatal(e)
			}
			patched := 0
			for _, s := range specs {
				found := false
				for _, m := range obj.Methods {
					name, _ := obj.getUtf8(m.NameIndex)
					if name != s.name {
						continue
					}
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							if bytes.Count(c.Code, s.pair) != 1 {
								t.Fatalf("unique sum/cast boundary %s: %x", name, c.Code)
							}
							c.Code = bytes.Replace(c.Code, s.pair, []byte{s.opcode, 0}, 1)
							patched++
							found = true
						}
					}
				}
				if !found {
					t.Fatalf("missing comparison producer %s", s.name)
				}
			}
			if patched != len(specs) {
				t.Fatal("incomplete opcode bank")
			}
			raw = obj.Bytes()
			if e = os.WriteFile(filepath.Join(dir, "ComparisonWordProbe.class"), raw, 0600); e != nil {
				t.Fatal(e)
			}
			check(t, "original", t04RunJava(t, java, dir, "ComparisonWordDriver"))
			dr, e := os.ReadFile(filepath.Join(dir, "ComparisonWordDriver.class"))
			if e != nil {
				t.Fatal(e)
			}
			for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
				t.Run(string(mode), func(t *testing.T) {
					var r DecompileResult
					var e error
					if mode == "legacy" {
						r.Source, e = Decompile(raw)
					} else {
						r, e = DecompileWithOptions(raw, DecompileOptions{Mode: mode})
					}
					if e != nil {
						t.Fatal(e)
					}
					outdir := t.TempDir()
					file := filepath.Join(outdir, "ComparisonWordProbe.java")
					if e = os.WriteFile(file, []byte(r.Source), 0600); e != nil {
						t.Fatal(e)
					}
					if out, e := exec.Command(javac, "-proc:none", "--release", "8", "-cp", outdir, "-d", outdir, file).CombinedOutput(); e != nil {
						t.Fatalf("candidate compile %v %s\n%s", e, out, r.Source)
					}
					if e = os.WriteFile(filepath.Join(outdir, "ComparisonWordDriver.class"), dr, 0600); e != nil {
						t.Fatal(e)
					}
					check(t, "candidate", t04RunJava(t, java, outdir, "ComparisonWordDriver"))
				})
			}
		})
	}
}

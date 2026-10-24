package javaclassparser

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdversarialCarriedFloatingJoinKeepsIEEEValuesAndOrder(t *testing.T) {
	javac, java := t04Tools(t)
	const source = `class FloatingEffects {static String trace="";static float word(){trace+="W";return -0.0f;}static double wide(){trace+="D";return -0.0d;}}
class FloatingOwner {static float fold(float seed,boolean select){return seed;}static double combine(double seed,boolean select){return seed;}}
public class FloatingDriver {public static void main(String[]args){int rows=0;for(double seed:new double[]{0.0,-0.0,1.0,-1.0,Float.MIN_VALUE,Double.MIN_VALUE,Float.MAX_VALUE,Double.MAX_VALUE,Double.POSITIVE_INFINITY,Double.NEGATIVE_INFINITY,Double.NaN})for(boolean select:new boolean[]{false,true}){
 FloatingEffects.trace="";float expected=(float)seed*31;if(select)expected+=-0.0f;float got=FloatingOwner.fold((float)seed,select);if(Float.floatToIntBits(got)!=Float.floatToIntBits(expected)||!FloatingEffects.trace.equals(select?"W":""))throw new AssertionError("float "+seed+":"+select);
 FloatingEffects.trace="";double wanted=seed*31;if(select)wanted+=-0.0d;double actual=FloatingOwner.combine(seed,select);if(Double.doubleToLongBits(actual)!=Double.doubleToLongBits(wanted)||!FloatingEffects.trace.equals(select?"D":""))throw new AssertionError("double "+seed+":"+select);rows+=2;}System.out.println(rows+":carried:IEEE:NaN:infinity:signed-zero:order");}}`
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "FloatingDriver.java")
			if err := os.WriteFile(path, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:"+debug, "-d", dir, path).CombinedOutput(); err != nil {
				t.Fatal(err, string(out))
			}
			obj := mustParseLowering(t, readClassBytes(t, dir, "FloatingOwner"))
			pool := obj.ConstantPoolManager
			pool.AddUtf8Info("StackMapTable")
			changed := 0
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n != "fold" && n != "combine" {
					continue
				}
				wide := n == "combine"
				load, convert, multiply, add, ret, tag, local, callee, desc := byte(0x22), byte(0x86), byte(0x6a), byte(0x62), byte(0xae), byte(2), byte(1), "word", "()F"
				if wide {
					load, convert, multiply, add, ret, tag, local, callee, desc = 0x26, 0x87, 0x6b, 0x63, 0xaf, 3, 2, "wide", "()D"
				}
				call := pool.AddNewMethodInfo("FloatingEffects", callee, desc)
				body := []byte{load, 0x10, 31, convert, multiply, 0x15, local}
				branch := len(body)
				body = append(body, 0x99, 0, 0, 0xb8, byte(call>>8), byte(call), add)
				join := len(body)
				binary.BigEndian.PutUint16(body[branch+1:], uint16(join-branch))
				body = append(body, ret)
				frames := []byte{0, 1, byte(64 + join), tag}
				c := constructorLoweringCode(m)
				c.Code = body
				c.MaxStack = 4
				c.MaxLocals = uint16(local) + 1
				c.Attributes = []AttributeInfo{&UnparsedAttribute{Name: "StackMapTable", Length: uint32(len(frames)), Info: frames}}
				c.AttrLen = uint32(12 + len(body) + 6 + len(frames))
				changed++
			}
			if changed != 2 {
				t.Fatal("original numeric methods")
			}
			raw := obj.Bytes()
			if err := os.WriteFile(filepath.Join(dir, "FloatingOwner.class"), raw, 0600); err != nil {
				t.Fatal(err)
			}
			run := func(cp string) {
				t.Helper()
				out, err := exec.Command(java, "-Xverify:all", "-cp", cp, "FloatingDriver").CombinedOutput()
				if err != nil || string(out) != "44:carried:IEEE:NaN:infinity:signed-zero:order\n" {
					t.Fatal("original-first IEEE oracle", err, string(out))
				}
			}
			run(dir)
			resolve := resolverFromClasses(classMapFromDir(t, dir))
			for _, mode := range []DecompileMode{Precision, Compatibility} {
				t.Run(string(mode), func(t *testing.T) {
					result, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
					if err != nil || len(result.StubMethods) != 0 || strings.Contains(result.Source, DecompileStubMarker) {
						t.Fatal(err, result.Diagnostics)
					}
					out := t.TempDir()
					src := filepath.Join(out, "FloatingOwner.java")
					if err := os.WriteFile(src, []byte(result.Source), 0600); err != nil {
						t.Fatal(err)
					}
					if log, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:"+debug, "-cp", dir, "-d", out, src).CombinedOutput(); err != nil {
						t.Fatal(err, string(log), result.Source)
					}
					run(out + string(os.PathListSeparator) + dir)
				})
			}
		})
	}
}

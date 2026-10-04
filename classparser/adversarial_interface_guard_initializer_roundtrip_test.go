package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A JVM conditional can have identical taken/fallthrough successors, but its
// GETSTATIC producer still initializes the declaring class. It must run once,
// before all field initializers, and its original failure must stop those writes.
func TestAdversarialInterfaceSameSuccessorGuardInitializerRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	const source = `class GuardInitState{static String trace="";static final Error failure=new AssertionError("identity");static boolean gate(boolean fail){trace+="G";if(fail)throw failure;return false;}static long mark(long n){trace+="M";return n+1;}static boolean both(boolean first,boolean second){trace+="B";return first&&second;}static boolean first(){trace+="P";return true;}static boolean second(){trace+="Q";return false;}static final RuntimeException runtime=new IllegalStateException("identity");static boolean failedSecond(){trace+="Q";throw runtime;}}
class GuardInitSuccess{static boolean $assertionsDisabled=GuardInitState.gate(false);}
class GuardInitFailure{static boolean $assertionsDisabled=GuardInitState.gate(true);}
interface GuardInitTable{long A=GuardInitState.mark(4);Object B=new Object();static boolean witness(){return GuardInitSuccess.$assertionsDisabled;}}
interface GuardInitFailedTable{long A=GuardInitState.mark(8);Object B=new Object();static boolean witness(){return GuardInitFailure.$assertionsDisabled;}}
interface GuardInitMultiTable{long A=GuardInitState.mark(40);Object B=new Object();static boolean witness(){return GuardInitState.both(GuardInitState.first(),GuardInitState.second());}}
interface GuardInitMultiFailedTable{long A=GuardInitState.mark(80);Object B=new Object();static boolean witness(){return GuardInitState.both(GuardInitState.first(),GuardInitState.failedSecond());}}
public class GuardInitializerDriver{public static void main(String[]args){System.out.println(GuardInitTable.A+":"+(GuardInitTable.B==GuardInitTable.B)+":"+GuardInitState.trace);try{System.out.println(GuardInitFailedTable.A);}catch(Throwable failure){System.out.println((failure==GuardInitState.failure)+":"+GuardInitState.trace);}try{System.out.println(GuardInitFailedTable.B);}catch(NoClassDefFoundError failure){System.out.println("failed:"+GuardInitState.trace);}System.out.println(GuardInitMultiTable.A+":"+(GuardInitMultiTable.B==GuardInitMultiTable.B)+":"+GuardInitState.trace);try{System.out.println(GuardInitMultiFailedTable.A);}catch(ExceptionInInitializerError failure){System.out.println((failure.getCause()==GuardInitState.runtime)+":"+GuardInitState.trace);}try{System.out.println(GuardInitMultiFailedTable.B);}catch(NoClassDefFoundError failure){System.out.println("failed:"+GuardInitState.trace);}}}`
	for _, debug := range []string{"-g", "-g:none"} {
		t.Run(debug, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "GuardInitializerDriver.java")
			if err := os.WriteFile(src, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", dir, src).CombinedOutput(); err != nil {
				t.Fatalf("original: %v\n%s", err, out)
			}
			for _, unit := range []string{"GuardInitTable", "GuardInitFailedTable", "GuardInitMultiTable", "GuardInitMultiFailedTable"} {
				file := filepath.Join(dir, unit+".class")
				raw, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				object, err := Parse(raw)
				if err != nil {
					t.Fatal(err)
				}
				var fieldRef []byte
				var prefixStack uint16
				var initializer *CodeAttribute
				for _, method := range object.Methods {
					name, _ := object.getUtf8(method.NameIndex)
					for _, attr := range method.Attributes {
						code, ok := attr.(*CodeAttribute)
						if !ok {
							continue
						}
						if name == "witness" {
							if len(code.Code) < 4 || code.Code[len(code.Code)-1] != 172 {
								t.Fatalf("unexpected witness code %x", code.Code)
							}
							fieldRef = append([]byte{}, code.Code[:len(code.Code)-1]...)
							prefixStack = code.MaxStack
						}
						if name == "<clinit>" {
							initializer = code
						}
					}
				}
				if initializer == nil || len(fieldRef) == 0 || len(initializer.ExceptionTable) != 0 {
					t.Fatal("fixture initializer proof missing")
				}
				for _, attr := range initializer.Attributes {
					if raw, ok := attr.(*UnparsedAttribute); ok && raw.Name == "StackMapTable" {
						t.Fatal("fixture unexpectedly needs existing branch frames")
					}
				}
				// The original producer prefix remains intact; IFEQ +3 has the
				// same taken/fallthrough successor with an empty stack.
				prefix := append(fieldRef, 153, 0, 3)
				initializer.Code = append(prefix, initializer.Code...)
				// Original straight-line debug tables no longer describe the prefix. The
				// sole verifier frame is exact and independently checked by -Xverify:all.
				initializer.Attributes = []AttributeInfo{&UnparsedAttribute{Name: "StackMapTable", Length: 3, Info: []byte{0, 1, byte(len(prefix))}}}
				initializer.AttrLen = uint32(12 + len(initializer.Code) + 9)
				NewConstantPoolWithConstant(&object.ConstantPool).AddUtf8Info("StackMapTable")
				if initializer.MaxStack < prefixStack {
					initializer.MaxStack = prefixStack
				}
				if err := os.WriteFile(file, object.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
			}
			want := t04RunJava(t, java, dir, "GuardInitializerDriver")
			if strings.TrimSpace(want) != "5:true:GM\ntrue:GMG\nfailed:GMG\n41:true:GMGPQBM\ntrue:GMGPQBMPQ\nfailed:GMGPQBMPQ" {
				t.Fatalf("independent guard/init oracle %q", want)
			}
			resolve := func(name string) ([]byte, bool) {
				b, e := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)+".class"))
				return b, e == nil
			}
			for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
				t.Run(string(mode), func(t *testing.T) {
					rebuilt := t.TempDir()
					args := []string{"-proc:none", "--release", "8", "-cp", dir, "-d", rebuilt}
					var all strings.Builder
					for _, unit := range []string{"GuardInitTable", "GuardInitFailedTable", "GuardInitMultiTable", "GuardInitMultiFailedTable"} {
						raw, ok := resolve(unit)
						if !ok {
							t.Fatal("missing original interface")
						}
						var result DecompileResult
						var err error
						if mode == "legacy" {
							result.Source, err = DecompileWithResolver(raw, resolve)
						} else {
							result, err = DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
						}
						if err != nil {
							t.Fatal(err)
						}
						t.Logf("%s %s diagnostics: %+v\n%s", unit, mode, result.Diagnostics, result.Source)
						all.WriteString(result.Source)
						all.WriteString("\n")
						path := filepath.Join(rebuilt, unit+".java")
						if err := os.WriteFile(path, []byte(result.Source), 0600); err != nil {
							t.Fatal(err)
						}
						args = append(args, path)
					}
					if out, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
						t.Fatalf("rebuild: %v\n%s\n%s", err, out, all.String())
					}
					if got := t04RunJava(t, java, rebuilt+string(os.PathListSeparator)+dir, "GuardInitializerDriver"); got != want {
						t.Fatalf("guard/init got %q want %q\n%s", got, want, all.String())
					}
				})
			}
		})
	}
}

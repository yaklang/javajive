package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const nativeInterfaceInitializationFixture = `class InterfaceEffects{static boolean fail;static String trace="";static final RuntimeException failure=new RuntimeException("same");static Object mark(String n){trace+=n;if(fail&&n.equals("B"))throw failure;return new Object();}static long number(){trace+="N";return Long.MIN_VALUE;}static String text(){trace+="T";return "kept";}}
class InterfaceOwner{interface Contract{Object early=Contract.later;Object first=InterfaceEffects.mark("A");Object second=InterfaceEffects.mark("B");Object alias=Contract.first;long number=InterfaceEffects.number();String text=InterfaceEffects.text();Object later=InterfaceEffects.mark("L");default long mix(long n){return n^number;}static Object identity(){return alias;}}static class Implementation implements Contract{}class Child{Object owner(){return InterfaceOwner.this;}}}
class InterfaceDriver{public static void main(String[]args)throws Exception{InterfaceEffects.fail=args.length>0;Class<?>contract=Class.forName("InterfaceOwner$Contract",false,InterfaceDriver.class.getClassLoader());if(contract.getDeclaredMethods().length!=2||!InterfaceEffects.trace.equals(""))throw new AssertionError("ABI or eager initialization");for(int i=0;i<2;i++){try{Object first=InterfaceOwner.Contract.first;if(InterfaceEffects.fail||InterfaceOwner.Contract.early!=null||InterfaceOwner.Contract.later==null||first==null||InterfaceOwner.Contract.second==first||InterfaceOwner.Contract.alias!=first||InterfaceOwner.Contract.identity()!=first||InterfaceOwner.Contract.number!=Long.MIN_VALUE||!InterfaceOwner.Contract.text.equals("kept")||new InterfaceOwner.Implementation().mix(Long.MAX_VALUE)!=-1||!InterfaceEffects.trace.equals("ABNTL"))throw new AssertionError("order identity or value");}catch(ExceptionInInitializerError e){if(!InterfaceEffects.fail||i!=0||e.getCause()!=InterfaceEffects.failure||!InterfaceEffects.trace.equals("AB"))throw new AssertionError("first failure",e);}catch(NoClassDefFoundError e){if(!InterfaceEffects.fail||i!=1||!InterfaceEffects.trace.equals("AB"))throw new AssertionError("repeated failure",e);}}System.out.println(InterfaceEffects.trace+":"+contract.getDeclaredMethods().length);}}`

func TestNativeMemberInterfaceInitializerPreservesMethodABIAndFailures(t *testing.T) {
	javac, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, nativeInterfaceInitializationFixture, debug)
			original := t.TempDir()
			for n, raw := range files {
				if e := os.WriteFile(filepath.Join(original, n), raw, 0600); e != nil {
					t.Fatal(e)
				}
			}
			run := func(dir string, fail bool) string {
				t.Helper()
				args := []string{"-Xverify:all", "-cp", dir, "InterfaceDriver"}
				if fail {
					args = append(args, "fail")
				}
				out, e := exec.Command(java, args...).CombinedOutput()
				if e != nil {
					t.Fatalf("independent JVM: %v %s", e, out)
				}
				return string(out)
			}
			oracles := []string{run(original, false), run(original, true)}
			if oracles[0] != "ABNTL:2\n" || oracles[1] != "AB:2\n" {
				t.Fatalf("original initialization %q", oracles)
			}
			for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(policy, func(t *testing.T) {
					if policy == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if policy == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					z := nativeArchive(t, files)
					defer z.Close()
					src, e := z.ReadFile("InterfaceOwner.class")
					if e != nil || strings.Contains(string(src), DecompileStubMarker) {
						t.Fatalf("source %v %s", e, src)
					}
					output := t.TempDir()
					for n, raw := range files {
						if strings.HasPrefix(n, "InterfaceOwner") {
							continue
						}
						if e := os.WriteFile(filepath.Join(output, n), raw, 0600); e != nil {
							t.Fatal(e)
						}
					}
					file := filepath.Join(output, "InterfaceOwner.java")
					if e := os.WriteFile(file, src, 0600); e != nil {
						t.Fatal(e)
					}
					if out, e := exec.Command(javac, "-proc:none", "--release", "8", "-cp", output, "-d", output, file).CombinedOutput(); e != nil {
						t.Fatalf("rebuilt %v %s\n%s", e, out, src)
					}
					for i, fail := range []bool{false, true} {
						if got := run(output, fail); got != oracles[i] {
							t.Fatalf("JVM %q != %q", got, oracles[i])
						}
					}
					for n, want := range files {
						if !strings.HasPrefix(n, "InterfaceOwner") {
							continue
						}
						got, e := os.ReadFile(filepath.Join(output, n))
						if e != nil {
							t.Fatal(e)
						}
						if nativeBinaryShape(t, got) != nativeBinaryShape(t, want) {
							t.Fatalf("ABI %s\n%s\n%s", n, nativeBinaryShape(t, want), nativeBinaryShape(t, got))
						}
					}
				})
			}
		})
	}
}

// Legal JVM initializers can have statement prefixes or nonconstant primitive
// stores that no direct field expression can regenerate with the same public
// method ABI. Their effects must not be dropped to commit the whole family.
func TestNativeMemberInterfaceRefusesAddedInitializationABI(t *testing.T) {
	_, java := t04Tools(t)
	for _, variant := range []string{"statement prefix", "primitive constant status"} {
		t.Run(variant, func(t *testing.T) {
			source := `class RejectEffects{static String trace="";static void touch(){trace+="S";}static int number(){trace+="N";return 7;}}
class RejectOwner{interface Contract{int value=RejectEffects.number();static void anchor(){RejectEffects.touch();}}class Child{}}
class RejectDriver{public static void main(String[]args){if(RejectOwner.Contract.value!=7)throw new AssertionError("value");System.out.println(RejectEffects.trace+":"+RejectOwner.Contract.class.getDeclaredMethods().length);}}`
			files := nativeCompileClasses(t, source)
			object, e := Parse(files["RejectOwner$Contract.class"])
			if e != nil {
				t.Fatal(e)
			}
			touch := 0
			for i, constant := range object.ConstantPool {
				if m, ok := constant.(*ConstantMethodrefInfo); ok {
					owner, _ := sourceBridgeClassName(object, m.ClassIndex)
					nt, ok := object.ConstantPool[m.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
					if !ok {
						continue
					}
					name, _ := sourceBridgeUTF8(object, nt.NameIndex)
					if owner == "RejectEffects" && name == "touch" {
						touch = i + 1
					}
				}
			}
			if touch == 0 {
				t.Fatal("original method reference")
			}
			found := false
			for _, method := range object.Methods {
				name, _ := sourceBridgeUTF8(object, method.NameIndex)
				if name != "<clinit>" {
					continue
				}
				for _, attribute := range method.Attributes {
					if code, ok := attribute.(*CodeAttribute); ok {
						if len(code.Code) != 7 {
							t.Fatalf("authored initializer shape %v", code.Code)
						}
						if variant == "statement prefix" {
							code.Code = append([]byte{0xb8, byte(touch >> 8), byte(touch)}, code.Code...)
							code.AttrLen += 3
						} else {
							copy(code.Code[:3], []byte{0x10, 7, 0})
						}
						found = true
					}
				}
			}
			if !found {
				t.Fatal("original initializer")
			}
			files["RejectOwner$Contract.class"] = object.Bytes()
			original := t.TempDir()
			for n, raw := range files {
				if e := os.WriteFile(filepath.Join(original, n), raw, 0600); e != nil {
					t.Fatal(e)
				}
			}
			want := "SN:1\n"
			if variant == "primitive constant status" {
				want = ":1\n"
			}
			if got := t04RunJava(t, java, original, "RejectDriver"); got != want {
				t.Fatalf("original JVM %q != %q", got, want)
			}
			z := nativeArchive(t, files)
			defer z.Close()
			src, e := z.ReadFile("RejectOwner$Child.class")
			if e != nil {
				t.Fatal(e)
			}
			if strings.Contains(string(src), "original member body owned by") {
				t.Fatalf("committed a family with added public initializer methods\n%s", src)
			}
		})
	}
}

package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Only the final capturing child is rebuilt. The authored original parent has
// a genuine arbitrary bootstrap with effects/failure; it must stay opaque to
// the capture-motion proof, rather than borrowing a LambdaMetafactory exception.
func TestAdversarialConstructorOpaqueBootstrapPreservesOriginalLinkage(t *testing.T) {
	const fixture = `class LinkageState{static String trace="";static boolean fail;static int links;static final RuntimeException failure=new RuntimeException("original bootstrap");static final Object token=new Object();static final java.util.function.Supplier<Object> supplier=new java.util.function.Supplier<Object>(){public Object get(){trace+="T";return token;}};}
class LinkageParent{final Object seen;final int number;LinkageParent(int n){LinkageState.trace+="L";java.util.function.Supplier<Object> source=()->null;seen=source.get();number=n;LinkageState.trace+="R";}public static java.lang.invoke.CallSite originalBootstrap(java.lang.invoke.MethodHandles.Lookup lookup,String name,java.lang.invoke.MethodType type,Object...args){LinkageState.trace+="B";LinkageState.links++;if(LinkageState.fail)throw LinkageState.failure;return new java.lang.invoke.ConstantCallSite(java.lang.invoke.MethodHandles.constant(java.util.function.Supplier.class,LinkageState.supplier).asType(type));}}
class LinkageOwner{final Object token;LinkageOwner(Object value){token=value;}final class Child extends LinkageParent{Child(int n){super(n);}Object capture(){return LinkageOwner.this.token;}}}
class LinkageDriver{public static void main(String[]args)throws Exception{String[] cp=System.getProperty("java.class.path").split(java.util.regex.Pattern.quote(java.io.File.pathSeparator));if(cp.length>1){java.io.File candidate=new java.io.File(cp[0]).getCanonicalFile(),original=new java.io.File(cp[1]).getCanonicalFile();if(!new java.io.File(LinkageOwner.Child.class.getProtectionDomain().getCodeSource().getLocation().toURI()).getCanonicalFile().equals(candidate)||!new java.io.File(LinkageParent.class.getProtectionDomain().getCodeSource().getLocation().toURI()).getCanonicalFile().equals(original)||!new java.io.File(LinkageOwner.class.getProtectionDomain().getCodeSource().getLocation().toURI()).getCanonicalFile().equals(original)||!new java.io.File(LinkageState.class.getProtectionDomain().getCodeSource().getLocation().toURI()).getCanonicalFile().equals(original))throw new AssertionError("code origin");}LinkageState.fail=args.length>0;if(LinkageState.fail){try{new LinkageOwner(null).new Child(17);throw new AssertionError("missing bootstrap failure");}catch(BootstrapMethodError error){if(error.getCause()!=LinkageState.failure||LinkageState.links!=1||!LinkageState.trace.equals("LB"))throw new AssertionError("original bootstrap cause/order",error);}System.out.println("bootstrap:original:failure:identity");return;}Object x=new Object();int rows=0;for(Object value:new Object[]{null,x})for(int n:new int[]{Integer.MIN_VALUE,0,Integer.MAX_VALUE}){LinkageState.trace="";LinkageOwner.Child child=new LinkageOwner(value).new Child(n);if(child.capture()!=value||child.seen!=LinkageState.token||child.number!=n||LinkageState.links!=1||!LinkageState.trace.equals(rows==0?"LBTR":"LTR"))throw new AssertionError("capture/linkage/target/order:"+LinkageState.trace);rows++;}System.out.println(rows+":bootstrap:original:effects:capture");}}`
	javac, java := t04Tools(t)
	for _, prefix := range []string{"Linkage", "Opaque"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(prefix+"/"+debug, func(t *testing.T) {
				files := nativeCompileDebugClasses(t, strings.ReplaceAll(fixture, "Linkage", prefix), debug)
				child, driver := prefix+"Owner$Child", prefix+"Driver"
				parent, e := Parse(append([]byte(nil), files[prefix+"Parent.class"]...))
				if e != nil {
					t.Fatal(e)
				}
				utf := func(value string) uint16 { return sourceBridgePoolString(t, parent, value) }
				name, descriptor := utf("originalBootstrap"), utf("(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;[Ljava/lang/Object;)Ljava/lang/invoke/CallSite;")
				parent.ConstantPool = append(parent.ConstantPool, &ConstantNameAndTypeInfo{NameIndex: name, DescriptorIndex: descriptor})
				methodType := uint16(len(parent.ConstantPool))
				parent.ConstantPool = append(parent.ConstantPool, &ConstantMethodrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: parent.ThisClass, NameAndTypeIndex: methodType}})
				methodIndex := uint16(len(parent.ConstantPool))
				parent.ConstantPool = append(parent.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 6, ReferenceIndex: methodIndex})
				handle := uint16(len(parent.ConstantPool))
				changed := 0
				for _, a := range parent.Attributes {
					if table, ok := a.(*BootstrapMethodsAttribute); ok {
						for _, boot := range table.BootstrapMethods {
							boot.BootstrapMethodRef = handle
							changed++
						}
					}
				}
				if changed != 1 {
					t.Fatal("fixture must have exactly one original constructor call site", changed)
				}
				files[prefix+"Parent.class"] = parent.Bytes()
				original, retained := t.TempDir(), t.TempDir()
				for n, raw := range files {
					if e := os.WriteFile(filepath.Join(original, n), raw, 0600); e != nil {
						t.Fatal(e)
					}
					if n != child+".class" {
						if e := os.WriteFile(filepath.Join(retained, n), raw, 0600); e != nil {
							t.Fatal(e)
						}
					}
				}
				run := func(t *testing.T, cp, mode, want string) {
					t.Helper()
					args := []string{"-Xverify:all", "-cp", cp, driver}
					if mode != "" {
						args = append(args, mode)
					}
					got, e := exec.Command(java, args...).CombinedOutput()
					if e != nil || string(got) != want {
						t.Fatalf("original driver %s:%v\n%s want=%s", mode, e, got, want)
					}
				}
				const success = "6:bootstrap:original:effects:capture\n"
				const failure = "bootstrap:original:failure:identity\n"
				run(t, original, "", success)
				run(t, original, "failure", failure)
				resolve := func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return b, ok }
				for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
					t.Run(string(mode), func(t *testing.T) {
						var source string
						var err error
						if mode == "legacy" {
							source, err = DecompileWithResolver(files[child+".class"], resolve)
						} else {
							var result DecompileResult
							result, err = DecompileWithOptions(files[child+".class"], DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
							source = result.Source
						}
						if err != nil || strings.Contains(source, DecompileStubMarker) {
							t.Fatalf("source:%v\n%s", err, source)
						}
						out := t.TempDir()
						file := filepath.Join(out, child+".java")
						if err = os.WriteFile(file, []byte(source), 0600); err != nil {
							t.Fatal(err)
						}
						// Supplying the source owner prevents ClassReader from
						// reattaching the flat child's symbol using a binary owner's
						// InnerClasses row and adding a second enclosing parameter.
						// This is real decompiled owner source, not a guessed ABI stub.
						ownerSource, err := DecompileWithOptions(files[prefix+"Owner.class"], DecompileOptions{Mode: Precision, TargetSourceVersion: 8, Resolve: resolve})
						if err != nil || strings.Contains(ownerSource.Source, DecompileStubMarker) {
							t.Fatalf("owner source:%v\n%s", err, ownerSource.Source)
						}
						ownerFile := filepath.Join(out, prefix+"Owner.java")
						if err = os.WriteFile(ownerFile, []byte(ownerSource.Source), 0600); err != nil {
							t.Fatal(err)
						}
						if log, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", retained, "-d", out, ownerFile, file).CombinedOutput(); err != nil {
							t.Fatalf("rebuild:%v\n%s\n%s", err, log, source)
						}
						// Only the selected child replaces original runtime bytes.
						if err = os.Remove(filepath.Join(out, prefix+"Owner.class")); err != nil {
							t.Fatal(err)
						}
						originalChild, err := Parse(append([]byte(nil), files[child+".class"]...))
						if err != nil {
							t.Fatal(err)
						}
						expected := map[string]bool{}
						for _, m := range originalChild.Methods {
							n, _ := sourceBridgeUTF8(originalChild, m.NameIndex)
							if n == "<init>" {
								descriptor, _ := sourceBridgeUTF8(originalChild, m.DescriptorIndex)
								expected[descriptor] = true
							}
						}
						compiledRaw, err := os.ReadFile(filepath.Join(out, child+".class"))
						if err != nil {
							t.Fatal(err)
						}
						compiled, err := Parse(compiledRaw)
						if err != nil {
							t.Fatal(err)
						}
						for _, m := range compiled.Methods {
							n, _ := sourceBridgeUTF8(compiled, m.NameIndex)
							if n == "<init>" {
								descriptor, _ := sourceBridgeUTF8(compiled, m.DescriptorIndex)
								if !expected[descriptor] {
									t.Fatalf("changed original constructor descriptor: %s", descriptor)
								}
								delete(expected, descriptor)
							}
						}
						if len(expected) != 0 {
							t.Fatalf("missing original constructors: %v", expected)
						}
						cp := out + string(os.PathListSeparator) + retained
						run(t, cp, "", success)
						run(t, cp, "failure", failure)
					})
				}
			})
		}
	}
}

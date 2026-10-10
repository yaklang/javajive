package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Old compilers recorded Synthetic as an attribute, while modern compilers
// usually use ACC_SYNTHETIC. The original JVM reflection/callback oracle owns
// their semantic equivalence; simply accepting a matching field name is unsafe.
func TestNativeLegacyMemberCaptureAndSyntheticReflection(t *testing.T) {
	const source = `class LegacyEffects{static Object published;static java.io.IOException failure=new java.io.IOException("same");}
class LegacyParent{final Object observed;final long number;LegacyParent(long n)throws java.io.IOException{observed=owner();number=n;LegacyEffects.published=this;if(n<0)throw LegacyEffects.failure;}Object owner(){return null;}}
class LegacyOwner{class Child extends LegacyParent{Child(long n)throws java.io.IOException{super(n);}Object owner(){return LegacyOwner.this;}}Child make(long n)throws java.io.IOException{return new Child(n);}}
class LegacyExternal{static LegacyOwner.Child make(LegacyOwner o,long n)throws java.io.IOException{return o.new Child(n);}}
class LegacyDriver{public static void main(String[]args)throws Exception{LegacyOwner outer=new LegacyOwner();int rows=0;for(long n:new long[]{-1,0,Long.MAX_VALUE})for(boolean external:new boolean[]{false,true}){LegacyEffects.published=null;try{LegacyOwner.Child c=external?LegacyExternal.make(outer,n):outer.make(n);if(c.observed!=outer||c.number!=n||c.owner()!=outer)throw new AssertionError("capture");}catch(java.io.IOException e){LegacyOwner.Child c=(LegacyOwner.Child)LegacyEffects.published;if(n>=0||e!=LegacyEffects.failure||c.observed!=outer||c.number!=n||c.owner()!=outer)throw new AssertionError("publication");}rows++;}java.lang.reflect.Field f=LegacyOwner.Child.class.getDeclaredField("this$0");if(!f.isSynthetic()||!java.lang.reflect.Modifier.isFinal(f.getModifiers())||f.getType()!=LegacyOwner.class||!LegacyOwner.Child.class.isMemberClass()||LegacyOwner.Child.class.getDeclaringClass()!=LegacyOwner.class)throw new AssertionError("reflection");System.out.println(rows+":"+f.getModifiers()+":"+LegacyOwner.Child.class.getDeclaredConstructor(LegacyOwner.class,long.class).getParameterCount());}}
`
	javac, java := t04Tools(t)
	for _, debug := range []string{"none", "vars"} {
		for _, encoding := range []string{"modern flag", "modern attribute", "both", "legacy48 attribute", "legacy45 attribute"} {
			t.Run(encoding+"/"+debug, func(t *testing.T) {
				files := nativeCompileDebugClasses(t, source, debug)
				for _, name := range []string{"LegacyOwner", "LegacyOwner$Child"} {
					obj, err := Parse(files[name+".class"])
					if err != nil {
						t.Fatal(err)
					}
					if strings.HasPrefix(encoding, "legacy48") {
						obj.MajorVersion = 48
					}
					if strings.HasPrefix(encoding, "legacy45") {
						obj.MajorVersion = 45
						obj.MinorVersion = 3
					}
					if name == "LegacyOwner$Child" && encoding != "modern flag" {
						field := obj.Fields[0]
						if n, _ := sourceBridgeUTF8(obj, field.NameIndex); n != "this$0" {
							t.Fatal("capture field")
						}
						obj.ConstantPoolManager.AddUtf8Info("Synthetic")
						field.Attributes = append(field.Attributes, &SyntheticAttribute{})
						if encoding != "both" {
							field.AccessFlags &^= 0x1000
						}
					}
					// Current javac emits MethodParameters even without -parameters.
					// It did not exist in these legacy formats; retain only the
					// authored Code/Exceptions while constructing the old fixture.
					if obj.MajorVersion < 49 {
						for _, method := range obj.Methods {
							attributes := method.Attributes[:0]
							for _, a := range method.Attributes {
								if raw, ok := a.(*UnparsedAttribute); ok && raw.Name == "MethodParameters" {
									continue
								}
								attributes = append(attributes, a)
							}
							method.Attributes = attributes
						}
					}
					files[name+".class"] = obj.Bytes()
				}
				original := t.TempDir()
				for name, raw := range files {
					if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
				oracle := t04RunJava(t, java, original, "LegacyDriver")
				for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
					t.Run(policy, func(t *testing.T) {
						if policy == "no-source-rewrites" {
							t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
						}
						if policy == "no-core-cleanups" {
							t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
						}
						z := nativeArchive(t, files)
						child, err := z.ReadFile("LegacyOwner$Child.class")
						if err != nil || !strings.Contains(string(child), "original member body owned by") {
							t.Fatalf("child ownership %v %s", err, child)
						}
						output := t.TempDir()
						for name, raw := range files {
							if name == "LegacyOwner.class" || name == "LegacyOwner$Child.class" || name == "LegacyExternal.class" {
								continue
							}
							if err := os.WriteFile(filepath.Join(output, name), raw, 0600); err != nil {
								t.Fatal(err)
							}
						}
						args := []string{"-proc:none", "--release", "8", "-cp", output, "-d", output}
						for _, name := range []string{"LegacyOwner", "LegacyExternal"} {
							src, err := z.ReadFile(name + ".class")
							if err != nil || strings.Contains(string(src), DecompileStubMarker) {
								t.Fatalf("source %v %s", err, src)
							}
							file := filepath.Join(output, name+".java")
							if err := os.WriteFile(file, src, 0600); err != nil {
								t.Fatal(err)
							}
							args = append(args, file)
						}
						if out, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
							t.Fatalf("rebuilt %v %s", err, out)
						}
						if got := t04RunJava(t, java, output, "LegacyDriver"); got != oracle {
							t.Fatalf("runtime/reflection changed %s != %s", got, oracle)
						}
					})
				}
			})
		}
	}
}

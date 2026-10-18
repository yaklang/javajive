package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/jdecenv"
)

const ordinaryConstructorMemberFixture = `
class MemberNewState {
 static String trace="";static Object published;
 static final RuntimeException failure=new IllegalStateException("same");
 static boolean argument(boolean fail,int phase){trace+="A";if(phase==1)throw failure;return fail;}
}
class MemberNewOwner<T> {
 class Inner {
  final RuntimeException stored;
  private <Witness extends Throwable>Inner(boolean fail,RuntimeException value)throws Witness{
   MemberNewState.trace+="C";MemberNewState.published=this;stored=value;
   if(fail)throw value;MemberNewState.trace+="E";
  }
  RuntimeException stored(){return stored;}
  MemberNewOwner<T> outer(){return MemberNewOwner.this;}
 }
 Object make(boolean fail,RuntimeException value,int phase){MemberNewState.trace+="F";return new Inner(MemberNewState.argument(fail,phase),value);}
 static Object raw(MemberNewOwner owner,boolean fail,RuntimeException value,int phase)throws Throwable{MemberNewState.trace+="F";return owner.new Inner(MemberNewState.argument(fail,phase),value);}
}
class MemberNewDriver {
 public static void main(String[]args){MemberNewOwner<String> owner=new MemberNewOwner<String>();int rows=0;
  for(int kind=0;kind<2;kind++){
   MemberNewState.trace="";MemberNewState.published=null;Throwable argumentFailure=null;
   try{if(kind==0)owner.make(false,null,1);else MemberNewOwner.raw(owner,false,null,1);}catch(Throwable actual){argumentFailure=actual;}
   if(argumentFailure!=MemberNewState.failure||MemberNewState.published!=null||!MemberNewState.trace.equals("FA"))throw new AssertionError("argument order",argumentFailure);rows++;
   for(boolean fail:new boolean[]{false,true})for(RuntimeException value:new RuntimeException[]{MemberNewState.failure,new IllegalArgumentException("other"),null}){
    Object result=null;Throwable caught=null;MemberNewState.trace="";MemberNewState.published=null;
    try{result=kind==0?owner.make(fail,value,0):MemberNewOwner.raw(owner,fail,value,0);}catch(Throwable actual){caught=actual;}
    if(fail?(value==null?!(caught instanceof NullPointerException):caught!=value):caught!=null)throw new AssertionError("exception identity",caught);
    if(!MemberNewState.trace.equals(fail?"FAC":"FACE")||MemberNewState.published==null)throw new AssertionError("effects:"+MemberNewState.trace);
    if(!fail){MemberNewOwner<?>.Inner inner=(MemberNewOwner<?>.Inner)result;if(result!=MemberNewState.published||inner.outer()!=owner||inner.stored()!=value)throw new AssertionError("outer/receiver/argument identity");}rows++;
   }
  }
  MemberNewState.trace="";MemberNewState.published=null;Throwable nullOuter=null;
  try{MemberNewOwner.raw(null,false,null,0);}catch(Throwable actual){nullOuter=actual;}
  if(!(nullOuter instanceof NullPointerException)||MemberNewState.published!=null||!MemberNewState.trace.equals("F"))throw new AssertionError("null enclosing must fail before argument effects",nullOuter);rows++;
  System.out.println(rows+":member:new:identity:methods="+MemberNewOwner.class.getDeclaredMethods().length);
 }
}`

func TestAdversarialOrdinaryConstructorMemberProjectionKeepsOriginalABI(t *testing.T) {
	modern, java := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			javac := modern
			if profile == NativeJavac8 {
				javac = os.Getenv("JAVA8_JAVAC")
				if javac == "" {
					t.Skip("JAVA8_JAVAC required for the independent compiler")
				}
				if version, err := exec.Command(javac, "-version").CombinedOutput(); err != nil || !strings.Contains(string(version), "javac 1.8.") {
					t.Fatalf("native8 profile requires actual javac8: %v / %s", err, version)
				}
			}
			for _, owner := range []string{"MemberNewOwner", "AlternateMemberAllocation"} {
				t.Run(owner, func(t *testing.T) {
					fixture := strings.ReplaceAll(ordinaryConstructorMemberFixture, "MemberNewOwner", owner)
					owned := func(name string) bool { return name == owner+".class" || strings.HasPrefix(name, owner+"$") }
					for _, debug := range []string{"none", "source,lines,vars"} {
						t.Run(debug, func(t *testing.T) {
							var files map[string][]byte
							if profile == NativeJavac8 {
								files = nativePrivateEnumCompile(t, fixture, owner, debug)
							} else {
								files = nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": fixture}, debug, "8")
							}
							write := func(dir, name string, raw []byte) {
								t.Helper()
								if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
									t.Fatal(err)
								}
							}
							before := map[string][]byte{}
							original := t.TempDir()
							var ownedNames []string
							for name, raw := range files {
								before[name] = bytes.Clone(raw)
								write(original, name, raw)
								if owned(name) {
									ownedNames = append(ownedNames, name)
								}
							}
							sort.Strings(ownedNames)
							const want = "15:member:new:identity:methods=2\n"
							if got := t04RunJava(t, java, original, "MemberNewDriver"); got != want {
								t.Fatalf("independent original oracle: %q", got)
							}
							t.Log("independent original member JVM: " + strings.TrimSpace(want))
							t.Run("archive-default", func(t *testing.T) {
								var source []byte
								err := jdecenv.Run(map[string]string{}, func() error {
									archive := nativeEnumOwnUnitArchive(t, files, profile)
									var readErr error
									source, readErr = archive.ReadFile(owner + ".class")
									return readErr
								})
								if err != nil || len(source) == 0 || strings.Contains(string(source), DecompileStubMarker) {
									t.Fatalf("source: %v\n%s", err, source)
								}
								out := t.TempDir()
								for name, raw := range files {
									if !owned(name) {
										write(out, name, raw)
									}
								}
								write(out, owner+".java", source)
								args := []string{"-proc:none", "--release", "8"}
								if profile == NativeJavac8 {
									args = []string{"-proc:none", "-source", "8", "-target", "8"}
								}
								args = append(args, "-g:"+debug, "-cp", out, "-d", out, filepath.Join(out, owner+".java"))
								if data, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
									t.Fatalf("rebuild without any original owned classes: %v\n%s\n%s", err, data, source)
								}
								if got := t04RunJava(t, java, out, "MemberNewDriver"); got != want {
									t.Fatalf("unchanged binary caller: %q want %q\n%s", got, want, source)
								}
								entries, err := os.ReadDir(out)
								if err != nil {
									t.Fatal(err)
								}
								var rebuiltNames []string
								for _, entry := range entries {
									if owned(entry.Name()) && strings.HasSuffix(entry.Name(), ".class") {
										rebuiltNames = append(rebuiltNames, entry.Name())
									}
								}
								sort.Strings(rebuiltNames)
								if strings.Join(ownedNames, "\n") != strings.Join(rebuiltNames, "\n") {
									t.Fatalf("owned class set changed: %v / %v", ownedNames, rebuiltNames)
								}
								for _, name := range ownedNames {
									rebuilt, err := os.ReadFile(filepath.Join(out, name))
									if err != nil {
										t.Fatal(err)
									}
									if a, b := constructorInvocationThrowsDeclarations(t, files[name]), constructorInvocationThrowsDeclarations(t, rebuilt); a != b {
										t.Fatalf("complete declaration ABI %s:\n%s\n%s", name, a, b)
									}
								}
								for name, raw := range files {
									if !bytes.Equal(raw, before[name]) {
										t.Fatal("original input/Driver changed", name)
									}
								}
								t.Log("fully rebuilt member JVM: " + strings.TrimSpace(want))
							})
						})
					}
				})
			}
		})
	}
}

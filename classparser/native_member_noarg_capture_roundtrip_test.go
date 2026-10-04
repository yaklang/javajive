package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeMemberImplicitNoArgSuperclassKeepsPreSuperCapture(t *testing.T) {
	const source = `class ZeroEffects{static String trace="";static Object published;static boolean fail;static IllegalArgumentException failure=new IllegalArgumentException("same");}
class ZeroParent{static{ZeroEffects.trace+="I";}final Object observed;ZeroParent(){ZeroEffects.trace+="P";observed=owner();ZeroEffects.published=this;if(ZeroEffects.fail)throw ZeroEffects.failure;}Object owner(){return null;}}
class ZeroOwner{class Child extends ZeroParent{final long number;Child(){this(17);}Child(long n){super();ZeroEffects.trace+="C";number=n;}Object owner(){return ZeroOwner.this;}}Child make(long n){return new Child(n);}Child chain(){return new Child();}}
class ZeroExternal{static ZeroOwner.Child make(ZeroOwner outer,long n){return outer.new Child(n);}}
class ZeroDriver{public static void main(String[]args){ZeroOwner outer=new ZeroOwner();int rows=0;for(boolean fail:new boolean[]{false,true})for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE})for(int mode=0;mode<3;mode++){ZeroEffects.fail=fail;ZeroEffects.trace="";ZeroEffects.published=null;long expected=mode==2?17:n;try{ZeroOwner.Child c=mode==0?outer.make(n):mode==1?ZeroExternal.make(outer,n):outer.chain();if(fail||c.observed!=outer||c.owner()!=outer||c.number!=expected||ZeroEffects.published!=c||!ZeroEffects.trace.equals(rows==0?"IPC":"PC"))throw new AssertionError("success");}catch(IllegalArgumentException e){ZeroOwner.Child c=(ZeroOwner.Child)ZeroEffects.published;if(!fail||e!=ZeroEffects.failure||c.observed!=outer||c.owner()!=outer||c.number!=0||!ZeroEffects.trace.equals("P"))throw new AssertionError("failure/publication");}rows++;}try{ZeroExternal.make(null,3);throw new AssertionError("null qualifier");}catch(NullPointerException expected){}System.out.println(rows+":"+ZeroEffects.trace+":"+ZeroOwner.Child.class.getDeclaredConstructors().length);}}
`
	javac, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, source, debug)
			original := t.TempDir()
			for n, b := range files {
				if err := os.WriteFile(filepath.Join(original, n), b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			oracle := t04RunJava(t, java, original, "ZeroDriver")
			for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(policy, func(t *testing.T) {
					if policy == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if policy == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					z := nativeArchive(t, files)
					child, err := z.ReadFile("ZeroOwner$Child.class")
					if err != nil || !strings.Contains(string(child), "original member body owned by") {
						t.Fatalf("ownership %v %s", err, child)
					}
					output := t.TempDir()
					for n, b := range files {
						if n == "ZeroOwner.class" || n == "ZeroOwner$Child.class" || n == "ZeroExternal.class" {
							continue
						}
						if err := os.WriteFile(filepath.Join(output, n), b, 0600); err != nil {
							t.Fatal(err)
						}
					}
					args := []string{"-proc:none", "--release", "8", "-cp", output, "-d", output}
					for _, n := range []string{"ZeroOwner", "ZeroExternal"} {
						src, err := z.ReadFile(n + ".class")
						if err != nil || strings.Contains(string(src), DecompileStubMarker) {
							t.Fatalf("source %v %s", err, src)
						}
						file := filepath.Join(output, n+".java")
						if err := os.WriteFile(file, src, 0600); err != nil {
							t.Fatal(err)
						}
						args = append(args, file)
					}
					if out, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
						t.Fatalf("rebuilt %v %s", err, out)
					}
					if got := t04RunJava(t, java, output, "ZeroDriver"); got != oracle {
						t.Fatalf("original effects/publication changed %s != %s", got, oracle)
					}
					raw, err := os.ReadFile(filepath.Join(output, "ZeroOwner$Child.class"))
					if err != nil {
						t.Fatal(err)
					}
					if want, got := nativeBinaryShape(t, files["ZeroOwner$Child.class"]), nativeBinaryShape(t, raw); want != got {
						t.Fatalf("original ABI changed\n%s\n%s", want, got)
					}
				})
			}
		})
	}
}

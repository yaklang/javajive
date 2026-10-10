package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// An anonymous declaration emitted inside the original owner has that owner's
// private member scope. The independent JVM oracle checks dispatch, identity,
// failure priority and the regenerated binary ownership, not source spelling.
const nativeJointPrivateAccessFixture = `class JointPrivateOwner<T>{
 private static class Item<U>{final U value;final long number;Item(U v,long n){value=v;number=n;}}
 static String trace="";static final IllegalArgumentException error=new IllegalArgumentException("same");
 final java.util.Comparator<Item<T>> order=new java.util.Comparator<Item<T>>(){public int compare(Item<T> a,Item<T> b){trace+="C";if(a==null)throw error;if(a==b)return 0;return a.number<b.number?-1:a.number==b.number?0:1;}};
 int check(T value,long x,long y){Item<T> a=new Item<T>(value,x);Item<T> b=new Item<T>(value,y);if(a.value!=value||b.value!=value)throw new AssertionError("identity");return order.compare(a,b);}
 int same(T value){Item<T> a=new Item<T>(value,0);return order.compare(a,a);}
 void fail(){order.compare(null,null);}
}
class JointPrivateDriver{public static void main(String[]args)throws Exception{
 JointPrivateOwner<Object> o=new JointPrivateOwner<Object>();Object token=new Object();int rows=0;
 for(Object v:new Object[]{null,token})for(long x:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE})for(long y:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){JointPrivateOwner.trace="";int actual=o.check(v,x,y);int expected=Long.compare(x,y);if(actual!=expected||!JointPrivateOwner.trace.equals("C"))throw new AssertionError("compare");rows++;}
 JointPrivateOwner.trace="";if(o.same(token)!=0||!JointPrivateOwner.trace.equals("C"))throw new AssertionError("same");
 JointPrivateOwner.trace="";try{o.fail();throw new AssertionError("no failure");}catch(IllegalArgumentException e){if(e!=JointPrivateOwner.error||!JointPrivateOwner.trace.equals("C"))throw new AssertionError("failure");}
 Class<?> anon=o.order.getClass();if(!anon.isAnonymousClass()||anon.getEnclosingClass()!=JointPrivateOwner.class)throw new AssertionError("anonymous ownership");
 java.lang.reflect.Field f=JointPrivateOwner.class.getDeclaredField("order");Class<?> order=Class.forName("JointPrivateOwner$Item");if(order.getDeclaringClass()!=JointPrivateOwner.class||!java.lang.reflect.Modifier.isPrivate(order.getModifiers()))throw new AssertionError("private ownership");
 System.out.println(rows+":"+JointPrivateOwner.trace+":"+anon.getName());
}}`

func TestNativeJointAnonymousPrivateMemberAccessRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, nativeJointPrivateAccessFixture, debug)
			original := t.TempDir()
			for n, raw := range files {
				if err := os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			oracle := t04RunJava(t, java, original, "JointPrivateDriver")
			if oracle != "18:C:JointPrivateOwner$1\n" {
				t.Fatalf("original oracle %q", oracle)
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
					owned, err := z.ReadFile("JointPrivateOwner$Item.class")
					if err != nil || !strings.Contains(string(owned), "original member body owned by") {
						t.Fatalf("ownership %v\n%s", err, owned)
					}
					src, err := z.ReadFile("JointPrivateOwner.class")
					if err != nil || strings.Contains(string(src), DecompileStubMarker) {
						t.Fatalf("source %v\n%s", err, src)
					}
					output := t.TempDir()
					for n, raw := range files {
						if strings.HasPrefix(n, "JointPrivateOwner") {
							continue
						}
						if err := os.WriteFile(filepath.Join(output, n), raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					file := filepath.Join(output, "JointPrivateOwner.java")
					if err := os.WriteFile(file, src, 0600); err != nil {
						t.Fatal(err)
					}
					if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", output, "-d", output, file).CombinedOutput(); err != nil {
						t.Fatalf("rebuild %v %s\n%s", err, out, src)
					}
					if got := t04RunJava(t, java, output, "JointPrivateDriver"); got != oracle {
						t.Fatalf("JVM mismatch %q != %q", got, oracle)
					}
					for _, n := range []string{"JointPrivateOwner", "JointPrivateOwner$Item", "JointPrivateOwner$1"} {
						raw, err := os.ReadFile(filepath.Join(output, n+".class"))
						if err != nil {
							t.Fatal(err)
						}
						if want, got := nativeBinaryShape(t, files[n+".class"]), nativeBinaryShape(t, raw); want != got {
							t.Fatalf("ABI %s\n%s\n%s", n, want, got)
						}
					}
				})
			}
		})
	}
}

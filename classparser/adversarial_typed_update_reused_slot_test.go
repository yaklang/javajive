package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A rejected-input arm uses the same physical slots as a category-2 cursor.
// Joined loops then consume several updates in sibling source scopes. The
// driver stays original and checks every byte's order and the selected range.
const typedUpdateReusedSlotFixture = `class ReusedCursorOwner {
 long scan(byte[] values,int start,int count){if((start|count|((values.length-start)-count))<0){Object[] scratch=new Object[]{values.length,start,count};throw new IndexOutOfBoundsException(java.util.Arrays.toString(scratch));}else{long cursor=start;long end=cursor+count;long total=0;do{if(cursor<end){byte head=ReusedCursorHelper.read(cursor);if((head&1)!=0)break;cursor++;total=total*31+head;}else break;}while(true);OUT:do{if(cursor<end){byte head=ReusedCursorHelper.read(cursor++);total=total*31+head;if((head&1)==0){do{if(cursor<end){byte tail=ReusedCursorHelper.read(cursor);if((tail&1)!=0)continue OUT;cursor++;total=total*31+tail;continue;}else continue OUT;}while(true);}else if((head&3)==1){if(cursor<end){byte tail=ReusedCursorHelper.read(cursor++);total=total*31+tail;continue;}}else{if(cursor<end){byte second=ReusedCursorHelper.read(cursor++);total=total*31+second;if(cursor<end){byte third=ReusedCursorHelper.read(cursor++);total=total*31+third;continue;}}}}else break;}while(true);return total;}}
}
class ReusedCursorHelper{static byte[] values;static byte read(long position){return values[(int)position];}}
class ReusedCursorDriver{public static void main(String[]args){ReusedCursorOwner c=new ReusedCursorOwner();int rows=0;for(int length=0;length<14;length++){for(int pattern=0;pattern<7;pattern++){byte[] input=new byte[length];for(int i=0;i<length;i++)input[i]=(byte)(pattern==0?0:pattern==1?1:pattern==2?3:pattern==3?-1:pattern==4?i:pattern==5?-i:(i*73+19));ReusedCursorHelper.values=input;for(int start=0;start<=length;start++){for(int count=0;count<=length-start;count++){long expected=0;for(int i=0;i<count;i++)expected=expected*31+input[start+i];long actual=c.scan(input,start,count);if(actual!=expected)throw new AssertionError("range/order length="+length+" pattern="+pattern+" start="+start+" count="+count+" got="+actual+" expected="+expected);rows++;}}}}System.out.println("reused:cursor:"+rows);}}`

func TestAdversarialTypedUpdatesPreserveReusedSlotAcrossSiblingScopes(t *testing.T) {
	javac, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, typedUpdateReusedSlotFixture, debug)
			original := t.TempDir()
			for n, raw := range files {
				if e := os.WriteFile(filepath.Join(original, n), raw, 0600); e != nil {
					t.Fatal(e)
				}
			}
			oracle := t04RunJava(t, java, original, "ReusedCursorDriver")
			if oracle != "reused:cursor:3920\n" {
				t.Fatal("independent original", oracle)
			}
			for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
				t.Run(string(mode), func(t *testing.T) {
					resolve := func(n string) ([]byte, bool) { b, k := files[n+".class"]; return b, k }
					var source string
					var err error
					if mode == "legacy" {
						source, err = DecompileWithResolver(files["ReusedCursorOwner.class"], resolve)
					} else {
						var result DecompileResult
						result, err = DecompileWithOptions(files["ReusedCursorOwner.class"], DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
						source = result.Source
					}
					if err != nil || strings.Contains(source, DecompileStubMarker) {
						t.Fatalf("source %v\n%s", err, source)
					}
					out := t.TempDir()
					for _, name := range []string{"ReusedCursorDriver.class", "ReusedCursorHelper.class"} {
						if e := os.WriteFile(filepath.Join(out, name), files[name], 0600); e != nil {
							t.Fatal(e)
						}
					}
					path := filepath.Join(out, "ReusedCursorOwner.java")
					if e := os.WriteFile(path, []byte(source), 0600); e != nil {
						t.Fatal(e)
					}
					if text, e := exec.Command(javac, "-proc:none", "--release", "8", "-cp", out, "-d", out, path).CombinedOutput(); e != nil {
						t.Fatalf("compile %v\n%s\n%s", e, text, source)
					}
					if actual := t04RunJava(t, java, out, "ReusedCursorDriver"); actual != oracle {
						t.Fatal("candidate", actual, "original", oracle)
					}
					raw, e := os.ReadFile(filepath.Join(out, "ReusedCursorOwner.class"))
					if e != nil {
						t.Fatal(e)
					}
					nativeLexicalExactSignatures(t, fmt.Sprint("ReusedCursorOwner.class"), files["ReusedCursorOwner.class"], raw)
				})
			}
		})
	}
}

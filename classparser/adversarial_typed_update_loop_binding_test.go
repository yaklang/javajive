package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Independently retained driver: slot reuse and loop joins can rename the
// declaration after a folded update has been created. Every write must follow
// the same binding as ordinary reads; copying only its current name is unsafe.
func TestAdversarialTypedUpdateLoopRetainsResolvedBinding(t *testing.T) {
	const owner = "TypedLoopBinding"
	const source = `class TypedLoopBinding {
 long read(long index){return index;}
 long loop(boolean invalid,long seed,int count){if(invalid){Object[] scratch=new Object[]{seed,count};if(scratch.length!=2)throw new AssertionError();return seed;}else{long cursor=seed;long end=cursor+count;long total=0;do{if(cursor<end){long first=read(cursor);if(first%3==0)break;cursor++;total+=first;}else break;}while(true);OUT:do{if(cursor<end){long first=read(cursor++);if(first%2==0){total+=first;do{if(cursor<end){long second=read(cursor);if(second%2!=0)continue OUT;cursor++;total+=second;}else continue OUT;}while(true);}else{total+=first;if(cursor<end){total+=read(cursor++);continue;}}}else break;}while(true);return total;}}
}
class TypedLoopDriver{public static void main(String[]args){TypedLoopBinding c=new TypedLoopBinding();int rows=0;for(long seed=-3;seed<4;seed++){for(int count=0;count<7;count++){long expected=0;for(int i=0;i<count;i++)expected+=seed+i;long actual=c.loop(false,seed,count);if(actual!=expected||c.loop(true,seed,count)!=seed)throw new AssertionError("binding seed="+seed+" count="+count+" got="+actual+" expected="+expected);rows++;}}System.out.println("typed:loop:"+rows);}}`
	javac, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, source, debug)
			original := t.TempDir()
			for n, raw := range files {
				if e := os.WriteFile(filepath.Join(original, n), raw, 0600); e != nil {
					t.Fatal(e)
				}
			}
			oracle := t04RunJava(t, java, original, "TypedLoopDriver")
			if oracle != "typed:loop:49\n" {
				t.Fatal("independent original", oracle)
			}
			for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
				t.Run(string(mode), func(t *testing.T) {
					resolve := func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return b, ok }
					var text string
					var err error
					if mode == "legacy" {
						text, err = DecompileWithResolver(files[owner+".class"], resolve)
					} else {
						var r DecompileResult
						r, err = DecompileWithOptions(files[owner+".class"], DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
						text = r.Source
					}
					if err != nil || strings.Contains(text, DecompileStubMarker) {
						t.Fatalf("source %v\n%s", err, text)
					}
					dir := t.TempDir()
					for n, raw := range files {
						if n == owner+".class" {
							continue
						}
						if e := os.WriteFile(filepath.Join(dir, n), raw, 0600); e != nil {
							t.Fatal(e)
						}
					}
					path := filepath.Join(dir, owner+".java")
					if e := os.WriteFile(path, []byte(text), 0600); e != nil {
						t.Fatal(e)
					}
					if out, e := exec.Command(javac, "-proc:none", "--release", "8", "-cp", dir, "-d", dir, path).CombinedOutput(); e != nil {
						t.Fatalf("recompile %v\n%s\n%s", e, out, text)
					}
					if actual := t04RunJava(t, java, dir, "TypedLoopDriver"); actual != oracle {
						t.Fatal("candidate", actual, "original", oracle)
					}
					raw, e := os.ReadFile(filepath.Join(dir, owner+".class"))
					if e != nil {
						t.Fatal(e)
					}
					nativeLexicalExactSignatures(t, fmt.Sprintf("%s.class", owner), files[owner+".class"], raw)
				})
			}
		})
	}
}

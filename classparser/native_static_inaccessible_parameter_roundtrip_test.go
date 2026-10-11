package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The public entry accepts a type that source in the other package cannot name.
// The original call is legal through implicit reference widening. Rebuilding
// only Subject preserves the original binary declarations and untouched oracle.
func TestNativeStaticCallInaccessibleParameterRetainsSelectionAndEffects(t *testing.T) {
	javac, _ := t04Tools(t)
	for _, variant := range []string{"original", "renamed"} {
		t.Run(variant, func(t *testing.T) {
			library, client := "opaque", "consumer"
			if variant == "renamed" {
				library, client = "changedlibrary", "changedcaller"
			}
			texts := map[string]string{
				"opaque/Factory.java":   `package opaque; public class Factory { static abstract class Hidden { final Object token; final long word; Hidden(Object t,long w){token=t;word=w;} } public static String trace="";public static int fail;public static final RuntimeException ERROR=new IllegalStateException("original");static void mark(String s,int n){trace+=s;if(fail==n)throw ERROR;}public static Concrete arg(Concrete c){mark("A",1);return c;}public static int argInt(int i){mark("B",2);return i;}public static long argLong(long i){mark("C",3);return i;} public static Object read(Hidden c){return c==null?null:c.token;}public static long digest(Hidden c,int mask,long salt){return (c==null?Long.MIN_VALUE:c.word)^((long)mask<<32)^salt;}}`,
				"opaque/Concrete.java":  `package opaque; public class Concrete extends Factory.Hidden {public Concrete(Object token,long word){super(token,word);}}`,
				"consumer/Subject.java": `package consumer; import opaque.Factory; public class Subject {public static Object token(opaque.Concrete c){return Factory.read(c);}public static Object nullToken(){return Factory.read(null);}public static long digest(opaque.Concrete c,int mask,long salt){return Factory.digest(Factory.arg(c),Factory.argInt(mask),Factory.argLong(salt));}}`,
				"consumer/Driver.java":  `package consumer; import opaque.Factory; public class Driver{public static void main(String[]args){int rows=0;Object shared=new Object();for(Object token:new Object[]{null,shared,"same",new String("same")})for(long word:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(boolean nil:new boolean[]{false,true})for(int fail=0;fail<4;fail++){opaque.Concrete c=nil?null:new opaque.Concrete(token,word);if(Subject.nullToken()!=null||Subject.token(c)!=(nil?null:token))throw new AssertionError("identity");Factory.trace="";Factory.fail=fail;int mask=(int)(word^0x12345678L);long salt=~word;try{long value=Subject.digest(c,mask,salt);if(fail!=0||value!=((nil?Long.MIN_VALUE:word)^((long)mask<<32)^salt)||!Factory.trace.equals("ABC"))throw new AssertionError("selection/word/order");}catch(RuntimeException e){if(fail==0||e!=Factory.ERROR||!Factory.trace.equals(fail==1?"A":fail==2?"AB":"ABC"))throw new AssertionError("failure identity/order",e);}rows++;}System.out.println(rows+":opaque:static:binding");}}`,
			}
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
				dir := t.TempDir()
				paths := []string{}
				for name, text := range texts {
					name = strings.NewReplacer("opaque", library, "consumer", client).Replace(name)
					text = strings.NewReplacer("opaque", library, "consumer", client).Replace(text)
					path := filepath.Join(dir, filepath.FromSlash(name))
					if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte(text), 0600); err != nil {
						t.Fatal(err)
					}
					paths = append(paths, path)
				}
				args := []string{"-proc:none", "--release", "8", "-g:" + debug, "-d", dir}
				if out, e := exec.Command(javac, append(args, paths...)...).CombinedOutput(); e != nil {
					t.Fatal("original compile", e, string(out))
				}
				files := map[string][]byte{}
				err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if !entry.IsDir() && strings.HasSuffix(path, ".class") {
						name, e := filepath.Rel(dir, path)
						if e != nil {
							return e
						}
						raw, e := os.ReadFile(path)
						if e != nil {
							return e
						}
						files[filepath.ToSlash(name)] = raw
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				return files
			}, ModernJavac, javac, []string{client + "/Subject"}, client+".Driver", "160:"+library+":static:binding\n", nil, nativeLexicalExactSignatures)
		})
	}
}

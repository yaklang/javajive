package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAdversarialForwardedAllocationLocalsKeepDistinctBindingsAndEffects(t *testing.T) {
	javac, java := t04Tools(t)
	for _, owner := range []string{"AllocationOwner", "RenamedAllocationOwner"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(owner+"/"+debug, func(t *testing.T) {
				source := `class AllocationEffects {static String trace="";static final RuntimeException failure=new IllegalStateException("same");static int[] words(int mode){trace+="W";return new int[]{mode,mode+3};}}
class AllocationCell {final int value;AllocationCell(int value){AllocationEffects.trace+="C";if(value==Integer.MIN_VALUE)throw AllocationEffects.failure;this.value=value;}int get(){return value;}}
class ` + owner + ` {static int calculate(int left,int right,int mode){AllocationCell first=new AllocationCell(left);AllocationCell second=new AllocationCell(right);int[] words=AllocationEffects.words(mode);if((mode&1)==0){int value=first.get()*31+second.get();return value+words[0];}else{AllocationCell third=new AllocationCell(first.get());int value=third.get()*17-second.get();return value+words[1];}}}
public class AllocationDriver {public static void main(String[]args){int rows=0;for(int left:new int[]{Integer.MIN_VALUE,-7,0,3,Integer.MAX_VALUE})for(int right:new int[]{Integer.MIN_VALUE,-9,0,5,Integer.MAX_VALUE})for(int mode:new int[]{0,1,2,3}){AllocationEffects.trace="";try{int value=` + owner + `.calculate(left,right,mode);java.math.BigInteger x=java.math.BigInteger.valueOf(left),y=java.math.BigInteger.valueOf(right);int expected=(mode&1)==0?x.multiply(java.math.BigInteger.valueOf(31)).add(y).add(java.math.BigInteger.valueOf(mode)).intValue():x.multiply(java.math.BigInteger.valueOf(17)).subtract(y).add(java.math.BigInteger.valueOf(mode+3)).intValue();if(left==Integer.MIN_VALUE||right==Integer.MIN_VALUE||value!=expected||!AllocationEffects.trace.equals((mode&1)==0?"CCW":"CCWC"))throw new AssertionError("value/order");}catch(Throwable failure){if(failure!=AllocationEffects.failure||left!=Integer.MIN_VALUE&&right!=Integer.MIN_VALUE||!AllocationEffects.trace.equals(left==Integer.MIN_VALUE?"C":"CC"))throw new AssertionError("identity/order",failure);}rows++;}System.out.println(rows+":allocation:identity:bindings:effects");}}`
				dir := t.TempDir()
				path := filepath.Join(dir, "AllocationDriver.java")
				if err := os.WriteFile(path, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
				if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:"+debug, "-d", dir, path).CombinedOutput(); err != nil {
					t.Fatal(err, string(out))
				}
				run := func(cp string) {
					t.Helper()
					out, err := exec.Command(java, "-Xverify:all", "-cp", cp, "AllocationDriver").CombinedOutput()
					if err != nil || string(out) != "100:allocation:identity:bindings:effects\n" {
						t.Fatal("original-first allocation oracle", err, string(out))
					}
				}
				run(dir)
				raw := readClassBytes(t, dir, owner)
				resolve := resolverFromClasses(classMapFromDir(t, dir))
				for _, mode := range []DecompileMode{Precision, Compatibility} {
					for _, policy := range []string{"normal", "no-source-rewrites"} {
						t.Run(fmt.Sprint(mode)+"/"+policy, func(t *testing.T) {
							environment := map[string]string{}
							if policy == "no-source-rewrites" {
								environment["JDEC_NO_SOURCE_REWRITES"] = "1"
							}
							result, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve, EnvSnapshot: environment})
							if err != nil || len(result.StubMethods) != 0 || legacySourceHasFailureMarker(result.Source) {
								t.Fatal("complete source", err, result.Diagnostics)
							}
							out := t.TempDir()
							file := filepath.Join(out, owner+".java")
							if err := os.WriteFile(file, []byte(result.Source), 0600); err != nil {
								t.Fatal(err)
							}
							if log, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:"+debug, "-cp", dir, "-d", out, file).CombinedOutput(); err != nil {
								t.Fatal("fresh declaration bindings", err, string(log), result.Source)
							}
							run(out + string(os.PathListSeparator) + dir)
						})
					}
				}
			})
		}
	}
}

package javaclassparser

import (
	"encoding/binary"
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
			for _, encoding := range []string{"javac", "argument-before-new"} {
				t.Run(owner+"/"+debug+"/"+encoding, func(t *testing.T) {
					source := `class AllocationEffects {static String trace="";static final RuntimeException failure=new IllegalStateException("same");static int[] words(int mode){trace+="W";return new int[]{mode,mode+3};}}
class AllocationCell {final int value;AllocationCell(int value){AllocationEffects.trace+="C";if(value==Integer.MIN_VALUE)throw AllocationEffects.failure;this.value=value;}int get(){return value;}}
class ` + owner + ` {static int calculate(int left,int right,int mode){AllocationCell first=new AllocationCell(left);AllocationCell second=new AllocationCell(right);int[] words=AllocationEffects.words(mode);return first.get()*31+second.get()+words[0];}}
public class AllocationDriver {public static void main(String[]args){int rows=0;for(int left:new int[]{Integer.MIN_VALUE,-7,0,3,Integer.MAX_VALUE})for(int right:new int[]{Integer.MIN_VALUE,-9,0,5,Integer.MAX_VALUE})for(int mode:new int[]{0,1,2,3}){AllocationEffects.trace="";try{int value=` + owner + `.calculate(left,right,mode);java.math.BigInteger x=java.math.BigInteger.valueOf(left),y=java.math.BigInteger.valueOf(right);int expected=x.multiply(java.math.BigInteger.valueOf(31)).add(y).add(java.math.BigInteger.valueOf(mode)).intValue();if(left==Integer.MIN_VALUE||right==Integer.MIN_VALUE||value!=expected||!AllocationEffects.trace.equals("CCW"))throw new AssertionError("value/order");}catch(Throwable failure){if(failure!=AllocationEffects.failure||left!=Integer.MIN_VALUE&&right!=Integer.MIN_VALUE||!AllocationEffects.trace.equals(left==Integer.MIN_VALUE?"C":"CC"))throw new AssertionError("identity/order",failure);}rows++;}System.out.println(rows+":allocation:identity:bindings:effects");}}`
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
					if encoding == "argument-before-new" {
						// An independent JVM producer may evaluate a constructor
						// operand before NEW. DUP_X1/SWAP initializes one retained
						// allocation identity; source must not confuse its name
						// with the later local slots or another allocation.
						object := mustParseLowering(t, readClassBytes(t, dir, owner))
						pool := object.ConstantPoolManager
						cell := pool.AddNewClassInfo("AllocationCell")
						ctor := pool.AddNewMethodInfo("AllocationCell", "<init>", "(I)V")
						get := pool.AddNewMethodInfo("AllocationCell", "get", "()I")
						words := pool.AddNewMethodInfo("AllocationEffects", "words", "(I)[I")
						var body []byte
						emit := func(op byte, index int) { body = append(body, op, byte(index>>8), byte(index)) }
						for i := 0; i < 2; i++ {
							body = append(body, byte(0x1a+i))
							emit(0xbb, cell)
							body = append(body, 0x5a, 0x5f)
							emit(0xb7, ctor)
							body = append(body, 0x3a, byte(3+i))
						}
						body = append(body, 0x1c)
						emit(0xb8, words)
						body = append(body, 0x3a, 5, 0x2d)
						emit(0xb6, get)
						body = append(body, 0x10, 31, 0x68, 0x19, 4)
						emit(0xb6, get)
						body = append(body, 0x60, 0x19, 5, 3, 0x2e, 0x60, 0xac)
						for _, m := range object.Methods {
							if n, _ := sourceBridgeUTF8(object, m.NameIndex); n == "calculate" {
								code := constructorLoweringCode(m)
								code.Code = body
								code.MaxStack = 4
								code.MaxLocals = 6
								code.ExceptionTable = nil
								code.Attributes = nil
								code.AttrLen = uint32(12 + len(body))
								if debug != "none" {
									pool.AddUtf8Info("LineNumberTable")
									info := make([]byte, 6)
									binary.BigEndian.PutUint16(info, 1)
									binary.BigEndian.PutUint16(info[4:], 7)
									code.Attributes = []AttributeInfo{&UnparsedAttribute{Name: "LineNumberTable", Length: 6, Info: info}}
									code.AttrLen += 12
								}
							}
						}
						if err := os.WriteFile(filepath.Join(dir, owner+".class"), object.Bytes(), 0600); err != nil {
							t.Fatal(err)
						}
					}
					run(dir)
					raw := readClassBytes(t, dir, owner)
					files := classMapFromDir(t, dir)
					resolve := resolverFromClasses(files)
					support := map[string][]byte{}
					for name, raw := range files {
						if name != owner {
							support[name+".class"] = raw
						}
					}
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
								out := sourceArtifactWriteFiles(t, support)
								file := filepath.Join(out, owner+".java")
								if err := os.WriteFile(file, []byte(result.Source), 0600); err != nil {
									t.Fatal(err)
								}
								if log, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, file).CombinedOutput(); err != nil {
									t.Fatal("fresh declaration bindings", err, string(log), result.Source)
								}
								run(out)
							})
						}
					}
				})
			}
		}
	}
}

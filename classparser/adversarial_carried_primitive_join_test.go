package javaclassparser

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Authored bytecode retains an accumulator across each conditional. One edge
// leaves that word untouched, while the other consumes and replaces it. Equal
// stack heights are therefore not proof of equal incoming values. Stack maps
// and the arithmetic/exception oracle are independent of the decompiler.
func TestAdversarialCarriedPrimitiveJoinKeepsEverySelectedContribution(t *testing.T) {
	javac, java := t04Tools(t)
	for _, wide := range []bool{false, true} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, methodName := range []string{"fold", "renamedCombine"} {
				t.Run(fmt.Sprintf("wide%v/%s/%s", wide, debug, methodName), func(t *testing.T) {
					typ, desc, valueMethod := "int", "I", "word"
					if wide {
						typ, desc, valueMethod = "long", "J", "wide"
					}
					fixture := `class CarryEffects {static String trace=""; static int fail; static final RuntimeException failure=new IllegalArgumentException("identity"); static ` + typ + ` finish(` + typ + ` n){trace+="F";return n^93;}}
class CarryWord {final int id;CarryWord(int i){id=i;} int word(){CarryEffects.trace+=id;if(CarryEffects.fail==id)throw CarryEffects.failure;return id==1?Integer.MAX_VALUE:id==2?-5:Integer.MIN_VALUE;}long wide(){CarryEffects.trace+=id;if(CarryEffects.fail==id)throw CarryEffects.failure;return id==1?Long.MAX_VALUE:id==2?-5:Long.MIN_VALUE;}}
class CarryOwner {static ` + typ + ` ` + methodName + `(` + typ + ` seed,CarryWord a,CarryWord b,CarryWord c){return seed;}}
public class CarryDriver {public static void main(String[] args){int rows=0;for(long seed:new long[]{Long.MIN_VALUE,Long.MAX_VALUE,-1,0,1,2147483647L})for(int mask=0;mask<8;mask++)for(int fail=0;fail<4;fail++){
 CarryWord[] words=new CarryWord[3];for(int i=0;i<3;i++)if((mask&(1<<i))!=0)words[i]=new CarryWord(i+1);
 java.math.BigInteger expected=java.math.BigInteger.valueOf((` + typ + `)seed);String trace="";boolean throwsExpected=false;
 for(int i=0;i<3;i++){expected=expected.multiply(java.math.BigInteger.valueOf(31));if(words[i]!=null){trace+=(i+1);if(fail==i+1){throwsExpected=true;break;}long contribution=i==0?` + map[bool]string{false: "Integer.MAX_VALUE", true: "Long.MAX_VALUE"}[wide] + `:i==1?-5:` + map[bool]string{false: "Integer.MIN_VALUE", true: "Long.MIN_VALUE"}[wide] + `;expected=expected.add(java.math.BigInteger.valueOf(contribution));}}
 CarryEffects.trace="";CarryEffects.fail=fail;try{` + typ + ` got=CarryOwner.` + methodName + `((` + typ + `)seed,words[0],words[1],words[2]);if(throwsExpected||got!=((` + typ + `)expected.longValue()^93)||!CarryEffects.trace.equals(trace+"F"))throw new AssertionError("contribution/order "+mask+":"+seed+":"+got+":"+CarryEffects.trace);}catch(RuntimeException e){if(!throwsExpected||e!=CarryEffects.failure||!CarryEffects.trace.equals(trace))throw new AssertionError("exception identity/order",e);}rows++;}System.out.println(rows+":carried:primitive:order:overflow:exception");}}`
					dir := t.TempDir()
					path := filepath.Join(dir, "CarryDriver.java")
					if err := os.WriteFile(path, []byte(fixture), 0600); err != nil {
						t.Fatal(err)
					}
					if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:"+debug, "-d", dir, path).CombinedOutput(); err != nil {
						t.Fatal(err, string(out))
					}
					obj := mustParseLowering(t, readClassBytes(t, dir, "CarryOwner"))
					pool := obj.ConstantPoolManager
					call := pool.AddNewMethodInfo("CarryWord", valueMethod, "()"+desc)
					finish := pool.AddNewMethodInfo("CarryEffects", "finish", "("+desc+")"+desc)
					var method *MemberInfo
					for _, m := range obj.Methods {
						if n, _ := sourceBridgeUTF8(obj, m.NameIndex); n == methodName {
							method = m
						}
					}
					if method == nil {
						t.Fatal("fixture method")
					}
					code := constructorLoweringCode(method)
					body := []byte{0x1a}
					wordTag := byte(1)
					firstLocal := 1
					if wide {
						body = []byte{0x1e}
						wordTag = 4
						firstLocal = 2
					}
					frames := []byte{0, 3}
					previous := -1
					for i := 0; i < 3; i++ {
						body = append(body, 0x10, 31)
						if wide {
							body = append(body, 0x85, 0x69)
						} else {
							body = append(body, 0x68)
						}
						body = append(body, 0x19, byte(firstLocal+i))
						branch := len(body)
						body = append(body, 0xc6, 0, 0, 0x19, byte(firstLocal+i), 0xb6, byte(call>>8), byte(call))
						if wide {
							body = append(body, 0x61)
						} else {
							body = append(body, 0x60)
						}
						join := len(body)
						binary.BigEndian.PutUint16(body[branch+1:], uint16(join-branch))
						delta := join - previous - 1
						if delta >= 64 {
							t.Fatal("authored frame extent")
						}
						frames = append(frames, byte(64+delta), wordTag)
						previous = join
					}
					body = append(body, 0xb8, byte(finish>>8), byte(finish))
					if wide {
						body = append(body, 0xad)
					} else {
						body = append(body, 0xac)
					}
					pool.AddUtf8Info("StackMapTable")
					code.Code = body
					code.MaxStack = 4
					code.MaxLocals = uint16(firstLocal + 3)
					code.Attributes = []AttributeInfo{&UnparsedAttribute{Name: "StackMapTable", Length: uint32(len(frames)), Info: frames}}
					code.AttrLen = uint32(12 + len(body) + 6 + len(frames))
					original := obj.Bytes()
					if err := os.WriteFile(filepath.Join(dir, "CarryOwner.class"), original, 0600); err != nil {
						t.Fatal(err)
					}
					const want = "192:carried:primitive:order:overflow:exception\n"
					run := func(cp string) {
						t.Helper()
						out, err := exec.Command(java, "-Xverify:all", "-cp", cp, "CarryDriver").CombinedOutput()
						if err != nil || string(out) != want {
							t.Fatal("original-first independent oracle", err, string(out))
						}
					}
					run(dir)
					resolve := resolverFromClasses(classMapFromDir(t, dir))
					for _, mode := range []DecompileMode{Precision, Compatibility} {
						t.Run(string(mode), func(t *testing.T) {
							result, err := DecompileWithOptions(original, DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve, EnvSnapshot: map[string]string{}})
							if err != nil || len(result.StubMethods) != 0 || strings.Contains(result.Source, DecompileStubMarker) {
								t.Fatal("complete source", err, result.Diagnostics)
							}
							out := t.TempDir()
							src := filepath.Join(out, "CarryOwner.java")
							if err := os.WriteFile(src, []byte(result.Source), 0600); err != nil {
								t.Fatal(err)
							}
							if log, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:"+debug, "-cp", dir, "-d", out, src).CombinedOutput(); err != nil {
								t.Fatal("recompile", err, string(log), result.Source)
							}
							run(out + string(os.PathListSeparator) + dir)
						})
					}
				})
			}
		}
	}
}

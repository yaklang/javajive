package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Kotlin's nullable receiver lowering retains a DUP across IFNULL and joins a
// null result with an inner receiver-dependent ternary. Equal stack heights do
// not mean those two incoming values are the same value. Both verifier frames
// and an independently specified executable oracle guard this bytecode shape.
func TestAdversarialNullableStackJoinRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	const source = `class NullableJoinEffects{static String trace="";static final RuntimeException failure=new IllegalArgumentException("same");static final NullableJoinNode node=new NullableJoinNode();static Object lookup(int mode){trace+="L";if(mode==3)throw failure;if(mode==4)return new Object();return mode==0?null:node;}}
class NullableJoinNode{String left(){NullableJoinEffects.trace+="A";if(NullableJoinEffects.trace.startsWith("X"))throw NullableJoinEffects.failure;return "left";}String right(){NullableJoinEffects.trace+="B";if(NullableJoinEffects.trace.startsWith("X"))throw NullableJoinEffects.failure;return "right";}}
class NullableJoinOwner{static String select(int mode,boolean flag){NullableJoinNode n=(NullableJoinNode)NullableJoinEffects.lookup(mode);if(n==null)return null;int unused=0;return flag?n.left():n.right();}}
public class NullableJoinDriver{public static void main(String[]args){for(int mode=0;mode<=4;mode++)for(boolean flag:new boolean[]{false,true}){NullableJoinEffects.trace=mode==2?"X":"";try{System.out.println(mode+":"+flag+":"+NullableJoinOwner.select(mode,flag)+":"+NullableJoinEffects.trace);}catch(Throwable failure){System.out.println(mode+":"+flag+":"+(failure==NullableJoinEffects.failure)+":"+failure.getClass().getSimpleName()+":"+NullableJoinEffects.trace);}}}}`
	const oracle = "0:false:null:L\n0:true:null:L\n1:false:right:LB\n1:true:left:LA\n2:false:true:IllegalArgumentException:XLB\n2:true:true:IllegalArgumentException:XLA\n3:false:true:IllegalArgumentException:L\n3:true:true:IllegalArgumentException:L\n4:false:false:ClassCastException:L\n4:true:false:ClassCastException:L\n"
	for _, debug := range []string{"-g", "-g:none"} {
		t.Run(debug, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "NullableJoinDriver.java")
			if err := os.WriteFile(path, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", dir, path).CombinedOutput(); err != nil {
				t.Fatalf("original: %v\n%s", err, out)
			}
			file := filepath.Join(dir, "NullableJoinOwner.class")
			object, err := Parse(readClassBytes(t, dir, "NullableJoinOwner"))
			if err != nil {
				t.Fatal(err)
			}
			pool := NewConstantPoolWithConstant(&object.ConstantPool)
			node := pool.AddNewClassInfo("NullableJoinNode")
			str := pool.AddNewClassInfo("java/lang/String")
			pool.AddUtf8Info("StackMapTable")
			changed := false
			for _, method := range object.Methods {
				name, _ := object.getUtf8(method.NameIndex)
				if name != "select" {
					continue
				}
				for _, attr := range method.Attributes {
					code, ok := attr.(*CodeAttribute)
					if !ok {
						continue
					}
					old := code.Code
					if len(old) != 32 || old[0] != 26 || old[1] != 184 || old[4] != 192 || old[7] != 77 || old[8] != 44 || old[9] != 199 || old[21] != 182 || old[28] != 182 || old[31] != 176 || len(code.ExceptionTable) != 0 {
						t.Fatalf("fixture changed: %x", old)
					}
					// Original call and CHECKCAST constant-pool operands are retained.
					// PC7 DUP, PC8 IFNULL32; PC32 POP+NULL; PC34 ARETURN.
					// The inner ternary joins at PC29 then reaches the outer PC34.
					code.Code = append([]byte{}, old[:7]...)
					code.Code = append(code.Code, 89, 198, 0, 24, 77, 3, 62, 27, 153, 0, 10, 44, 182, old[22], old[23], 167, 0, 7, 44, 182, old[29], old[30], 167, 0, 5, 87, 1, 176)
					code.MaxStack, code.MaxLocals = 2, 4
					frames := []byte{0, 4}
					// Full frames include exact stack/local categories at both branch
					// arms and the common one-word return. No old PC debug tables survive.
					frames = append(frames, 255, 0, 25, 0, 4, 1, 1, 7, byte(node>>8), byte(node), 1, 0, 0)
					frames = append(frames, 255, 0, 3, 0, 4, 1, 1, 7, byte(node>>8), byte(node), 1, 0, 1, 7, byte(str>>8), byte(str))
					frames = append(frames, 255, 0, 2, 0, 2, 1, 1, 0, 1, 7, byte(node>>8), byte(node))
					frames = append(frames, 255, 0, 1, 0, 2, 1, 1, 0, 1, 7, byte(str>>8), byte(str))
					code.Attributes = []AttributeInfo{&UnparsedAttribute{Name: "StackMapTable", Length: uint32(len(frames)), Info: frames}}
					code.AttrLen = uint32(12 + len(code.Code) + 6 + len(frames))
					changed = true
				}
			}
			if !changed {
				t.Fatal("missing select Code")
			}
			if err := os.WriteFile(file, object.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			if got := t04RunJava(t, java, dir, "NullableJoinDriver"); got != oracle {
				t.Fatalf("verified bytecode oracle got %q want %q", got, oracle)
			}
			resolve := resolverFromClasses(classMapFromDir(t, dir))
			for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
				t.Run(string(mode), func(t *testing.T) {
					raw, _ := resolve("NullableJoinOwner")
					var result DecompileResult
					var err error
					if mode == "legacy" {
						result.Source, err = DecompileWithResolver(raw, resolve)
					} else {
						result, err = DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
					}
					if err != nil {
						t.Fatal(err)
					}
					t.Logf("%s/%s\n%s", debug, mode, result.Source)
					if len(result.StubMethods) != 0 {
						t.Fatalf("stubbed: %+v", result.Diagnostics)
					}
					rebuilt := t.TempDir()
					src := filepath.Join(rebuilt, "NullableJoinOwner.java")
					if err := os.WriteFile(src, []byte(result.Source), 0600); err != nil {
						t.Fatal(err)
					}
					if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", dir, "-d", rebuilt, src).CombinedOutput(); err != nil {
						t.Fatalf("rebuild: %v\n%s\n%s", err, out, result.Source)
					}
					if got := t04RunJava(t, java, rebuilt+string(os.PathListSeparator)+dir, "NullableJoinDriver"); got != oracle {
						t.Fatalf("roundtrip got %q want %q\n%s", got, oracle, result.Source)
					}
				})
			}
		})
	}
}

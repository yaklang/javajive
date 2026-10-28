package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Legal JVM code may return a retained old field word while writing its update
// to another field or receiver. Mutate only the original field operand or the
// two receiver loads; no verifier metadata, stack width or control flow changes.
// The untouched driver checks both physical destinations independently.
func TestAdversarialPostFieldWriteRetainsOriginalDestination(t *testing.T) {
	javac, java := t04Tools(t)
	for _, owner := range []string{"PostPair", "RetainedCounter"} {
		t.Run(owner, func(t *testing.T) {
			for _, compiler := range []string{"modern", "javac8"} {
				t.Run(compiler, func(t *testing.T) {
					for _, debug := range []string{"none", "source,lines,vars"} {
						t.Run(debug, func(t *testing.T) {
							for _, variant := range []string{"field", "static", "receiver"} {
								t.Run(variant, func(t *testing.T) {
									for _, update := range []string{"increment", "decrement"} {
										t.Run(update, func(t *testing.T) {
											field, method, call, setup, check := "int left,right;", "int step(){return left++;} int other(){return right;}", "x.step()", "x.left=n;x.right=91;", "x.left!=n||x.right!=n+1"
											if variant == "static" {
												field = "static int left,right;"
											}
											if variant == "receiver" {
												method = "static int step(PostPair x,PostPair y){return x.left++;}"
												call = "PostPair.step(x,y)"
												setup = "x.left=n;x.right=91;y.left=71;y.right=83;"
												check = "x.left!=n||x.right!=91||y.left!=n+1||y.right!=83"
											}
											source := "class PostPair{" + field + method + "} class PostPairDriver{public static void main(String[]args){int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){PostPair x=new PostPair(),y=new PostPair();" + setup + "int got=" + call + ";if(got!=n||" + check + ")throw new AssertionError(\"field destination \"+n+\":\"+got+\":\"+x.left+\":\"+x.right);rows++;}"
											if variant == "receiver" {
												source += `PostPair x=new PostPair(),y=new PostPair();x.left=19;y.left=37;try{PostPair.step(null,y);throw new AssertionError("null read");}catch(NullPointerException expected){}try{PostPair.step(x,null);throw new AssertionError("null write");}catch(NullPointerException expected){}if(x.left!=19||y.left!=37)throw new AssertionError("null write changed read receiver");`
											}
											source += "System.out.println(rows);}}"
											if update == "decrement" {
												source = strings.ReplaceAll(strings.ReplaceAll(source, "left++", "left--"), "n+1", "n-1")
											}
											source = strings.ReplaceAll(source, "PostPair", owner)
											sources := map[string]string{owner + ".java": source}
											var files map[string][]byte
											if compiler == "javac8" {
												files = fieldOwnerCompileJava8Sources(t, sources, debug)
											} else {
												files = nativeCompileSourceReleaseClasses(t, sources, debug, "8")
											}
											obj, err := Parse(files[owner+".class"])
											if err != nil {
												t.Fatal(err)
											}
											var methodCode, otherCode *CodeAttribute
											for _, m := range obj.Methods {
												name, _ := obj.getUtf8(m.NameIndex)
												for _, a := range m.Attributes {
													if c, ok := a.(*CodeAttribute); ok {
														if name == "step" {
															methodCode = c
														}
														if name == "other" {
															otherCode = c
														}
													}
												}
											}
											if methodCode == nil {
												t.Fatal("missing authored method")
											}
											if variant == "receiver" {
												if len(methodCode.Code) < 2 || methodCode.Code[0] != core.OP_ALOAD_0 || methodCode.Code[1] != core.OP_DUP {
													t.Fatal("unexpected receiver packet")
												}
												methodCode.Code[0] = core.OP_ALOAD_1
												methodCode.Code[1] = core.OP_ALOAD_0
											} else {
												if otherCode == nil {
													t.Fatal("missing other-field accessor")
												}
												decode := func(c *CodeAttribute) *core.Decompiler {
													d := core.NewDecompiler(c.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
													if err := d.ParseOpcode(); err != nil {
														t.Fatal(err)
													}
													return d
												}
												var target []byte
												for _, op := range constructorMotionOps(decode(otherCode)) {
													if op.Instr.OpCode == core.OP_GETFIELD || op.Instr.OpCode == core.OP_GETSTATIC {
														target = op.Data
													}
												}
												if len(target) != 2 {
													t.Fatal("missing field operand")
												}
												changed := 0
												for _, op := range constructorMotionOps(decode(methodCode)) {
													if op.Instr.OpCode == core.OP_PUTFIELD || op.Instr.OpCode == core.OP_PUTSTATIC {
														copy(methodCode.Code[int(op.CurrentOffset)+1:], target)
														changed++
													}
												}
												if changed != 1 {
													t.Fatal("expected one original store")
												}
											}
											files[owner+".class"] = obj.Bytes()
											original := t.TempDir()
											for n, b := range files {
												if err := os.WriteFile(filepath.Join(original, n), b, 0600); err != nil {
													t.Fatal(err)
												}
											}
											if got := t04RunJava(t, java, original, owner+"Driver"); got != "5\n" {
												t.Fatal("original independent oracle", got)
											}
											resolve := func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }
											for _, mode := range []DecompileMode{"archive", Precision, Compatibility} {
												t.Run(string(mode), func(t *testing.T) {
													var generated string
													if mode == "archive" {
														z := nativeArchive(t, files)
														defer z.Close()
														z.targetSourceVersion = 8
														b, err := z.ReadFile(owner + ".class")
														if err != nil {
															t.Fatal(err)
														}
														generated = string(b)
													} else {
														r, err := DecompileWithOptions(files[owner+".class"], DecompileOptions{Mode: mode, Resolve: resolve, TargetSourceVersion: 8})
														if err != nil {
															t.Fatal(err)
														}
														generated = r.Source
													}
													if strings.Contains(generated, DecompileStubMarker) {
														t.Fatal("stub", generated)
													}
													out := t.TempDir()
													if err := os.WriteFile(filepath.Join(out, owner+"Driver.class"), files[owner+"Driver.class"], 0600); err != nil {
														t.Fatal(err)
													}
													path := filepath.Join(out, owner+".java")
													if err := os.WriteFile(path, []byte(generated), 0600); err != nil {
														t.Fatal(err)
													}
													if data, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", out, "-d", out, path).CombinedOutput(); err != nil {
														t.Fatalf("compile %v\n%s\n%s", err, data, generated)
													}
													t.Cleanup(func() {
														if t.Failed() {
															t.Log("reconstructed owner:\n", generated)
														}
													})
													if got := t04RunJava(t, java, out, owner+"Driver"); got != "5\n" {
														t.Fatal(got)
													}
												})
											}
										})
									}
								})
							}
						})
					}
				})
			}
		})
	}
}

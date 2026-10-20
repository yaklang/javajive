package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func largeSourceComponentFixture(size int) string {
	var s strings.Builder
	s.WriteString("abstract class LargeParent{final int seen;LargeParent(){seen=read();}abstract int read();}\n")
	for i := 0; i < size; i++ {
		next := (i + 1) % size
		fmt.Fprintf(&s, "class LargeFamily%d{private final int value;LargeFamily%d(int value){this.value=value;}static class Token{final int value;Token(int value){this.value=value;}}final class Leaf extends LargeParent{final LargeFamily%d.Token peer;Leaf(LargeFamily%d.Token peer){this.peer=peer;}int read(){return LargeFamily%d.this.value;}int sum(){return read()+peer.value;}}Leaf make(LargeFamily%d.Token peer){return new Leaf(peer);}}\n", i, i, next, next, i, next)
	}
	s.WriteString(strings.ReplaceAll(`class LargeDriver{public static void main(String[]args)throws Exception{int rows=0;for(int value:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int i=0;i<COMPONENT_SIZE;i++){int own=value+i,foreign=~value-i;Class<?> rootType=Class.forName("LargeFamily"+i),tokenType=Class.forName("LargeFamily"+((i+1)%COMPONENT_SIZE)+"$Token");java.lang.reflect.Constructor<?> rootCtor=rootType.getDeclaredConstructor(int.class),tokenCtor=tokenType.getDeclaredConstructor(int.class);rootCtor.setAccessible(true);tokenCtor.setAccessible(true);Object root=rootCtor.newInstance(own),peer=tokenCtor.newInstance(foreign);java.lang.reflect.Method make=rootType.getDeclaredMethod("make",tokenType);make.setAccessible(true);Object leaf=make.invoke(root,peer);Class<?> childType=leaf.getClass();java.lang.reflect.Method read=childType.getDeclaredMethod("read"),sum=childType.getDeclaredMethod("sum");read.setAccessible(true);sum.setAccessible(true);java.lang.reflect.Field field=childType.getDeclaredField("peer");field.setAccessible(true);int expected=java.math.BigInteger.valueOf(own).add(java.math.BigInteger.valueOf(foreign)).intValue();if(((LargeParent)leaf).seen!=own||((Integer)read.invoke(leaf)).intValue()!=own||((Integer)sum.invoke(leaf)).intValue()!=expected||field.get(leaf)!=peer||childType.getDeclaringClass()!=rootType)throw new AssertionError("large component binding/early capture");rows++;}if(rows!=5*COMPONENT_SIZE)throw new AssertionError(rows);System.out.println(rows+":large:source:component");}}`, "COMPONENT_SIZE", fmt.Sprint(size)))
	return s.String()
}

func TestAdversarialSourceComponentCapacity64(t *testing.T) { testLargeSourceComponent(t, 64) }
func TestAdversarialSourceComponentCapacity65(t *testing.T) { testLargeSourceComponent(t, 65) }
func TestAdversarialSourceComponentCapacity96(t *testing.T) { testLargeSourceComponent(t, 96) }
func testLargeSourceComponent(t *testing.T, size int) {
	compiler, java := t04Tools(t)
	t.Run(fmt.Sprintf("families%d", size), func(t *testing.T) {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(debug, func(t *testing.T) {
				files := nativeCompileSourceReleaseClasses(t, map[string]string{"LargeFamily0.java": largeSourceComponentFixture(size)}, debug, "8")
				original := t.TempDir()
				for name, raw := range files {
					if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
				want := t04RunJava(t, java, original, "LargeDriver")
				if expected := fmt.Sprintf("%d:large:source:component\n", 5*size); want != expected {
					t.Fatal("original mathematical/identity oracle", want, expected)
				}
				for _, reverse := range []bool{false, true} {
					label := "forward"
					if reverse {
						label = "reverse"
					}
					t.Run(label, func(t *testing.T) {
						z := nativeArchive(t, files)
						defer z.Close()
						out := t.TempDir()
						for _, name := range []string{"LargeParent.class", "LargeDriver.class"} {
							if err := os.WriteFile(filepath.Join(out, name), files[name], 0600); err != nil {
								t.Fatal(err)
							}
						}
						args := []string{"-proc:none", "--release", "8", "-g:" + debug, "-cp", out, "-d", out}
						for index := 0; index < size; index++ {
							i := index
							if reverse {
								i = size - 1 - index
							}
							name := fmt.Sprintf("LargeFamily%d", i)
							raw, err := z.ReadFile(name + ".class")
							if err != nil || strings.Contains(string(raw), DecompileStubMarker) || !strings.Contains(string(raw), "class Leaf") || !strings.Contains(string(raw), "class Token") {
								t.Fatalf("incomplete source component at %s: %v\n%s", name, err, raw)
							}
							path := filepath.Join(out, name+".java")
							if err := os.WriteFile(path, raw, 0600); err != nil {
								t.Fatal(err)
							}
							args = append(args, path)
						}
						if log, err := exec.Command(compiler, args...).CombinedOutput(); err != nil {
							t.Fatalf("compile every source family without original fallback: %v\n%s", err, log)
						}
						if got := t04RunJava(t, java, out, "LargeDriver"); got != want {
							t.Fatalf("same binary original callers: %q want %q", got, want)
						}
						for name, before := range files {
							after, err := os.ReadFile(filepath.Join(out, name))
							if err != nil {
								t.Fatal(err)
							}
							if name == "LargeParent.class" || name == "LargeDriver.class" {
								if string(before) != string(after) {
									t.Fatal("original caller/support changed", name)
								}
							} else if a, b := nativeBinaryShape(t, before), nativeBinaryShape(t, after); a != b {
								t.Fatalf("physical ABI changed %s\n%s\n%s", name, a, b)
							} else if a, b := nativeAnonymousAccessorShape(t, before), nativeAnonymousAccessorShape(t, after); a != b {
								t.Fatalf("original lexical ownership changed %s\n%s\n%s", name, a, b)
							}
						}
						if len(z.nativeMemberTransactions) != 1 {
							t.Fatal("component did not complete once", len(z.nativeMemberTransactions))
						}
					})
				}
			})
		}
	})
}

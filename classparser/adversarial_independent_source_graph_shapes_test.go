package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Each edge is an original declaration/binding dependency, not ownership.
// The retained binary callers exercise all rebuilt roots/children together;
// private access still goes through the original compiler's accessor protocol.
func independentSourceGraphShapeFixture(edges [][]int, label string) (string, []string) {
	var source strings.Builder
	source.WriteString("import java.util.function.IntUnaryOperator;abstract class GraphParent{final int seen;GraphParent(){seen=read();}abstract int read();}\n")
	roots := make([]string, len(edges))
	for i, targets := range edges {
		outer := fmt.Sprintf("GraphOuter%d", i)
		roots[i] = outer + "$Capsule"
		fmt.Fprintf(&source, "class %s{static class Capsule{private final int value;Capsule(int value){this.value=value;}int get(){return value;}", outer)
		for _, target := range targets {
			fmt.Fprintf(&source, "GraphOuter%d.Capsule peer%d;", target, target)
		}
		source.WriteString("final class Worker extends GraphParent{final int bias;Worker(int bias){this.bias=bias;}int read(){return Capsule.this.value;}int sum(){return bias+Capsule.this.value")
		for _, target := range targets {
			fmt.Fprintf(&source, "+peer%d.get()", target)
		}
		source.WriteString(";}}Worker make(int bias){return new Worker(bias);}}static IntUnaryOperator unrelated(final String text){return value->value+text.length();}}\n")
	}
	source.WriteString("class GraphDriver{public static void main(String[]args){int rows=0;for(int left:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int right:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){")
	for i := range edges {
		fmt.Fprintf(&source, "int value%d=left+%d;GraphOuter%d.Capsule c%d=new GraphOuter%d.Capsule(value%d);", i, (i+1)*17, i, i, i, i)
	}
	for i, targets := range edges {
		for _, target := range targets {
			fmt.Fprintf(&source, "c%d.peer%d=c%d;", i, target, target)
		}
	}
	for i, targets := range edges {
		fmt.Fprintf(&source, "GraphOuter%d.Capsule.Worker w%d=c%d.make(right);java.math.BigInteger expected%d=java.math.BigInteger.valueOf(right).add(java.math.BigInteger.valueOf(value%d))", i, i, i, i, i)
		for _, target := range targets {
			fmt.Fprintf(&source, ".add(java.math.BigInteger.valueOf(value%d))", target)
		}
		fmt.Fprintf(&source, ";if(w%d.seen!=value%d||w%d.sum()!=expected%d.intValue()||w%d.getClass().getDeclaringClass()!=c%d.getClass()||GraphOuter%d.unrelated(\"xyz\").applyAsInt(4)!=7)throw new AssertionError(\"graph binding or early callback\");rows++;", i, i, i, i, i, i, i)
	}
	fmt.Fprintf(&source, "}if(rows!=%d)throw new AssertionError(rows);System.out.println(\"%d:graph:%s\");}}", 25*len(edges), 25*len(edges), label)
	return source.String(), roots
}

func TestAdversarialIndependentSourceGraphShapesKeepOriginalCallers(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Skip("original native8 compiler")
	}
	cases := []struct {
		name  string
		edges [][]int
	}{
		{"ring3", [][]int{{1}, {2}, {0}}},
		{"chain3", [][]int{{1}, {2}, {}}},
		{"diamond4", [][]int{{1, 2}, {3}, {3}, {}}},
		{"downstreamCycle", [][]int{{1}, {2}, {1}}},
	}
	for _, shape := range cases {
		t.Run(shape.name, func(t *testing.T) {
			source, roots := independentSourceGraphShapeFixture(shape.edges, shape.name)
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, source, "GraphOuter0", debug, true)
					original := t.TempDir()
					for name, raw := range files {
						if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					want := t04RunJava(t, java, original, "GraphDriver")
					if expected := fmt.Sprintf("%d:graph:%s\n", 25*len(roots), shape.name); want != expected {
						t.Fatalf("original oracle %q expected %q", want, expected)
					}
					for _, reverse := range []bool{false, true} {
						label := "forward"
						if reverse {
							label = "reverse"
						}
						t.Run(label, func(t *testing.T) {
							z := nativeArchive(t, files)
							defer z.Close()
							order := append([]string(nil), roots...)
							if reverse {
								for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
									order[i], order[j] = order[j], order[i]
								}
							}
							generated := map[string][]byte{}
							for _, root := range order {
								raw, err := z.ReadFile(root + ".class")
								if err != nil || strings.Contains(string(raw), DecompileStubMarker) || !strings.Contains(string(raw), "class Worker") {
									t.Fatalf("complete graph scope %s: %v\n%s", root, err, raw)
								}
								generated[root] = raw
							}
							owned := map[string][]byte{}
							out := t.TempDir()
							for name, raw := range files {
								isOwned := false
								for _, root := range roots {
									isOwned = isOwned || name == root+".class" || strings.HasPrefix(name, root+"$")
								}
								if isOwned {
									owned[name] = raw
									continue
								}
								if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
									t.Fatal(err)
								}
							}
							if len(owned) != 2*len(roots) {
								t.Fatal("original owned graph class inventory", len(owned))
							}
							args := []string{"-proc:none", "--release", "8", "-parameters", "-g:" + debug, "-cp", out, "-d", out}
							for _, root := range roots {
								path := filepath.Join(out, root+".java")
								if err := os.WriteFile(path, generated[root], 0600); err != nil {
									t.Fatal(err)
								}
								args = append(args, path)
							}
							if log, err := exec.Command(modern, args...).CombinedOutput(); err != nil {
								t.Fatalf("compile whole graph without original class fallback: %v\n%s\n%s", err, log, generated)
							}
							if got := t04RunJava(t, java, out, "GraphDriver"); got != want {
								t.Fatalf("unchanged original callers %q wanted %q", got, want)
							}
							for name, before := range owned {
								after, err := os.ReadFile(filepath.Join(out, name))
								if err != nil {
									t.Fatal(err)
								}
								outer := strings.SplitN(name, "$", 2)[0]
								nativeStaticIndependentRootRepresentationShape(outer)(t, name, before, after)
							}
							for name, before := range files {
								if owned[name] != nil {
									continue
								}
								after, err := os.ReadFile(filepath.Join(out, name))
								if err != nil || string(after) != string(before) {
									t.Fatal("unowned original class changed", name, err)
								}
							}
						})
					}
				})
			}
		})
	}
}

package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The older consumer ABI references an independently compiled newer producer.
// Both are valid input; declining the producer's native source protocol must
// not erase an unrelated consumer's original lexical declaration ownership.
func flatStaticDeclarationSources(kind string) map[string]string {
	if kind == "protected enum" {
		return map[string]string{
			"p/ForeignOwner.java": `package p;public class ForeignOwner{protected enum Choice{A,B}public static Object[] choices(){return Choice.values();}public static Object first(){return Choice.A;}public static int ordinal(Object value){return ((Choice)value).ordinal();}}`,
			"q/Consumer.java":     `package q;import p.ForeignOwner;public class Consumer{public static class Member extends ForeignOwner{final Choice value;Member(Object value){this.value=(Choice)value;}Object read(){return value;}static Object selected(){return Choice.A;}}public static Object first(){return Member.selected();}public static Object make(Object value){return new Member(value);}public static Object read(Object value){return ((Member)value).read();}}`,
			"q/Driver.java":       `package q;import p.ForeignOwner;public class Driver{public static void main(String[]args){int rows=0;for(Object value:ForeignOwner.choices()){Object object=Consumer.make(value);if(Consumer.first()!=ForeignOwner.first()||Consumer.read(object)!=value||object.getClass().getDeclaringClass()!=Consumer.class||ForeignOwner.ordinal(value)!=rows)throw new AssertionError("protected original identity / ordinal / declaration owner");rows++;}Object object=Consumer.make(null);if(Consumer.read(object)!=null||object.getClass().getDeclaringClass()!=Consumer.class)throw new AssertionError("null/declaration owner");System.out.println(rows+":static-declaration:identity:owner:null");}}`,
		}
	}
	declaration := `public enum Choice{A,B}`
	if kind == "package enum" {
		declaration = `static enum Choice{A,B}`
	}
	if kind == "interface" {
		declaration = `public interface Choice{Choice A=new Implementation(1),B=new Implementation(2);int ordinal();static Choice[] values(){return new Choice[]{A,B};}}public static final class Implementation implements Choice{private final int value;Implementation(int value){this.value=value;}public int ordinal(){return value-1;}}`
	}
	if kind == "class" {
		declaration = `public static final class Choice{public static final Choice A=new Choice(1),B=new Choice(2);private final int value;private Choice(int value){this.value=value;}public int ordinal(){return value-1;}public static Choice[] values(){return new Choice[]{A,B};}}`
	}
	sources := map[string]string{
		"p/ForeignOwner.java": `package p;public class ForeignOwner{` + declaration + `}`,
		"q/Consumer.java":     `package q;import p.ForeignOwner;public class Consumer{static int p=7;public static ForeignOwner.Choice first(){return ForeignOwner.Choice.A;}public static class Member{final ForeignOwner.Choice value;Member(ForeignOwner.Choice value){this.value=value;}ForeignOwner.Choice read(){return value;}}public static Object make(ForeignOwner.Choice value){return new Member(value);}public static ForeignOwner.Choice read(Object value){return ((Member)value).read();}}`,
		"q/Driver.java":       `package q;import p.ForeignOwner;public class Driver{public static void main(String[]args){int rows=0;for(ForeignOwner.Choice value:ForeignOwner.Choice.values()){Object object=Consumer.make(value);if(Consumer.first()!=ForeignOwner.Choice.A||Consumer.read(object)!=value||object.getClass().getDeclaringClass()!=Consumer.class||value.ordinal()!=rows)throw new AssertionError("original identity / ordinal / declaration owner");rows++;}Object object=Consumer.make(null);if(Consumer.read(object)!=null||object.getClass().getDeclaringClass()!=Consumer.class)throw new AssertionError("null/declaration owner");System.out.println(rows+":static-declaration:identity:owner:null");}}`,
	}
	if kind == "package enum" {
		next := map[string]string{}
		for name, source := range sources {
			next[strings.Replace(name, "q/", "p/", 1)] = strings.Replace(source, "package q;", "package p;", 1)
		}
		sources = next
	}
	return sources
}

func compileFlatStaticDeclarationSources(t *testing.T, sources map[string]string, debug, foreignRelease string, foreignSources ...string) map[string][]byte {
	t.Helper()
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	var foreign, consumers []string
	if len(foreignSources) == 0 {
		foreignSources = []string{"p/ForeignOwner.java"}
	}
	producerInputs := map[string]bool{}
	for _, name := range foreignSources {
		producerInputs[name] = true
	}
	for name, source := range sources {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		if producerInputs[name] {
			foreign = append(foreign, path)
		} else {
			consumers = append(consumers, path)
		}
	}
	for _, batch := range []struct {
		release string
		paths   []string
	}{{foreignRelease, foreign}, {"8", consumers}} {
		sort.Strings(batch.paths)
		args := []string{"-proc:none", "--release", batch.release, "-g:" + debug, "-cp", dir, "-d", dir}
		if out, err := exec.Command(javac, append(args, batch.paths...)...).CombinedOutput(); err != nil {
			t.Fatalf("original compile: %v\n%s", err, out)
		}
	}
	files := map[string][]byte{}
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".class") {
			return nil
		}
		name, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err == nil {
			files[filepath.ToSlash(name)] = raw
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return files
}

func TestNativeFlatStaticEnumDeclarationRetainsConsumerOwnership(t *testing.T) {
	flatStaticDeclarationRoundTrip(t, "enum")
}
func TestNativeFlatStaticClassDeclarationRetainsConsumerOwnership(t *testing.T) {
	flatStaticDeclarationRoundTrip(t, "class")
}
func TestNativeFlatStaticProtectedEnumDeclarationRetainsConsumerOwnership(t *testing.T) {
	flatStaticDeclarationRoundTrip(t, "protected enum")
}
func TestNativeFlatStaticPackageEnumDeclarationRetainsConsumerOwnership(t *testing.T) {
	flatStaticDeclarationRoundTrip(t, "package enum")
}
func TestNativeFlatStaticInterfaceDeclarationRetainsConsumerOwnership(t *testing.T) {
	flatStaticDeclarationRoundTrip(t, "interface")
}
func flatStaticDeclarationRoundTrip(t *testing.T, kind string) {
	t.Helper()
	javac, java := t04Tools(t)
	consumerPackage := "q"
	if kind == "package enum" {
		consumerPackage = "p"
	}
	for _, variant := range []string{"original", "renamed", "unicode"} {
		t.Run(variant, func(t *testing.T) {
			sources := flatStaticDeclarationSources(kind)
			consumer, producer, choice := "Consumer", "ForeignOwner", "Choice"
			if variant != "original" {
				consumer, producer, choice = "Reader", "Atlas", "Mode"
				member := "Node"
				if variant == "unicode" {
					consumer, producer, choice, member = "消费者", "外部", "取值", "条目"
				}
				next := map[string]string{}
				replace := strings.NewReplacer("ForeignOwner", producer, "Consumer", consumer, "Member", member, "Choice", choice)
				for name, source := range sources {
					next[replace.Replace(name)] = replace.Replace(source)
				}
				sources = next
			}
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					files := compileFlatStaticDeclarationSources(t, sources, debug, "11", "p/"+producer+".java")
					write := func(root, name string, raw []byte) {
						t.Helper()
						path := filepath.Join(root, filepath.FromSlash(name))
						if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(path, raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					original := t.TempDir()
					for name, raw := range files {
						write(original, name, raw)
					}
					const want = "2:static-declaration:identity:owner:null\n"
					if got := t04RunJava(t, java, original, consumerPackage+".Driver"); got != want {
						t.Fatalf("original oracle=%q", got)
					}
					for _, order := range []string{"forward", "reverse"} {
						t.Run(order, func(t *testing.T) {
							archive := nativeArchive(t, files)
							defer archive.Close()
							archive.targetSourceVersion = 8
							archive.sourceCompiler = ModernJavac
							out := t.TempDir()
							var names, paths []string
							for name := range files {
								names = append(names, name)
							}
							sort.Strings(names)
							if order == "reverse" {
								sort.Sort(sort.Reverse(sort.StringSlice(names)))
							}
							for _, name := range names {
								if name == consumerPackage+"/Driver.class" {
									write(out, name, files[name])
									continue
								}
								source, err := archive.ReadFile(name)
								if err != nil || strings.Contains(string(source), DecompileStubMarker) {
									t.Fatalf("requested source %s: %v\n%s", name, err, source)
								}
								path := strings.TrimSuffix(name, ".class") + ".java"
								write(out, path, source)
								paths = append(paths, filepath.Join(out, filepath.FromSlash(path)))
							}
							args := []string{"-proc:none", "--release", "8", "-cp", out, "-d", out}
							if raw, err := exec.Command(javac, append(args, paths...)...).CombinedOutput(); err != nil {
								for _, path := range paths {
									if source, readErr := os.ReadFile(path); readErr == nil {
										t.Logf("rebuilt source %s\n%s", path, source)
									}
								}
								t.Fatalf("rebuilt source: %v\n%s", err, raw)
							}
							if got := t04RunJava(t, java, out, consumerPackage+".Driver"); got != want {
								t.Fatalf("rebuilt original driver=%q", got)
							}
							root, err := Parse(files[consumerPackage+"/"+consumer+".class"])
							if err != nil {
								t.Fatal(err)
							}
							entry := archive.nativeMemberEntry(root)
							if entry == nil || entry.family == nil {
								t.Fatal("consumer's original lexical family was not reconstructed")
							}
							for name, raw := range files {
								if name != consumerPackage+"/"+consumer+".class" && !strings.HasPrefix(name, consumerPackage+"/"+consumer+"$") {
									continue
								}
								rebuilt, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(name)))
								if err != nil {
									t.Fatal(err)
								}
								if a, b := nativeBinaryShape(t, raw), nativeBinaryShape(t, rebuilt); a != b {
									t.Fatalf("consumer ABI changed %s\n%s\n%s", name, a, b)
								}
								nativeLexicalExactSignatures(t, name, raw, rebuilt)
							}
							for name := range files {
								if !strings.HasPrefix(name, "p/"+producer+"$") {
									continue
								}
								binary := strings.TrimSuffix(name, ".class")
								spelling := entry.family.sourceDependencies[binary]
								fullName := strings.ReplaceAll(binary, "/", ".")
								needsSpelling := binary == "p/"+producer+"$"+choice
								if needsSpelling && spelling != fullName || spelling != "" && spelling != fullName || entry.family.children[binary] != nil || entry.family.lexicalObjects[binary] != nil || entry.family.allocationDependencies[binary] != nil {
									t.Fatal("declaration-only spelling lost or foreign ownership imported")
								}
							}
						})
					}
				})
			}
		})
	}
}

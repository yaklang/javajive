package javaclassparser

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Original and rebuilt programs use the same compiler flags. -parameters is
// explicit because ordinary method finality otherwise has no parameter table.
// Every JVM program here is authored source or an audited local-word mutation
// of that source, and runs before any JavaJive source is requested.
func testFinalParameterDefinition(t *testing.T, source, owner, driver, want string, mutate func(*testing.T, *ClassObject), refuse bool) {
	t.Helper()
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(policy, func(t *testing.T) {
					if policy == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if policy == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					javac, java := t04Tools(t)
					original := t.TempDir()
					input := filepath.Join(original, "Authored.java")
					if err := os.WriteFile(input, []byte(source), 0600); err != nil {
						t.Fatal(err)
					}
					if raw, err := exec.Command(javac, "-proc:none", "--release", "8", "-parameters", "-g:"+debug, "-d", original, input).CombinedOutput(); err != nil {
						t.Fatalf("original compile:%v\n%s", err, raw)
					}
					files := map[string][]byte{}
					if err := filepath.Walk(original, func(path string, info os.FileInfo, err error) error {
						if err != nil || info.IsDir() || !strings.HasSuffix(path, ".class") {
							return err
						}
						rel, err := filepath.Rel(original, path)
						if err != nil {
							return err
						}
						raw, err := os.ReadFile(path)
						files[filepath.ToSlash(rel)] = raw
						return err
					}); err != nil {
						t.Fatal(err)
					}
					if mutate != nil {
						obj, err := Parse(files[owner+".class"])
						if err != nil {
							t.Fatal(err)
						}
						mutate(t, obj)
						files[owner+".class"] = obj.Bytes()
						if err := os.WriteFile(filepath.Join(original, owner+".class"), files[owner+".class"], 0600); err != nil {
							t.Fatal(err)
						}
					}
					if got := t04RunJava(t, java, original, driver); got != want {
						t.Fatalf("original oracle=%q want=%q", got, want)
					}
					z := nativeArchive(t, files)
					defer z.Close()
					rebuilt := t.TempDir()
					var paths []string
					refused := false
					var sources []string
					for name, raw := range files {
						path := filepath.Join(rebuilt, filepath.FromSlash(name))
						if name == owner+".class" || strings.HasPrefix(name, owner+"$") {
							text, err := z.ReadFile(name)
							if err != nil || strings.Contains(string(text), DecompileStubMarker) || strings.Contains(string(text), "// decompile dump failed") {
								refused = true
								if !refuse {
									t.Fatalf("source %s:%v\n%s", name, err, text)
								}
							}
							path = strings.TrimSuffix(path, ".class") + ".java"
							raw = text
							paths = append(paths, path)
							sources = append(sources, string(text))
						}
						if err := os.WriteFile(path, raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					if refuse {
						if !refused {
							t.Fatalf("unproved parameter definition accepted\n%s", strings.Join(sources, "\n"))
						}
						return
					}
					args := append([]string{"-proc:none", "--release", "8", "-parameters", "-cp", rebuilt, "-d", rebuilt}, paths...)
					if raw, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
						t.Fatalf("rebuilt compile:%v\n%s\n%s", err, raw, strings.Join(sources, "\n"))
					}
					if got := t04RunJava(t, java, rebuilt, driver); got != want {
						t.Fatalf("rebuilt oracle=%q want=%q", got, want)
					}
					for name, raw := range files {
						if name != owner+".class" && !strings.HasPrefix(name, owner+"$") {
							continue
						}
						next, err := os.ReadFile(filepath.Join(rebuilt, name))
						if err != nil {
							t.Fatal(err)
						}
						if a, b := finalParameterFlags(t, raw), finalParameterFlags(t, next); !reflect.DeepEqual(a, b) {
							t.Fatalf("original parameter flags changed %s: %v -> %v", name, a, b)
						}
						nativeLexicalExactSignatures(t, name, raw, next)
					}
				})
			}
		})
	}
}

// Read every original flag word independently of the production finality map.
func finalParameterFlags(t *testing.T, raw []byte) map[string][]uint16 {
	t.Helper()
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	flags := map[string][]uint16{}
	for _, method := range obj.Methods {
		name, _ := sourceBridgeUTF8(obj, method.NameIndex)
		descriptor, _ := sourceBridgeUTF8(obj, method.DescriptorIndex)
		for _, attribute := range method.Attributes {
			if table, ok := attribute.(*UnparsedAttribute); ok && table.Name == "MethodParameters" {
				if len(table.Info) < 1 || len(table.Info) != 1+4*int(table.Info[0]) {
					t.Fatal("bad independent parameter table")
				}
				for i := 0; i < int(table.Info[0]); i++ {
					flags[name+descriptor] = append(flags[name+descriptor], binary.BigEndian.Uint16(table.Info[3+4*i:5+4*i]))
				}
			}
		}
	}
	return flags
}

func finalParameterReadCode(t *testing.T, obj *ClassObject) *CodeAttribute {
	t.Helper()
	for _, method := range obj.Methods {
		name, _ := sourceBridgeUTF8(obj, method.NameIndex)
		if name == "read" {
			for _, attribute := range method.Attributes {
				if code, ok := attribute.(*CodeAttribute); ok {
					return code
				}
			}
		}
	}
	t.Fatal("missing authored read Code")
	return nil
}

func finalParameterMarkReadFinal(t *testing.T, obj *ClassObject, index int) {
	t.Helper()
	for _, method := range obj.Methods {
		name, _ := sourceBridgeUTF8(obj, method.NameIndex)
		if name != "read" {
			continue
		}
		for _, attribute := range method.Attributes {
			if table, ok := attribute.(*UnparsedAttribute); ok && table.Name == "MethodParameters" && len(table.Info) >= 5+4*index {
				entry := table.Info[3+4*index : 5+4*index]
				binary.BigEndian.PutUint16(entry, binary.BigEndian.Uint16(entry)|0x10)
				return
			}
		}
	}
	t.Fatal("missing original parameter flag word")
}

func TestAdversarialFinalParameterDefinitionConstantStore(t *testing.T) {
	const source = `class ConstantFinalOwner{static class Anchor{}static int read(final int input){int local=7;return local;}}
class ConstantFinalDriver{public static void main(String[]args){for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})if(ConstantFinalOwner.read(n)!=7)throw new AssertionError("constant later local definition");System.out.println("constant:entry:storage");}}`
	testFinalParameterDefinition(t, source, "ConstantFinalOwner", "ConstantFinalDriver", "constant:entry:storage\n", func(t *testing.T, obj *ClassObject) {
		code := finalParameterReadCode(t, obj)
		code.Code = []byte{0x10, 7, 0x3b, 0x1a, 0xac}
	}, false)
}

func TestAdversarialFinalParameterDefinitionAbstractHeader(t *testing.T) {
	const source = `abstract class AbstractFinalOwner{static class Anchor{}abstract int read(final long wide,int ordinary,final Object token);}
class AbstractFinalDriver{public static void main(String[]args)throws Exception{java.lang.reflect.Parameter[] p=AbstractFinalOwner.class.getDeclaredMethod("read",long.class,int.class,Object.class).getParameters();if(p[0].getModifiers()!=16||p[1].getModifiers()!=0||p[2].getModifiers()!=16)throw new AssertionError("abstract descriptor positions and finality");System.out.println("abstract:entry:flags");}}`
	testFinalParameterDefinition(t, source, "AbstractFinalOwner", "AbstractFinalDriver", "abstract:entry:flags\n", nil, false)
}

func TestAdversarialFinalParameterDefinitionIINC(t *testing.T) {
	const source = `class IncrementFinalOwner{static class Anchor{}static int read(int input){input++;return input;}}
class IncrementFinalDriver{public static void main(String[]args){for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})if(IncrementFinalOwner.read(n)!=n+1)throw new AssertionError("original IINC/entry value");System.out.println("iinc:entry:storage");}}`
	testFinalParameterDefinition(t, source, "IncrementFinalOwner", "IncrementFinalDriver", "iinc:entry:storage\n", func(t *testing.T, obj *ClassObject) {
		finalParameterMarkReadFinal(t, obj, 0)
	}, false)
}

func TestAdversarialFinalParameterDefinitionWideStore(t *testing.T) {
	const source = `class WideFinalOwner{static class Anchor{}static long read(final long input){return input;}}
class WideFinalDriver{public static void main(String[]args){for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})if(WideFinalOwner.read(n)!=n+1)throw new AssertionError("original wide entry/store");System.out.println("wide:entry:storage");}}`
	testFinalParameterDefinition(t, source, "WideFinalOwner", "WideFinalDriver", "wide:entry:storage\n", func(t *testing.T, obj *ClassObject) {
		code := finalParameterReadCode(t, obj)
		code.AttrLen += 4
		code.MaxStack = 4
		code.Code = []byte{0x1e, 0x0a, 0x61, 0x3f, 0x1e, 0xad}
	}, false)
}

func TestAdversarialFinalParameterDefinitionHandlerDomain(t *testing.T) {
	const source = `class HandlerFinalOwner{static class Anchor{}static int read(int input,int divisor){try{input++;return input/divisor;}catch(ArithmeticException expected){return input;}}}
class HandlerFinalDriver{public static void main(String[]args){for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int d:new int[]{-1,0,1}){int value=n+1;if(HandlerFinalOwner.read(n,d)!=(d==0?value:value/d))throw new AssertionError("entry update and original handler domain");}System.out.println("handler:entry:storage");}}`
	testFinalParameterDefinition(t, source, "HandlerFinalOwner", "HandlerFinalDriver", "handler:entry:storage\n", func(t *testing.T, obj *ClassObject) {
		finalParameterMarkReadFinal(t, obj, 0)
	}, false)
}

func TestAdversarialFinalParameterDefinitionAnnotationsAndInstanceWideIndex(t *testing.T) {
	const source = `@java.lang.annotation.Retention(java.lang.annotation.RetentionPolicy.RUNTIME) @java.lang.annotation.Target(java.lang.annotation.ElementType.PARAMETER) @interface FinalWordMark{}
class MixedFinalOwner{static class Anchor{}long read(final long first,@FinalWordMark int last){last++;return first+last;}}
class MixedFinalDriver{public static void main(String[]args)throws Exception{MixedFinalOwner owner=new MixedFinalOwner();for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(int k:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})if(owner.read(n,k)!=n+(k+1))throw new AssertionError("logical parameter index versus slot words");java.lang.reflect.Parameter[]p=MixedFinalOwner.class.getDeclaredMethod("read",long.class,int.class).getParameters();if(p[0].getModifiers()!=16||p[1].getModifiers()!=16||p[1].getAnnotations().length!=1)throw new AssertionError("annotation and finality projection");System.out.println("instance:wide:index:annotation:storage");}}`
	testFinalParameterDefinition(t, source, "MixedFinalOwner", "MixedFinalDriver", "instance:wide:index:annotation:storage\n", func(t *testing.T, obj *ClassObject) {
		finalParameterMarkReadFinal(t, obj, 1)
	}, false)
}

func TestAdversarialFinalParameterDefinitionPartialWideWordRefused(t *testing.T) {
	const source = `class OverlapFinalOwner{static class Anchor{}static int read(final long input){return 7;}}
class OverlapFinalDriver{public static void main(String[]args){if(OverlapFinalOwner.read(Long.MIN_VALUE)!=7)throw new AssertionError("valid category-2 high-word reuse");System.out.println("wide:overlap:original");}}`
	testFinalParameterDefinition(t, source, "OverlapFinalOwner", "OverlapFinalDriver", "wide:overlap:original\n", func(t *testing.T, obj *ClassObject) {
		code := finalParameterReadCode(t, obj)
		code.AttrLen += 2
		code.Code = []byte{0x10, 7, 0x3c, 0x1b, 0xac}
	}, true)
}

// Entry copying a reference would add a strong root after the physical JVM
// parameter slot is overwritten. The current projection proves primitive
// storage only; even an ordinary reference/array assignment needs a separate
// lifetime certificate. The original methods remain valid JVM programs.
func TestAdversarialFinalParameterReferenceStorageRequiresLifetimeProof(t *testing.T) {
	for _, array := range []bool{false, true} {
		name := "object"
		typ := "Object"
		values := "new Object[]{null,new Object()}"
		if array {
			name = "array"
			typ = "Object[]"
			values = "new Object[][]{null,new Object[0],new Object[]{new Object()}}"
		}
		t.Run(name, func(t *testing.T) {
			source := `class ReferenceFinalOwner{static class Anchor{}static ` + typ + ` read(` + typ + ` input,` + typ + ` replacement){input=replacement;return input;}}
   class ReferenceFinalDriver{public static void main(String[]args){for(` + typ + ` input:` + values + `)for(` + typ + ` replacement:` + values + `)if(ReferenceFinalOwner.read(input,replacement)!=replacement)throw new AssertionError("original reference slot value");System.out.println("reference:original:slot:identity");}}`
			testFinalParameterDefinition(t, source, "ReferenceFinalOwner", "ReferenceFinalDriver", "reference:original:slot:identity\n", func(t *testing.T, obj *ClassObject) { finalParameterMarkReadFinal(t, obj, 0) }, true)
		})
	}
}

package javaclassparser

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

const nativeInterfaceVarargsFixture = `class VarargsScope {
 final int seed;VarargsScope(int seed){this.seed=seed;}
 interface Operation {int apply(int... values);}
 class Worker implements Operation {
  int[] seen;
  public int apply(int... values){seen=values;if(values==null)return seed;int sum=seed;for(int value:values)sum+=value;return sum;}
  public int apply(int wrong){throw new AssertionError("wrong overload");}
 }
 Worker worker(){return new Worker();}
}
class VarargsDriver{public static void main(String[]args){
 for(int seed:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
  VarargsScope.Worker worker=new VarargsScope(seed).worker();VarargsScope.Operation operation=worker;
  int[] values=new int[]{Integer.MAX_VALUE,1,-3};
  if(operation.apply(values)!=seed+Integer.MAX_VALUE+1-3||worker.seen!=values)throw new AssertionError("array identity/overflow");
  if(operation.apply((int[])null)!=seed||worker.seen!=null)throw new AssertionError("null array");
  if(operation.apply()!=seed||worker.seen==null||worker.seen.length!=0)throw new AssertionError("empty varargs");
  if(operation.apply(7)!=seed+7)throw new AssertionError("overload binding");
 }System.out.println("interface:varargs:identity:binding");}}
`

const nativeInterfaceReferenceVarargsFixture = `class VarargsScope {
 final int seed;VarargsScope(int seed){this.seed=seed;}
 interface Operation {FORMAL ARRAY identity(ELEMENT... values);}
 class Worker implements Operation {
  int mark;ARRAY_SEEN seen;
  public FORMAL ARRAY identity(ELEMENT... values){mark=seed;seen=values;return values;}
  public Object identity(Object wrong){throw new AssertionError("wrong overload");}
 }
 Worker worker(){return new Worker();}
}
class VarargsDriver{public static void main(String[]args){
 for(int seed:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
  VarargsScope.Worker worker=new VarargsScope(seed).worker();VarargsScope.Operation operation=worker;
  DATA[] values=new DATA[]{TOKEN,null};
  if(operation.identity(values)!=values||worker.seen!=values||worker.mark!=seed)throw new AssertionError("array identity/state");
  if(operation.identity((DATA[])null)!=null||worker.seen!=null)throw new AssertionError("null array");
  if(operation.identity().length!=0)throw new AssertionError("empty varargs");
  if(operation.identity(TOKEN)[0]!=TOKEN)throw new AssertionError("overload binding");
 }System.out.println("interface:varargs:identity:binding");}}
`

func nativeInterfaceVarargsSource(variant string) string {
	switch variant {
	case "reference":
		return strings.NewReplacer("FORMAL", "", "ARRAY_SEEN", "Object[]", "ARRAY", "Object[]", "ELEMENT", "Object", "DATA", "Object", "TOKEN", "VarargsDriver.class").Replace(nativeInterfaceReferenceVarargsFixture)
	case "bounded generic":
		return strings.NewReplacer("FORMAL", "<T extends CharSequence>", "ARRAY_SEEN", "CharSequence[]", "ARRAY", "T[]", "ELEMENT", "T", "DATA", "String", "TOKEN", `"fixed"`).Replace(nativeInterfaceReferenceVarargsFixture)
	case "default":
		source := strings.Replace(nativeInterfaceVarargsFixture, "interface Operation {int apply(int... values);}", "interface Operation {default int apply(int... values){if(values==null)return 0;int sum=0;for(int value:values)sum+=value;return sum;}}", 1)
		source = strings.Replace(source, "public int apply(int... values){seen=values;if(values==null)return seed;int sum=seed;for(int value:values)sum+=value;return sum;}", "Worker(){seen=new int[]{seed};}", 1)
		source = strings.Replace(source, "||worker.seen!=values", "||worker.seen[0]!=seed", 1)
		source = strings.Replace(source, "seed+Integer.MAX_VALUE+1-3", "Integer.MAX_VALUE+1-3", 1)
		source = strings.Replace(source, "operation.apply((int[])null)!=seed||worker.seen!=null", "operation.apply((int[])null)!=0", 1)
		source = strings.Replace(source, "operation.apply()!=seed||worker.seen==null||worker.seen.length!=0", "operation.apply()!=0", 1)
		return strings.Replace(source, "operation.apply(7)!=seed+7", "operation.apply(7)!=7", 1)
	case "static":
		source := strings.Replace(nativeInterfaceVarargsSource("default"), "default int apply", "static int apply", 1)
		return strings.ReplaceAll(source, "operation.apply(", "VarargsScope.Operation.apply(")
	default:
		return nativeInterfaceVarargsFixture
	}
}

func TestNativeMemberInterfaceVarargsCallingConventionRoundTrip(t *testing.T) {
	for _, variant := range []string{"primitive", "reference", "bounded generic", "default", "static"} {
		t.Run(variant, func(t *testing.T) {
			source := nativeInterfaceVarargsSource(variant)
			t.Run("modern", func(t *testing.T) {
				testNativeIndependentFamilyFixture(t, source, []string{"VarargsScope"}, "VarargsDriver", "interface:varargs:identity:binding\n", nativeLexicalExactSignatures)
			})
			t.Run("native8", func(t *testing.T) {
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return localGetterCompileJavac8(t, map[string]string{"VarargsScope.java": source}, debug)
				}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{"VarargsScope"}, "VarargsDriver", "interface:varargs:identity:binding\n", nil, nativeLexicalExactSignatures)
			})
		})
	}
}

// Java 5 introduced this calling convention. Compile genuine major-49
// originals instead of changing a modern class's version field as evidence.
func TestNativeMemberInterfaceVarargsActualJava5RoundTrip(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Fatal("actual javac8 required for authored Java5 originals")
	}
	for _, variant := range []string{"primitive", "reference", "bounded generic"} {
		t.Run(variant, func(t *testing.T) {
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
				dir := t.TempDir()
				file := filepath.Join(dir, "VarargsScope.java")
				if err := os.WriteFile(file, []byte(nativeInterfaceVarargsSource(variant)), 0600); err != nil {
					t.Fatal(err)
				}
				if out, err := exec.Command(javac, "-proc:none", "-source", "5", "-target", "5", "-g:"+debug, "-d", dir, file).CombinedOutput(); err != nil {
					t.Fatalf("authored Java5: %v\n%s", err, out)
				}
				entries, err := os.ReadDir(dir)
				if err != nil {
					t.Fatal(err)
				}
				files := map[string][]byte{}
				for _, entry := range entries {
					if strings.HasSuffix(entry.Name(), ".class") {
						raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
						if err != nil {
							t.Fatal(err)
						}
						obj, err := Parse(raw)
						if err != nil || obj.MajorVersion != 49 {
							t.Fatalf("actual major49: %s %v", entry.Name(), err)
						}
						files[entry.Name()] = raw
					}
				}
				return files
			}, NativeJavac8, javac, []string{"VarargsScope"}, "VarargsDriver", "interface:varargs:identity:binding\n", nil, nativeLexicalExactSignatures)
		})
	}
}

func TestNativeMemberInterfaceVarargsRequiresOriginalArrayFormalAndKind(t *testing.T) {
	files := nativeCompileClasses(t, nativeInterfaceVarargsFixture)
	for _, variant := range []string{"original", "no formals", "scalar formal", "not last array", "bad descriptor", "overlong descriptor", "before varargs version", "private", "synthetic", "bridge", "abstract Code", "static without Code", "default without Code", "old static", "old default", "initializer", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(files["VarargsScope$Operation.class"])
			if err != nil {
				t.Fatal(err)
			}
			_, _, flags, known := originalMemberOwner(obj)
			if !known || len(obj.Methods) != 1 || obj.Methods[0].AccessFlags != 0x0481 {
				t.Fatal("original abstract varargs declaration")
			}
			method := obj.Methods[0]
			var work *workbudget.Budget
			switch variant {
			case "no formals":
				method.DescriptorIndex = nativeMemberLambdaTestUTF8(obj, "()I")
			case "scalar formal":
				method.DescriptorIndex = nativeMemberLambdaTestUTF8(obj, "(I)I")
			case "not last array":
				method.DescriptorIndex = nativeMemberLambdaTestUTF8(obj, "([II)I")
			case "bad descriptor":
				method.DescriptorIndex = nativeMemberLambdaTestUTF8(obj, "([V)I")
			case "overlong descriptor":
				method.DescriptorIndex = nativeMemberLambdaTestUTF8(obj, "([L"+strings.Repeat("a", 65535)+";)I")
			case "before varargs version":
				obj.MajorVersion = 48
			case "private":
				method.AccessFlags = 0x0082
			case "synthetic":
				method.AccessFlags |= 0x1000
			case "bridge":
				method.AccessFlags |= 0x0040
			case "abstract Code":
				method.Attributes = append(method.Attributes, &CodeAttribute{})
			case "static without Code":
				method.AccessFlags = 0x0089
			case "default without Code":
				method.AccessFlags = 0x0081
			case "old static", "old default":
				method.AccessFlags = 0x0081
				if variant == "old static" {
					method.AccessFlags = 0x0089
				}
				obj.MajorVersion = 51
				method.Attributes = append(method.Attributes, &CodeAttribute{})
			case "initializer":
				method.NameIndex = nativeMemberLambdaTestUTF8(obj, "<clinit>")
				method.DescriptorIndex = nativeMemberLambdaTestUTF8(obj, "()V")
				method.AccessFlags = 0x0088
				method.Attributes = append(method.Attributes, &CodeAttribute{})
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberDeclarationKindRepresentable(obj, flags, work); got != (variant == "original") {
				t.Fatalf("admitted=%v", got)
			}
		})
	}
}

func TestNativeMemberInterfaceVarargsDescriptorMemoKeepsPerMethodChecks(t *testing.T) {
	files := nativeCompileClasses(t, nativeInterfaceVarargsFixture)
	for _, variant := range []string{"shared descriptor", "second scalar descriptor", "second synthetic", "second abstract Code", "second static without Code", "per method budget"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(files["VarargsScope$Operation.class"])
			if err != nil {
				t.Fatal(err)
			}
			_, _, flags, known := originalMemberOwner(obj)
			if !known || len(obj.Methods) != 1 {
				t.Fatal("original interface")
			}
			second := *obj.Methods[0]
			second.NameIndex = nativeMemberLambdaTestUTF8(obj, "other")
			obj.Methods = append(obj.Methods, &second)
			work := workbudget.New(nil, workbudget.Limits{MaxGraphScans: 7}) // two method visits plus one five-byte descriptor
			switch variant {
			case "second scalar descriptor":
				second.DescriptorIndex = nativeMemberLambdaTestUTF8(obj, "(I)I")
				work = nil
			case "second synthetic":
				second.AccessFlags |= 0x1000
			case "second abstract Code":
				second.Attributes = append([]AttributeInfo(nil), second.Attributes...)
				second.Attributes = append(second.Attributes, &CodeAttribute{})
			case "second static without Code":
				second.AccessFlags = 0x0089
			case "per method budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 6})
			}
			if got := nativeMemberDeclarationKindRepresentable(obj, flags, work); got != (variant == "shared descriptor") {
				t.Fatalf("admitted=%v", got)
			}
		})
	}
}

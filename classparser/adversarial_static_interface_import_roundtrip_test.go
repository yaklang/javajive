package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdversarialNestedStaticInterfaceImportsShareWholeSourceTransaction(t *testing.T) {
	const api = `package owners;public interface ApiProducer{static int compute(int x){OwnerState.calls++;if(x<0)throw OwnerState.failure;return x*17+5;}}`
	const state = `package owners;public class OwnerState{public static int calls;public static final RuntimeException failure=new RuntimeException("same");}`
	const thunk = `interface ImportThunk{int get();}`
	const decoy = `class ImportNestedDecoy{static ImportNestedDecoy ApiProducer;static int compute(int x){throw new AssertionError("decoy");}}`
	javac, java := t04Tools(t)
	for _, kind := range []string{"anonymous", "member"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(kind+"/"+debug, func(t *testing.T) {
				body := `static ImportThunk make(final int x){return new ImportThunk(){public int get(){return compute(x);}};}`
				if kind == "member" {
					body = `class Entry implements ImportThunk{final int x;Entry(int x){this.x=x;}public int get(){return compute(x);}}ImportThunk make(int x){return new Entry(x);}`
				}
				root := `import static owners.ApiProducer.compute;class ImportNestedRoot{static ImportNestedDecoy ApiProducer=new ImportNestedDecoy(),owners=new ImportNestedDecoy();` + body + `}`
				driver := `class ImportNestedDriver{public static void main(String[]args){int rows=0;for(int x:new int[]{Integer.MIN_VALUE,-1,0,1,17,Integer.MAX_VALUE}){owners.OwnerState.calls=0;ImportThunk thunk=new ImportNestedRoot().make(x);try{if(thunk.get()!=java.math.BigInteger.valueOf(x).multiply(java.math.BigInteger.valueOf(17)).add(java.math.BigInteger.valueOf(5)).intValue()||x<0)throw new AssertionError("value");}catch(RuntimeException e){if(x>=0||e!=owners.OwnerState.failure)throw new AssertionError("identity");}if(owners.OwnerState.calls!=1)throw new AssertionError("once");rows++;}System.out.println(rows+":nested-import:values:once:identity");}}`
				files := nativeCompileSourceReleaseClasses(t, map[string]string{"ImportNestedRoot.java": root, "ImportNestedDriver.java": driver, "ImportThunk.java": thunk, "ImportNestedDecoy.java": decoy, "owners/ApiProducer.java": api, "owners/OwnerState.java": state}, debug, "8")
				original := t.TempDir()
				rebuilt := t.TempDir()
				for name, raw := range files {
					for _, dir := range []string{original, rebuilt} {
						if dir == rebuilt && strings.HasPrefix(name, "ImportNestedRoot") {
							continue
						}
						path := filepath.Join(dir, name)
						if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(path, raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
				}
				oracle := t04RunJava(t, java, original, "ImportNestedDriver")
				if oracle != "6:nested-import:values:once:identity\n" {
					t.Fatal(oracle)
				}
				archive := nativeArchive(t, files)
				defer archive.Close()
				source, err := archive.ReadFile("ImportNestedRoot.class")
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(source), "import static owners.ApiProducer.compute;") {
					t.Fatalf("nested declaration lost whole-unit import: %s", source)
				}
				path := filepath.Join(rebuilt, "ImportNestedRoot.java")
				if err := os.WriteFile(path, source, 0600); err != nil {
					t.Fatal(err)
				}
				if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", rebuilt, "-d", rebuilt, path).CombinedOutput(); err != nil {
					t.Fatalf("candidate %v %s\n%s", err, out, source)
				}
				if got := t04RunJava(t, java, rebuilt, "ImportNestedDriver"); got != oracle {
					t.Fatal(got)
				}
			})
		}
	}
}

func TestAdversarialInterfaceStaticImportKeepsOwnerAgainstCompilingDecoys(t *testing.T) {
	const api = `package owners;public interface ApiProducer{static int compute(int x){OwnerState.calls++;if(x<0)throw OwnerState.failure;return x*17+5;}static long compute(long x){throw new AssertionError("wrong overload");}}`
	const source = `import static owners.ApiProducer.compute;
class OwnerDecoy{static OwnerDecoy ApiProducer=new OwnerDecoy();static int calls;static int compute(int x){calls++;return 123;}static long compute(long x){calls++;return 456;}}
class InterfaceImportProbe{static OwnerDecoy ApiProducer=new OwnerDecoy(),owners=new OwnerDecoy();static int m(int x){return compute(x);}}
public class InterfaceImportDriver{public static void main(String[]args){int rows=0;for(int x:new int[]{Integer.MIN_VALUE,-1,0,1,17,Integer.MAX_VALUE}){owners.OwnerState.calls=0;OwnerDecoy.calls=0;try{int got=InterfaceImportProbe.m(x);if(x<0||got!=java.math.BigInteger.valueOf(x).multiply(java.math.BigInteger.valueOf(17)).add(java.math.BigInteger.valueOf(5)).intValue())throw new AssertionError("value/overload");}catch(RuntimeException e){if(x>=0||e!=owners.OwnerState.failure)throw new AssertionError("failure identity");}if(owners.OwnerState.calls!=1||OwnerDecoy.calls!=0)throw new AssertionError("owner/effects/once");rows++;}System.out.println(rows+":interface:static-import:value:effects:failure");}}`
	for _, scope := range []string{"own fields", "inherited fields", "package root type", "package root formal", "interface type formal"} {
		for _, method := range []string{"compute", "mix", "produce"} {
			t.Run(scope+"/"+method, func(t *testing.T) {
				variant := source
				switch scope {
				case "inherited fields":
					variant = strings.Replace(variant, "class InterfaceImportProbe{static OwnerDecoy ApiProducer=new OwnerDecoy(),owners=new OwnerDecoy();", "class ImportFieldParent{static OwnerDecoy ApiProducer=new OwnerDecoy(),owners=new OwnerDecoy();}class InterfaceImportProbe extends ImportFieldParent{", 1)
				case "package root type":
					variant = strings.Replace(variant, "class InterfaceImportProbe{static OwnerDecoy ApiProducer=new OwnerDecoy(),owners=new OwnerDecoy();", "class owners{}class InterfaceImportProbe{static OwnerDecoy ApiProducer=new OwnerDecoy();", 1)
				case "package root formal":
					variant = strings.Replace(variant, "static OwnerDecoy ApiProducer=new OwnerDecoy(),owners=new OwnerDecoy();", "static OwnerDecoy ApiProducer=new OwnerDecoy();", 1)
					variant = strings.Replace(variant, "static int m(int x)", "static <owners> int m(int x)", 1)
				case "interface type formal":
					variant = strings.Replace(variant, "static OwnerDecoy ApiProducer=new OwnerDecoy(),owners=new OwnerDecoy();", "static OwnerDecoy owners=new OwnerDecoy();", 1)
					variant = strings.Replace(variant, "static int m(int x)", "static <ApiProducer> int m(int x)", 1)
				}
				// The driver's own qualified package observations must remain outside
				// a compilation unit that declares a same-spelled package-root type.
				if scope == "package root type" {
					variant = strings.Replace(variant, "owners.OwnerState", "OwnerState", -1)
					variant = strings.Replace(variant, "import static owners.ApiProducer.compute;", "import static owners.ApiProducer.compute;import owners.OwnerState;", 1)
				}
				variant = strings.ReplaceAll(variant, "compute", method)
				apiVariant := strings.ReplaceAll(api, "compute", method)
				roundTripGenericFlowSources(t, "InterfaceImportDriver", variant, map[string]string{"owners/ApiProducer.java": apiVariant, "owners/OwnerState.java": `package owners;public class OwnerState{public static int calls;public static final RuntimeException failure=new RuntimeException("same");}`}, nil, []string{"InterfaceImportProbe"}, true, Precision, Compatibility, "legacy")
			})
		}
	}
}

// Rename unused fields after compiling an authored original. The JVM owner
// reference is unchanged and executes correctly; the corresponding source
// namespace becomes unnameable because an own method shadows a static import.
func TestAdversarialUnnameableInterfaceOwnerRejectsUnitWithExactMethodDiagnostic(t *testing.T) {
	const api = `package owners;public interface ApiProducer{static int compute(int x){return x*17+5;}}`
	const source = `class RejectDecoy{static RejectDecoy ApiProducer;static int compute(int x){throw new AssertionError("decoy");}}
class RejectProbe{static RejectDecoy decoyType,decoyRoot;static int compute(long x){throw new AssertionError("own overload");}static int m(int x){return owners.ApiProducer.compute(x);}}
class RejectDriver{public static void main(String[]args){for(int x:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})if(RejectProbe.m(x)!=java.math.BigInteger.valueOf(x).multiply(java.math.BigInteger.valueOf(17)).add(java.math.BigInteger.valueOf(5)).intValue())throw new AssertionError("binding");System.out.println("5:original-owner");}}`
	_, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileSourceReleaseClasses(t, map[string]string{"RejectProbe.java": source, "owners/ApiProducer.java": api}, debug, "8")
			obj, err := Parse(files["RejectProbe.class"])
			if err != nil {
				t.Fatal(err)
			}
			renames := 0
			for _, cp := range obj.ConstantPool {
				if word, ok := cp.(*ConstantUtf8Info); ok {
					switch word.Value {
					case "decoyType":
						word.Value = "ApiProducer"
						renames++
					case "decoyRoot":
						word.Value = "owners"
						renames++
					}
				}
			}
			if renames != 2 {
				t.Fatal("unused field declarations were not independently renamed")
			}
			files["RejectProbe.class"] = obj.Bytes()
			original := t.TempDir()
			for name, raw := range files {
				path := filepath.Join(original, name)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got := t04RunJava(t, java, original, "RejectDriver"); got != "5:original-owner\n" {
				t.Fatal(got)
			}
			resolve := func(name string) ([]byte, bool) { raw, known := files[name+".class"]; return raw, known }
			for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
				t.Run(string(mode), func(t *testing.T) {
					if mode == "legacy" {
						text, err := DecompileWithResolver(files["RejectProbe.class"], resolve)
						if err == nil || text != "" {
							t.Fatalf("legacy accepted unnameable binding: %v %s", err, text)
						}
						return
					}
					result, err := DecompileWithOptions(files["RejectProbe.class"], DecompileOptions{Mode: mode, Resolve: resolve, TargetSourceVersion: 8})
					if err == nil || result.Source != "" || result.Status != "unsupported" || len(result.StubMethods) != 0 {
						t.Fatalf("unnameable source was published or stubbed: err=%v status=%s source=%s", err, result.Status, result.Source)
					}
					found := false
					for _, diagnostic := range result.Diagnostics {
						if diagnostic.Code == "static_owner_binding_unknown" && diagnostic.Method == "RejectProbe.m(I)I" {
							found = true
						}
					}
					if !found {
						t.Fatalf("missing original source-method binding diagnosis: %+v", result.Diagnostics)
					}
					for _, member := range result.Members {
						if member.State != "unsupported" || !strings.Contains(member.Evidence, "source unit rejected") {
							t.Fatalf("rejected unit retained a positive declaration witness: %+v", member)
						}
					}
				})
			}
		})
	}
}

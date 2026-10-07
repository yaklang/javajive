package javaclassparser

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

// A call's descriptor can mention a source member without a CONSTANT_Class
// entry. CONSTANT_NameAndType and CONSTANT_MethodType are binding evidence,
// including when the result is immediately widened to Object.
const callsiteDependencyFixture = `class CallsiteOwner{static class Anchor{}static Object run(){return CallsiteFactory.make();}}
class CallsiteFactory{static ForeignCallsiteScope.Value make(){return new ForeignCallsiteScope.Value();}}
class ForeignCallsiteScope{static class Value{}}
class CallsiteDriver{public static void main(String[]args){Object a=CallsiteOwner.run();if(a.getClass()!=ForeignCallsiteScope.Value.class)throw new AssertionError();System.out.println("callsite:return:identity");}}`

func TestNativeCallsiteDescriptorDependencyRequiresOriginalTypeClosure(t *testing.T) {
	javac, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, callsiteDependencyFixture, debug)
			root, err := Parse(files["CallsiteOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			// Remove only redundant metadata and its unused class constant. Keep
			// the actual original invocation and its descriptor byte-for-byte.
			const target = "ForeignCallsiteScope$Value"
			for _, attr := range root.Attributes {
				if table, ok := attr.(*InnerClassesAttribute); ok {
					var kept []*InnerClassInfo
					for _, row := range table.Classes {
						name, known := sourceBridgeClassName(root, row.InnerClassInfoIndex)
						if !known {
							t.Fatal("original row")
						}
						if name != target {
							kept = append(kept, row)
						}
					}
					table.Classes, table.NumberOfClasses, table.AttrLen = kept, uint16(len(kept)), uint32(2+8*len(kept))
				}
			}
			for i, c := range root.ConstantPool {
				if cls, ok := c.(*ConstantClassInfo); ok {
					name, _ := sourceBridgeUTF8(root, cls.NameIndex)
					if name == target {
						root.ConstantPool[i] = NewUtf8FromString("unused class constant")
					}
				}
			}
			files["CallsiteOwner.class"] = root.Bytes()
			root, err = Parse(files["CallsiteOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			original := t.TempDir()
			for name, raw := range files {
				if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got := t04RunJava(t, java, original, "CallsiteDriver"); got != "callsite:return:identity\n" {
				t.Fatalf("original descriptor-only invocation %q", got)
			}
			refs, known := nativeMemberDependencyNames(root, nil)
			if !known || !slices.Contains(refs, target) {
				t.Fatalf("actual invocation return type omitted from original closure: known=%v references=%v", known, refs)
			}
			z := nativeArchive(t, files)
			defer z.Close()
			graph, known := z.nativeMemberOriginalDependencyGraph("CallsiteOwner", nil)
			if !known || !graph["CallsiteOwner"]["ForeignCallsiteScope"] {
				t.Fatalf("actual source binding dependency lost: known=%v graph=%v", known, graph)
			}
			index := z.originalMemberIndex()
			if !index.valid || !index.typeUsers[target]["CallsiteOwner"] {
				t.Fatal("archive index lost descriptor user")
			}
			for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(policy, func(t *testing.T) {
					if policy == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if policy == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					archive := nativeArchive(t, files)
					defer archive.Close()
					out := t.TempDir()
					var sources []string
					var names []string
					for name := range files {
						names = append(names, name)
					}
					slices.Sort(names)
					for _, name := range names {
						if name == "CallsiteDriver.class" {
							if err := os.WriteFile(filepath.Join(out, name), files[name], 0600); err != nil {
								t.Fatal(err)
							}
							continue
						}
						src, err := archive.ReadFile(name)
						if err != nil || strings.Contains(string(src), DecompileStubMarker) {
							t.Fatalf("production source %s: %v\n%s", name, err, src)
						}
						path := filepath.Join(out, strings.TrimSuffix(name, ".class")+".java")
						if err := os.WriteFile(path, src, 0600); err != nil {
							t.Fatal(err)
						}
						sources = append(sources, path)
					}
					args := append([]string{"-proc:none", "--release", "8", "-cp", out, "-d", out}, sources...)
					if log, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
						t.Fatalf("candidate javac: %v\n%s", err, log)
					}
					if got := t04RunJava(t, java, out, "CallsiteDriver"); got != "callsite:return:identity\n" {
						t.Fatalf("candidate behavior %q", got)
					}
					for _, name := range names {
						raw, err := os.ReadFile(filepath.Join(out, name))
						if err != nil {
							t.Fatal(err)
						}
						rebuilt, err := Parse(raw)
						if err != nil {
							t.Fatal(err)
						}
						original, err := Parse(files[name])
						if err != nil {
							t.Fatal(err)
						}
						if rebuilt.GetClassName() != original.GetClassName() {
							t.Fatal("physical class identity changed")
						}
						owner, label, flags, member := originalMemberOwner(original)
						rOwner, rLabel, rFlags, rMember := originalMemberOwner(rebuilt)
						if owner != rOwner || label != rLabel || flags != rFlags || member != rMember {
							t.Fatalf("source ownership changed for %s", name)
						}
					}
				})
			}
		})
	}
}

func TestNativeConstantPoolDescriptorDependenciesAreTypedAndBounded(t *testing.T) {
	for _, kind := range []string{"method return", "method argument array", "field", "method type", "primitive only", "ordinary string", "invalid name type", "invalid method type", "field in method type", "generic signature", "zero descriptor", "missing descriptor", "nil name type", "nil method type", "budget", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			obj := &ClassObject{ConstantPool: []ConstantInfo{NewUtf8FromString("(Ldependency/Input;[[Ldependency/Output;)Ldependency/Result;")}}
			var work *workbudget.Budget
			wanted := []string{"dependency/Input", "dependency/Output", "dependency/Result"}
			obj.ConstantPool = append(obj.ConstantPool, &ConstantNameAndTypeInfo{DescriptorIndex: 1})
			switch kind {
			case "method return":
				obj.ConstantPool[0] = NewUtf8FromString("()Ldependency/Result;")
				wanted = wanted[2:]
			case "field":
				obj.ConstantPool[0] = NewUtf8FromString("[Ldependency/Input;")
				wanted = wanted[:1]
			case "method type":
				obj.ConstantPool[1] = &ConstantMethodTypeInfo{DescriptorIndex: 1}
			case "primitive only":
				obj.ConstantPool[0] = NewUtf8FromString("(IJFDZ[[B)V")
				wanted = nil
			case "ordinary string":
				obj.ConstantPool[1] = &ConstantStringInfo{StringIndex: 1}
				wanted = nil
			case "invalid name type", "invalid method type":
				obj.ConstantPool[0] = NewUtf8FromString("(Ldependency/Truncated")
				if kind == "invalid method type" {
					obj.ConstantPool[1] = &ConstantMethodTypeInfo{DescriptorIndex: 1}
				}
			case "missing descriptor":
				obj.ConstantPool[1] = &ConstantNameAndTypeInfo{DescriptorIndex: 3}
			case "zero descriptor":
				obj.ConstantPool[1] = &ConstantNameAndTypeInfo{}
			case "nil name type":
				obj.ConstantPool[1] = (*ConstantNameAndTypeInfo)(nil)
			case "nil method type":
				obj.ConstantPool[1] = (*ConstantMethodTypeInfo)(nil)
			case "field in method type":
				obj.ConstantPool[0] = NewUtf8FromString("Ldependency/Input;")
				obj.ConstantPool[1] = &ConstantMethodTypeInfo{DescriptorIndex: 1}
			case "generic signature":
				obj.ConstantPool[0] = NewUtf8FromString("()Ljava/util/List<Ldependency/Input;>;")
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			refs, known := nativeMemberDependencyNames(obj, work)
			valid := slices.Contains([]string{"method return", "method argument array", "field", "method type", "primitive only", "ordinary string"}, kind)
			if known != valid {
				t.Fatalf("closure=%v want=%v references=%v", known, valid, refs)
			}
			if !known {
				if refs != nil {
					t.Fatal("partial dependencies published")
				}
				return
			}
			slices.Sort(refs)
			slices.Sort(wanted)
			if !slices.Equal(refs, wanted) {
				t.Fatalf("dependencies=%v want=%v", refs, wanted)
			}
		})
	}
}

// The oracle walks a finite descriptor AST's leaves, independently of both
// descriptor parsers. Enumerate all argument lists of length 0..2, dimensions
// 0..2, eight leaf types, every return type and both constant-pool roles.
func TestNativeConstantPoolDescriptorClosureAgainstFiniteTypeModel(t *testing.T) {
	type leaf struct{ descriptor, class string }
	var fields []leaf
	for _, atom := range []leaf{{"I", ""}, {"J", ""}, {"F", ""}, {"D", ""}, {"Z", ""}, {"Lmodel/Left;", "model/Left"}, {"Lmodel/Right;", "model/Right"}, {"Lmodel/Shared;", "model/Shared"}} {
		for dims := 0; dims <= 2; dims++ {
			fields = append(fields, leaf{strings.Repeat("[", dims) + atom.descriptor, atom.class})
		}
	}
	parameters := [][]leaf{nil}
	for _, first := range fields {
		parameters = append(parameters, []leaf{first})
		for _, second := range fields {
			parameters = append(parameters, []leaf{first, second})
		}
	}
	returns := append(append([]leaf(nil), fields...), leaf{"V", ""})
	samples := 0
	for _, args := range parameters {
		for _, result := range returns {
			desc := "("
			want := map[string]bool{}
			for _, arg := range args {
				desc += arg.descriptor
				if arg.class != "" {
					want[arg.class] = true
				}
			}
			desc += ")" + result.descriptor
			if result.class != "" {
				want[result.class] = true
			}
			for _, methodType := range []bool{false, true} {
				var constant ConstantInfo = &ConstantNameAndTypeInfo{DescriptorIndex: 1}
				if methodType {
					constant = &ConstantMethodTypeInfo{DescriptorIndex: 1}
				}
				obj := &ClassObject{ConstantPool: []ConstantInfo{NewUtf8FromString(desc), constant}}
				got, known := nativeMemberDependencyNames(obj, nil)
				if !known || len(got) != len(want) {
					t.Fatalf("descriptor=%s methodType=%v known=%v got=%v want=%v", desc, methodType, known, got, want)
				}
				for _, name := range got {
					if !want[name] {
						t.Fatalf("descriptor=%s introduced foreign type=%s", desc, name)
					}
				}
				samples++
			}
		}
	}
	if samples != 30050 {
		t.Fatalf("finite domain changed: %d", samples)
	}
}

func TestNativeDescriptorDependencyValidationSharesWorkWithoutSharingRoles(t *testing.T) {
	const desc = "([[Lmodel/Left;J)Lmodel/Right;"
	obj := &ClassObject{ConstantPool: []ConstantInfo{NewUtf8FromString(desc)}}
	for i := 0; i < 1024; i++ {
		obj.ConstantPool = append(obj.ConstantPool, &ConstantNameAndTypeInfo{DescriptorIndex: 1})
	}
	// One pool scan plus one grammar scan and one reference scan. Rechecking
	// the same long descriptor at every use cannot fit this linear budget.
	work := workbudget.New(nil, workbudget.Limits{MaxGraphScans: int64(len(obj.ConstantPool) + 2*len(desc) + 8)})
	refs, known := nativeMemberDependencyNames(obj, work)
	slices.Sort(refs)
	if !known || !slices.Equal(refs, []string{"model/Left", "model/Right"}) {
		t.Fatalf("repeated descriptor closure: known=%v refs=%v err=%v", known, refs, work.Err())
	}
	obj.ConstantPool = []ConstantInfo{NewUtf8FromString("Lmodel/Left;"), &ConstantNameAndTypeInfo{DescriptorIndex: 1}, &ConstantMethodTypeInfo{DescriptorIndex: 1}}
	if refs, known := nativeMemberDependencyNames(obj, nil); known || refs != nil {
		t.Fatalf("cached field grammar licensed a method type: %v %v", known, refs)
	}
	for _, kind := range []string{"field void", "dimensions255", "dimensions256", "slots255", "slots256", "wide slots254", "wide slots256", "memory"} {
		t.Run(kind, func(t *testing.T) {
			text := "Lmodel/Left;"
			var constant ConstantInfo = &ConstantNameAndTypeInfo{DescriptorIndex: 1}
			valid := true
			var work *workbudget.Budget
			switch kind {
			case "field void":
				text, valid = "V", false
			case "dimensions255":
				text = strings.Repeat("[", 255) + text
			case "dimensions256":
				text, valid = strings.Repeat("[", 256)+text, false
			case "slots255":
				text = "(" + strings.Repeat("I", 255) + ")Lmodel/Left;"
			case "slots256":
				text, valid = "("+strings.Repeat("I", 256)+")Lmodel/Left;", false
			case "wide slots254":
				text = "(" + strings.Repeat("J", 127) + ")Lmodel/Left;"
			case "wide slots256":
				text, valid = "("+strings.Repeat("J", 128)+")Lmodel/Left;", false
			case "memory":
				work, valid = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1}), false
			}
			if strings.HasPrefix(text, "(") {
				constant = &ConstantMethodTypeInfo{DescriptorIndex: 1}
			}
			obj := &ClassObject{ConstantPool: []ConstantInfo{NewUtf8FromString(text), constant}}
			got, known := nativeMemberDependencyNames(obj, work)
			if known != valid || !known && got != nil || known && !slices.Equal(got, []string{"model/Left"}) {
				t.Fatalf("grammar boundary known=%v refs=%v want=%v", known, got, valid)
			}
		})
	}
}

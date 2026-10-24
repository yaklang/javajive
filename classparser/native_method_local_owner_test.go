package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

// Metadata ownership alone must not authorize private/source access decisions.
// Every control starts from freshly parsed authored original class bytes.
func TestNativeMethodLocalOwnerRequiresJointOriginalDeclarations(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, `class LocalFactOwner {Object first(){class Entry{}return new Entry();}Object second(){class Entry{}return new Entry();}}`, debug)
			for _, variant := range []string{"first", "second", "pre EnclosingMethod version", "foreign owner", "absent declaring method", "wrong descriptor", "duplicate method", "missing EnclosingMethod", "duplicate EnclosingMethod", "bad attribute length", "no method index", "wrong CP kind", "missing self row", "duplicate self row", "unnamed row", "member outer row", "unsafe source name", "static self row", "missing parent corroboration", "duplicate parent corroboration", "wrong parent flags", "overlapping registration", "budget", "canceled"} {
				t.Run(variant, func(t *testing.T) {
					root, e := Parse(append([]byte(nil), files["LocalFactOwner.class"]...))
					if e != nil {
						t.Fatal(e)
					}
					ordinal := "1"
					methodName := "first"
					if variant == "second" {
						ordinal = "2"
						methodName = "second"
					}
					local, e := Parse(append([]byte(nil), files["LocalFactOwner$"+ordinal+"Entry.class"]...))
					if e != nil {
						t.Fatal(e)
					}
					var attr *UnparsedAttribute
					var self *InnerClassInfo
					var parent *InnerClassesAttribute
					var declaration *MemberInfo
					for _, a := range local.Attributes {
						if raw, ok := a.(*UnparsedAttribute); ok && raw.Name == "EnclosingMethod" {
							attr = raw
						}
						if table, ok := a.(*InnerClassesAttribute); ok {
							for _, row := range table.Classes {
								n, _ := sourceBridgeClassName(local, row.InnerClassInfoIndex)
								if n == local.GetClassName() {
									self = row
								}
							}
						}
					}
					for _, a := range root.Attributes {
						if table, ok := a.(*InnerClassesAttribute); ok {
							parent = table
						}
					}
					for _, m := range root.Methods {
						n, _ := sourceBridgeUTF8(root, m.NameIndex)
						if n == methodName {
							declaration = m
						}
					}
					if attr == nil || self == nil || parent == nil || declaration == nil {
						t.Fatal("original joint declaration")
					}
					var work *workbudget.Budget
					switch variant {
					case "pre EnclosingMethod version":
						local.MajorVersion = 48
					case "foreign owner":
						root = local
					case "absent declaring method":
						root.Methods = nil
					case "wrong descriptor":
						declaration.DescriptorIndex = root.ThisClass
					case "duplicate method":
						root.Methods = append(root.Methods, declaration)
					case "missing EnclosingMethod":
						attr.Name = "unknown"
					case "duplicate EnclosingMethod":
						local.Attributes = append(local.Attributes, attr)
					case "bad attribute length":
						attr.Length = 3
					case "no method index":
						attr.Info[2], attr.Info[3] = 0, 0
					case "wrong CP kind":
						attr.Info[2], attr.Info[3] = byte(local.ThisClass>>8), byte(local.ThisClass)
					case "missing self row":
						self.InnerClassInfoIndex = local.SuperClass
					case "duplicate self row":
						for _, a := range local.Attributes {
							if table, ok := a.(*InnerClassesAttribute); ok {
								table.Classes = append(table.Classes, self)
							}
						}
					case "unnamed row":
						self.InnerNameIndex = 0
					case "member outer row":
						self.OuterClassInfoIndex = local.ThisClass
					case "unsafe source name":
						local.ConstantPool[self.InnerNameIndex-1].(*ConstantUtf8Info).Value = "not valid"
					case "static self row":
						self.InnerClassAccessFlags |= 8
					case "missing parent corroboration":
						parent.Classes = nil
					case "duplicate parent corroboration":
						for _, row := range parent.Classes {
							n, _ := sourceBridgeClassName(root, row.InnerClassInfoIndex)
							if n == local.GetClassName() {
								parent.Classes = append(parent.Classes, row)
								break
							}
						}
					case "wrong parent flags":
						for _, row := range parent.Classes {
							n, _ := sourceBridgeClassName(root, row.InnerClassInfoIndex)
							if n == local.GetClassName() {
								row.InnerClassAccessFlags ^= 16
							}
						}
					case "overlapping registration":
						originalName := local.GetClassName()
						for _, constant := range local.ConstantPool {
							if text, ok := constant.(*ConstantUtf8Info); ok {
								if text.Value == originalName || text.Value == "Entry" {
									text.Value = "LocalFactOwner$"
								}
							}
						}
						for _, constant := range root.ConstantPool {
							if text, ok := constant.(*ConstantUtf8Info); ok {
								if text.Value == originalName || text.Value == "Entry" {
									text.Value = "LocalFactOwner$"
								}
							}
						}
					case "budget":
						work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
					case "canceled":
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						work = workbudget.New(ctx, workbudget.Limits{})
					}
					proof, known := originalMethodLocalOwner(local, root, work)
					if known != (variant == "first" || variant == "second") {
						t.Fatalf("ownership known=%v proof=%+v", known, proof)
					}
					if known && (proof.owner != "LocalFactOwner" || proof.method != methodName || proof.descriptor != "()Ljava/lang/Object;" || proof.name != "Entry" || proof.ordinal != int(ordinal[0]-'0') || proof.declaration != declaration) {
						t.Fatalf("wrong original method-local identity %+v", proof)
					}
				})
			}
		})
	}
}

func TestNativeMethodLocalLambdaAllocationKeepsOriginalLexicalOwner(t *testing.T) {
	source := `class LambdaLocalOwner {static java.util.function.Function<Object,java.util.function.Supplier<Object>> bind(final long weight,final Object left,final Object right){return input->{class Local implements java.util.function.Supplier<Object>{public Object get(){return weight==7?left:right;}}return new Local();};}}`
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileSourceReleaseClasses(t, map[string]string{"LambdaLocalOwner.java": source}, debug, "8")
			for _, mode := range []string{"original", "cached lexical method", "cached lexical descriptor", "cached lexical declaration", "missing synthetic", "wrong handle kind", "wrong bootstrap owner", "wrong bootstrap arity", "unreferenced implementation", "factory capture written", "implementation capture written", "shared factory", "ordinary implementation call", "duplicate implementation body", "foreign allocation user", "changed proof at publication", "cached execution descriptor", "cached execution declaration", "foreign source method", "missing source method", "cached capture vector", "original capture permutation", "javac8 lowering", "unknown lowering", "pre lambda source", "pre lambda input", "changed local version", "preview input", "budget", "memory", "canceled"} {
				t.Run(mode, func(t *testing.T) {
					root, e := Parse(append([]byte(nil), files["LambdaLocalOwner.class"]...))
					if e != nil {
						t.Fatal(e)
					}
					local, e := Parse(append([]byte(nil), files["LambdaLocalOwner$1Local.class"]...))
					if e != nil {
						t.Fatal(e)
					}
					lexical, ok := originalMethodLocalOwner(local, root, nil)
					if !ok || lexical.method != "bind" {
						t.Fatal("original lexical owner")
					}
					var impl *MemberInfo
					var factoryCode, implCode *CodeAttribute
					var boot *BootstrapMethodsAttribute
					for _, m := range root.Methods {
						n, _ := sourceBridgeUTF8(root, m.NameIndex)
						if strings.HasPrefix(n, "lambda$") {
							impl = m
						}
						for _, a := range m.Attributes {
							if code, ok := a.(*CodeAttribute); ok {
								if m == lexical.declaration {
									factoryCode = code
								}
								if strings.HasPrefix(n, "lambda$") {
									implCode = code
								}
							}
						}
					}
					for _, a := range root.Attributes {
						if b, ok := a.(*BootstrapMethodsAttribute); ok {
							boot = b
						}
					}
					if impl == nil || implCode == nil || factoryCode == nil || boot == nil || len(boot.BootstrapMethods) != 1 {
						t.Fatal("original compiler protocol")
					}
					z := nativeArchive(t, files)
					defer z.Close()
					d := z.nativeMemberReader(root)
					execution, known := d.nativeMethodLocalExecutionOwner(local, lexical)
					if !known || execution.declaration != impl || execution.method == lexical.method || lexical.declaration == impl {
						t.Fatal("independent physical and lexical roles")
					}
					originalLexical := *lexical
					constructor, known := originalMethodLocalSourceConstructor(local, root, nil)
					if !known {
						t.Fatal("original capture constructor")
					}
					sites, known := d.nativeMethodLocalAllocationFacts(local, lexical, constructor, true)
					if !known {
						t.Fatal("original typed allocation vector")
					}
					plan := &nativeMethodLocalClass{object: local, owner: lexical, allocationOwner: execution, constructor: constructor, allocations: sites, calls: map[int]bool{}}
					for pc := range sites {
						plan.calls[pc] = true
					}
					d.CurrentMethod = impl
					site := boot.BootstrapMethods[0]
					h := root.ConstantPool[site.BootstrapArguments[1]-1].(*ConstantMethodHandleInfo)
					switch mode {
					case "cached lexical method":
						lexical.method = "other"
					case "cached lexical descriptor":
						lexical.descriptor = "()Ljava/lang/Object;"
					case "cached lexical declaration":
						copy := *lexical.declaration
						lexical.declaration = &copy
					case "missing synthetic":
						impl.AccessFlags &^= 0x1000
					case "wrong handle kind":
						h.ReferenceKind = 5
					case "wrong bootstrap owner":
						handle := root.ConstantPool[site.BootstrapMethodRef-1].(*ConstantMethodHandleInfo)
						root.ConstantPool[handle.ReferenceIndex-1].(*ConstantMethodrefInfo).ClassIndex = uint16(root.ConstantPoolManager.AddNewClassInfo("probe/OpaqueFactory"))
					case "wrong bootstrap arity":
						site.NumBootstrapArguments++
					case "unreferenced implementation":
						site.BootstrapArguments[1] = site.BootstrapMethodRef
					case "factory capture written":
						factoryCode.Code = append(append([]byte{byte(core.OP_LCONST_0), byte(core.OP_LSTORE_0)}, factoryCode.Code...), byte(core.OP_NOP))
					case "implementation capture written":
						implCode.Code = append(append([]byte{byte(core.OP_LCONST_0), byte(core.OP_LSTORE_0)}, implCode.Code...), byte(core.OP_NOP))
					case "shared factory":
						first := append([]byte(nil), factoryCode.Code[:len(factoryCode.Code)-1]...)
						first = append(first, byte(core.OP_POP))
						factoryCode.Code = append(first, factoryCode.Code...)
					case "ordinary implementation call":
						factoryCode.Code = append(factoryCode.Code[:len(factoryCode.Code)-1], byte(core.OP_INVOKESTATIC), byte(h.ReferenceIndex>>8), byte(h.ReferenceIndex), byte(core.OP_ARETURN))
					case "duplicate implementation body":
						impl.Attributes = append(impl.Attributes, implCode)
					case "foreign allocation user":
						copy := *impl
						copy.NameIndex = uint16(root.ConstantPoolManager.AddUtf8Info("other"))
						root.Methods = append(root.Methods, &copy)
					case "changed proof at publication":
						h.ReferenceKind = 5
					case "cached execution descriptor":
						plan.allocationOwner.descriptor = "()Ljava/lang/Object;"
					case "cached execution declaration":
						copy := *impl
						plan.allocationOwner.declaration = &copy
					case "foreign source method":
						copy := *impl
						d.CurrentMethod = &copy
					case "missing source method":
						d.CurrentMethod = nil
					case "cached capture vector":
						for pc, site := range plan.allocations {
							site.slots = append([]int(nil), site.slots...)
							site.slots[0]++
							plan.allocations[pc] = site
						}
					case "original capture permutation":
						decoder := core.NewDecompiler(implCode.Code, func(i int) values.JavaValue { return GetValueFromCP(root.ConstantPool, i) })
						if decoder.ParseOpcode() != nil {
							t.Fatal("original implementation bytecode")
						}
						changed := 0
						for _, op := range decoder.Opcodes() {
							if op.Instr.OpCode == core.OP_ALOAD_2 {
								implCode.Code[op.CurrentOffset] = byte(core.OP_ALOAD_3)
								changed++
							} else if op.Instr.OpCode == core.OP_ALOAD_3 {
								implCode.Code[op.CurrentOffset] = byte(core.OP_ALOAD_2)
								changed++
							}
						}
						if changed != 2 {
							t.Fatalf("two original equal-erasure constructor operands: %d", changed)
						}
					case "javac8 lowering":
						d.options.SourceCompiler = NativeJavac8
						d.options.TargetSourceVersion = 8
					case "unknown lowering":
						d.options.SourceCompiler = "future"
					case "pre lambda source":
						d.options.TargetSourceVersion = 7
					case "pre lambda input":
						root.MajorVersion = 51
						local.MajorVersion = 51
					case "changed local version":
						local.MajorVersion++
					case "preview input":
						root.MinorVersion = 65535
						local.MinorVersion = 65535
					case "budget":
						d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
					case "memory":
						d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
					case "canceled":
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						d.Work = workbudget.New(ctx, workbudget.Limits{})
					}
					got := d.nativeMethodLocalExecutionValid(plan)
					plan.source = "class Local {}"
					d.nativeMemberRoot = &nativeMemberFamily{methodLocals: map[string]*nativeMethodLocalClass{local.GetClassName(): plan}}
					originalName, _ := sourceBridgeUTF8(root, impl.NameIndex)
					originalDesc, _ := sourceBridgeUTF8(root, impl.DescriptorIndex)
					published := d.nativeMethodLocalMethodSourceComplete(originalName, originalDesc, plan.source) && plan.sourceEmitted && nativeMethodLocalSourceComplete(d.nativeMemberRoot)
					if published != (mode == "original") {
						t.Fatalf("source publication accepted stale allocation scope=%v", published)
					}
					if got != (mode == "original" || mode == "foreign source method" || mode == "missing source method") {
						t.Fatalf("original physical scope reopened=%v", got)
					}
					if mode == "original" && (*lexical != originalLexical || lexical.declaration == execution.declaration) {
						t.Fatal("allocation scope replaced original lexical metadata")
					}
				})
			}
		})
	}
}

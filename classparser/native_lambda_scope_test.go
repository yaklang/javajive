package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeLambdaAllocationScopeRequiresOriginalFactoryAndStableWords(t *testing.T) {
	for _, mode := range []string{"static", "instance"} {
		source := `class CaptureScopeOwner{int base;CaptureScopeOwner(int n){base=n;}private CaptureScopeOwner(CaptureScopeOwner n){base=n.base;}static java.util.function.Supplier<CaptureScopeOwner> make(final CaptureScopeOwner input,final long first,final double second){return ()->new CaptureScopeOwner(input){int value(){return input.base+(int)first+(int)second;}};}}`
		if mode == "instance" {
			source = strings.Replace(source, "static java.util.function.Supplier", "java.util.function.Supplier", 1)
			source = strings.Replace(source, "return input.base+", "return CaptureScopeOwner.this.base+input.base+", 1)
		}
		files := nativeCompileClasses(t, source)
		for _, variant := range []string{"original", "cached implementation scope", "missing original child", "handler enters tuple", "foreign receiver", "wrong lexical scope", "wrong implementation descriptor", "missing synthetic", "wrong handle kind", "wrong bootstrap owner", "wrong bootstrap arity", "wrong instantiated result", "unreferenced handle", "factory written low word", "factory written high word", "implementation written capture", "computed capture", "duplicate factory site", "ordinary implementation call", "budget", "canceled"} {
			t.Run(mode+"/"+variant, func(t *testing.T) {
				root, err := Parse(append([]byte(nil), files["CaptureScopeOwner.class"]...))
				if err != nil {
					t.Fatal(err)
				}
				anon, err := Parse(append([]byte(nil), files["CaptureScopeOwner$1.class"]...))
				if err != nil {
					t.Fatal(err)
				}
				_, lexical, known := originalAnonymousOwner(anon)
				if !known {
					t.Fatal("missing original lexical method")
				}
				child := &nativeAnonymousClass{object: anon, method: lexical}
				var impl, factory *MemberInfo
				var implCode, factoryCode *CodeAttribute
				var name, desc string
				for _, m := range root.Methods {
					n, _ := sourceBridgeUTF8(root, m.NameIndex)
					d, _ := sourceBridgeUTF8(root, m.DescriptorIndex)
					if strings.HasPrefix(n, "lambda$") {
						if impl != nil {
							t.Fatal("ambiguous fixture")
						}
						impl = m
						name, desc = n, d
					}
					if n+d == lexical {
						factory = m
					}
				}
				if impl == nil || factory == nil {
					t.Fatal("missing implementation/factory")
				}
				for _, a := range impl.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						implCode = c
					}
				}
				for _, a := range factory.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						factoryCode = c
					}
				}
				var table *BootstrapMethodsAttribute
				for _, a := range root.Attributes {
					if b, ok := a.(*BootstrapMethodsAttribute); ok {
						table = b
					}
				}
				if table == nil || len(table.BootstrapMethods) != 1 || implCode == nil || factoryCode == nil {
					t.Fatal("fixture packet")
				}
				site := table.BootstrapMethods[0]
				cp := root.ConstantPoolManager
				decoder := core.NewDecompiler(factoryCode.Code, func(i int) values.JavaValue { return GetValueFromCP(root.ConstantPool, i) })
				if decoder.ParseOpcode() != nil {
					t.Fatal("factory bytecode")
				}
				ops := constructorMotionOps(decoder)
				var indy *core.OpCode
				for _, op := range ops {
					if op.Instr.OpCode == core.OP_INVOKEDYNAMIC {
						indy = op
					}
				}
				if indy == nil {
					t.Fatal("missing indy")
				}
				var work *workbudget.Budget
				switch variant {
				case "cached implementation scope":
					child.method = name + desc
				case "missing original child":
					child.object = nil
				case "handler enters tuple":
					factoryCode.ExceptionTable = append(factoryCode.ExceptionTable, &ExceptionTableEntry{StartPc: 0, EndPc: uint16(len(factoryCode.Code) - 1), HandlerPc: indy.CurrentOffset})
				case "foreign receiver":
					factoryCode.Code[0] = byte(core.OP_ALOAD_1)
				case "wrong lexical scope":
					child.method = "other()Ljava/lang/Object;"
				case "wrong implementation descriptor":
					desc = "()Ljava/lang/Object;"
				case "missing synthetic":
					impl.AccessFlags &^= 0x1000
				case "wrong handle kind":
					root.ConstantPool[site.BootstrapArguments[1]-1].(*ConstantMethodHandleInfo).ReferenceKind = 8
				case "wrong bootstrap owner":
					h := root.ConstantPool[site.BootstrapMethodRef-1].(*ConstantMethodHandleInfo)
					root.ConstantPool[h.ReferenceIndex-1].(*ConstantMethodrefInfo).ClassIndex = uint16(cp.AddNewClassInfo("probe/OpaqueFactory"))
				case "wrong bootstrap arity":
					site.NumBootstrapArguments++
				case "wrong instantiated result":
					root.ConstantPool[site.BootstrapArguments[2]-1].(*ConstantMethodTypeInfo).DescriptorIndex = uint16(cp.AddUtf8Info("()Ljava/lang/Object;"))
				case "unreferenced handle":
					site.BootstrapArguments[1] = site.BootstrapMethodRef
				case "factory written low word", "factory written high word":
					slot := 1
					if mode == "instance" {
						slot++
					}
					if variant == "factory written high word" {
						slot++
					}
					factoryCode.Code = append(factoryCode.Code[:len(factoryCode.Code)-1], byte(core.OP_DCONST_0), byte(core.OP_DSTORE), byte(slot), byte(core.OP_ARETURN))
				case "implementation written capture":
					slot := 0
					if mode == "instance" {
						slot = 1
					}
					implCode.Code = append(implCode.Code[:len(implCode.Code)-1], byte(core.OP_ACONST_NULL), byte(core.OP_ASTORE), byte(slot), byte(core.OP_ARETURN))
				case "computed capture":
					factoryCode.Code[0] = byte(core.OP_ACONST_NULL)
				case "duplicate factory site":
					factoryCode.Code = append(factoryCode.Code[:len(factoryCode.Code)-1], factoryCode.Code...)
				case "ordinary implementation call":
					h := root.ConstantPool[site.BootstrapArguments[1]-1].(*ConstantMethodHandleInfo)
					factoryCode.Code = append(factoryCode.Code[:len(factoryCode.Code)-1], byte(core.OP_INVOKESTATIC), byte(h.ReferenceIndex>>8), byte(h.ReferenceIndex), byte(core.OP_ARETURN))
				case "budget":
					work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				if got := nativeAnonymousAllocationScope(root, child, name, desc, work); got != (variant == "original") {
					t.Fatalf("scope certificate=%v", got)
				}
			})
		}
	}
}

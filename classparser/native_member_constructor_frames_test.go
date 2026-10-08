package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeMemberFrameDelegationRequiresOriginalReceiverAndBoundary(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"FrameProofOwner.java": `
class FrameProofParent {FrameProofParent(Object o,long n){}}
class FrameProofArg {static Object select(Object o){return o;}}
class FrameProofOwner {class Child extends FrameProofParent {
 Child(){this(7,new Object());}
 Child(long n,Object o){super(n==0?new FrameProofParent(o,n):FrameProofArg.select(o),new java.util.concurrent.atomic.AtomicLong(n).get());}
 Object owner(){return FrameProofOwner.this;}
}}`}, "none", "8")
	for _, variant := range []string{"original", "THIS", "THIS foreign enclosure", "absent method", "copied method", "duplicate method", "copied code", "duplicate code", "missing code", "static", "abstract", "native", "bad descriptor", "small stack", "small locals", "wrong start", "negative start", "wrong receiver", "wrong capture receiver", "wrong capture parameter", "missing opcode", "copied delegate PC", "prefix local definition", "prefix THIS alias", "prefix THIS duplication", "prefix backedge", "before-delegation handler", "budget", "allocation budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(append([]byte(nil), files["FrameProofOwner$Child.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			var method *MemberInfo
			var code *CodeAttribute
			start := 3
			for _, m := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, m.NameIndex)
				descriptor, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
				if name != "<init>" || strings.Contains(descriptor, "J") == strings.HasPrefix(variant, "THIS") {
					continue
				}
				method = m
				for _, attr := range m.Attributes {
					if c, ok := attr.(*CodeAttribute); ok {
						code = c
					}
				}
			}
			if method == nil || code == nil {
				t.Fatal("missing original constructor")
			}
			if strings.HasPrefix(variant, "THIS") {
				start = 0
			}
			var work *workbudget.Budget
			switch variant {
			case "absent method":
				for i, m := range obj.Methods {
					if m == method {
						obj.Methods = append(obj.Methods[:i], obj.Methods[i+1:]...)
						break
					}
				}
			case "copied method":
				copy := *method
				method = &copy
			case "duplicate method":
				obj.Methods = append(obj.Methods, method)
			case "copied code":
				copy := *code
				code = &copy
			case "duplicate code":
				method.Attributes = append(method.Attributes, code)
			case "missing code":
				method.Attributes = nil
			case "static":
				method.AccessFlags |= StaticFlag
			case "abstract":
				method.AccessFlags |= 0x0400
			case "native":
				method.AccessFlags |= 0x0100
			case "bad descriptor":
				method.DescriptorIndex = method.NameIndex
			case "small stack":
				code.MaxStack = 1
			case "small locals":
				code.MaxLocals = 1
			case "wrong start":
				start = 1
			case "negative start":
				start = -1
			case "wrong receiver":
				code.Code[5] = byte(core.OP_ALOAD_1)
			case "wrong capture receiver":
				code.Code[0] = byte(core.OP_ALOAD_1)
			case "wrong capture parameter":
				code.Code[1] = byte(core.OP_ALOAD_0)
			case "THIS foreign enclosure":
				code.Code[1] = byte(core.OP_ACONST_NULL)
			case "prefix local definition", "prefix THIS alias", "prefix THIS duplication", "prefix backedge":
				var prefix []byte
				switch variant {
				case "prefix local definition":
					prefix = []byte{byte(core.OP_ALOAD), 4, byte(core.OP_ASTORE), 5}
				case "prefix THIS alias":
					prefix = []byte{byte(core.OP_ALOAD_0), byte(core.OP_ASTORE), 5}
				case "prefix THIS duplication":
					prefix = []byte{byte(core.OP_DUP)}
				case "prefix backedge":
					prefix = []byte{byte(core.OP_ICONST_0), byte(core.OP_IFNE), 0, 6, byte(core.OP_GOTO), 0xff, 0xfc}
				}
				// Insert before every original branch, so its relative offsets
				// and targets remain unchanged. This is parsed/analysed only;
				// no altered original fixture is executed as the JVM oracle.
				code.Code = append(append(append([]byte(nil), code.Code[:6]...), prefix...), code.Code[6:]...)
				code.MaxLocals++
				code.MaxStack++
			case "before-delegation handler":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 5, EndPc: 6, HandlerPc: 5}}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation budget":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			decoder := core.NewDecompiler(code.Code, func(index int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, index) })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			ops := constructorMotionOps(decoder)
			if variant == "missing opcode" {
				ops[start] = nil
			}
			if variant == "copied delegate PC" {
				for i := len(ops) - 1; i >= 0; i-- {
					if m := constructorMotionMember(obj, ops[i], core.OP_INVOKESPECIAL); m != nil && m.Member == "<init>" && m.Name == obj.GetSupperClassName() {
						ops[i].CurrentOffset++
						break
					}
				}
			}
			next, member := nativeMemberFrameDelegation(obj, method, code, ops, start, work)
			want := variant == "original" || variant == "THIS"
			if (next > 0 && member != nil) != want {
				t.Fatalf("accepted=%v next=%d member=%v", next > 0 && member != nil, next, member)
			}
			if want {
				owner := obj.GetSupperClassName()
				if variant == "THIS" {
					owner = obj.GetClassName()
				}
				if member.Name != owner {
					t.Fatalf("NEW inside argument was mistaken for THIS: %s", member.Name)
				}
			}
		})
	}
}

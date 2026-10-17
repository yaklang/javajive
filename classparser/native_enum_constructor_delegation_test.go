package javaclassparser

import (
	"context"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func nativeEnumDelegationConstructor(t *testing.T, obj *ClassObject, descriptor string) (*MemberInfo, *CodeAttribute) {
	t.Helper()
	for _, method := range obj.Methods {
		name, _ := sourceBridgeUTF8(obj, method.NameIndex)
		desc, _ := sourceBridgeUTF8(obj, method.DescriptorIndex)
		if name == "<init>" && desc == descriptor {
			for _, attr := range method.Attributes {
				if code, ok := attr.(*CodeAttribute); ok && code != nil {
					return method, code
				}
			}
		}
	}
	t.Fatal("missing original constructor", descriptor)
	return nil, nil
}

func TestNativeEnumConstructorDelegationSourceSignatureBinding(t *testing.T) {
	const descriptor = "(Ljava/lang/String;ILjava/lang/Object;)V"
	for _, variant := range []string{"source signature", "metadata fallback", "raw descriptor", "generic raw rendering retained", "conflicting signature", "wrong source erasure"} {
		t.Run(variant, func(t *testing.T) {
			ctx := &class_context.ClassContext{ClassName: "ModelMode", FunctionName: "<init>", CurrentMethodDesc: "(Ljava/lang/String;I)V"}
			name := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.String"))
			ordinal := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
			this := &values.JavaRef{IsThis: true}
			call := &values.FunctionCallExpression{ClassName: ctx.ClassName, FunctionName: "<init>", Descriptor: descriptor, Object: this, Kind: values.InvokeSpecial, IsSpecialInvoke: true, HasOriginPC: true, OriginPC: 5, Arguments: []values.JavaValue{name, ordinal, values.JavaNull}}
			methodType, err := types.ParseMethodDescriptor(descriptor)
			if err != nil {
				t.Fatal(err)
			}
			call.FuncType = methodType.FunctionType()
			signature := "(Ljava/lang/Object;)V"
			if variant == "generic raw rendering retained" {
				signature = "<T:Ljava/lang/Object;>(TT;)V"
			}
			ctx.ConstructorSignaturesByDesc = map[string]string{descriptor: signature}
			declaration := callbinding.Class{Name: "ModelMode", Public: true, MembersComplete: true, Methods: []callbinding.Method{{Name: "<init>", Desc: descriptor, Signature: signature, Generic: true}}}
			if variant == "metadata fallback" || variant == "raw descriptor" {
				ctx.ConstructorSignaturesByDesc = nil
			}
			if variant == "raw descriptor" {
				declaration.Methods[0].Signature, declaration.Methods[0].Generic = "", false
			}
			if variant == "conflicting signature" {
				declaration.Methods[0].Signature = "(Ljava/lang/String;)V"
			}
			if variant == "wrong source erasure" {
				ctx.ConstructorSignaturesByDesc[descriptor] = "(Ljava/lang/String;)V"
				declaration.Methods[0].Signature = "(Ljava/lang/String;)V"
			}
			ctx.InvocationMetadata = func(owner string) (callbinding.Class, bool) { return declaration, owner == declaration.Name }
			before := append([]callbinding.Method(nil), declaration.Methods...)
			got, valid := enumThisDelegateSource(&statements.ExpressionStatement{Expression: call}, []values.JavaValue{name, ordinal}, ctx)
			if variant == "conflicting signature" || variant == "wrong source erasure" {
				if valid {
					t.Fatal("contradictory source declaration admitted", got)
				}
			} else {
				want := "this((Object)(null))"
				if variant == "generic raw rendering retained" {
					want = "this(null)"
				}
				if !valid || got != want {
					t.Fatalf("source %q/%t want %q", got, valid, want)
				}
			}
			if !reflect.DeepEqual(before, declaration.Methods) || call.Descriptor != descriptor || len(call.Arguments) != 3 {
				t.Fatal("source view changed original declaration or invocation")
			}
		})
	}
}

func nativeEnumDelegationMethodRef(t *testing.T, obj *ClassObject, descriptor string) uint16 {
	t.Helper()
	for i, constant := range obj.ConstantPool {
		ref, ok := constant.(*ConstantMethodrefInfo)
		if !ok || ref == nil {
			continue
		}
		owner, _ := sourceBridgeClassName(obj, ref.ClassIndex)
		if owner != obj.GetClassName() || ref.NameAndTypeIndex == 0 || int(ref.NameAndTypeIndex) > len(obj.ConstantPool) {
			continue
		}
		nt, ok := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
		if !ok || nt == nil {
			continue
		}
		name, _ := sourceBridgeUTF8(obj, nt.NameIndex)
		desc, _ := sourceBridgeUTF8(obj, nt.DescriptorIndex)
		if name == "<init>" && desc == descriptor {
			return uint16(i + 1)
		}
	}
	t.Fatal("missing original member reference", descriptor)
	return 0
}

func TestNativeEnumConstructorDelegationRejectsChangedHiddenRolesAndCycles(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"DelegateSwitchOwner.java": nativeEnumDelegationFixture}, "none", "8")
	for _, variant := range []string{
		"original", "substituted name", "substituted ordinal", "swapped equal typed arguments",
		"hidden ordinal duplicated as payload", "hidden ordinal removed then replaced", "extra uninitialized receiver alias",
		"hidden ordinal read after initialization", "hidden ordinal incremented", "hidden name overwritten", "wide store overlaps hidden name",
		"self cycle", "indirect cycle", "missing target", "wrong target pool category", "zero target index", "out of range target index",
		"wrong target descriptor", "wrong referenced owner", "nil nested name type", "duplicate code", "nil code", "missing code",
		"too little stack", "too few locals", "budget", "memory", "canceled",
	} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(append([]byte(nil), files["DelegateMode.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			method, code := nativeEnumDelegationConstructor(t, obj, "(Ljava/lang/String;I)V")
			target := nativeEnumDelegationMethodRef(t, obj, "(Ljava/lang/String;II)V")
			call := []byte{core.OP_INVOKESPECIAL, byte(target >> 8), byte(target)}
			prefix := []byte{core.OP_ALOAD_0, core.OP_ALOAD_1, core.OP_ILOAD_2}
			rewrite := func(body []byte, stack, locals uint16) {
				code.Code = append([]byte(nil), body...)
				code.MaxStack, code.MaxLocals = stack, locals
				code.Attributes = nil // No stale debug/frame offsets in a bytecode countermodel.
			}
			appendCall := func(body []byte) []byte {
				return append(append(body, call...), core.OP_RETURN)
			}
			var work *workbudget.Budget
			switch variant {
			case "substituted name":
				code.Code[1] = core.OP_ACONST_NULL
			case "substituted ordinal":
				code.Code[2] = core.OP_ICONST_0
			case "swapped equal typed arguments":
				rewrite(appendCall(append(prefix, core.OP_BIPUSH, 13, core.OP_SWAP)), 4, 3)
			case "hidden ordinal duplicated as payload":
				rewrite(appendCall(append(prefix, core.OP_DUP)), 4, 3)
			case "hidden ordinal removed then replaced":
				rewrite(appendCall(append(prefix, core.OP_POP, core.OP_ICONST_0, core.OP_ICONST_1)), 4, 3)
			case "extra uninitialized receiver alias":
				rewrite(appendCall(append(prefix, core.OP_ALOAD_0, core.OP_ASTORE_3, core.OP_BIPUSH, 13)), 4, 4)
			case "hidden ordinal read after initialization":
				rewrite(append(append([]byte(nil), code.Code[:len(code.Code)-1]...), core.OP_ILOAD_2, core.OP_POP, core.OP_RETURN), 4, 3)
			case "hidden ordinal incremented":
				rewrite(appendCall(append(prefix, core.OP_IINC, 2, 1, core.OP_BIPUSH, 13)), 4, 3)
			case "hidden name overwritten":
				rewrite(append(append([]byte(nil), code.Code[:len(code.Code)-1]...), core.OP_ACONST_NULL, core.OP_ASTORE_1, core.OP_RETURN), 4, 3)
			case "wide store overlaps hidden name":
				rewrite(append(append([]byte(nil), code.Code[:len(code.Code)-1]...), core.OP_LCONST_0, core.OP_LSTORE_0, core.OP_RETURN), 4, 3)
			case "self cycle":
				self := nativeEnumDelegationMethodRef(t, obj, "(Ljava/lang/String;I)V")
				rewrite(append(prefix, core.OP_INVOKESPECIAL, byte(self>>8), byte(self), core.OP_RETURN), 3, 3)
			case "indirect cycle":
				_, last := nativeEnumDelegationConstructor(t, obj, "(Ljava/lang/String;IIJ)V")
				self := nativeEnumDelegationMethodRef(t, obj, "(Ljava/lang/String;I)V")
				last.Code = append(prefix, core.OP_INVOKESPECIAL, byte(self>>8), byte(self), core.OP_RETURN)
				last.Attributes = nil
			case "missing target":
				missing, _ := nativeEnumDelegationConstructor(t, obj, "(Ljava/lang/String;II)V")
				var methods []*MemberInfo
				for _, candidate := range obj.Methods {
					if candidate != missing {
						methods = append(methods, candidate)
					}
				}
				obj.Methods = methods
			case "wrong target pool category":
				ref := obj.ConstantPool[target-1].(*ConstantMethodrefInfo)
				obj.ConstantPool[target-1] = &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
			case "zero target index":
				code.Code[len(code.Code)-3], code.Code[len(code.Code)-2] = 0, 0
			case "out of range target index":
				code.Code[len(code.Code)-3], code.Code[len(code.Code)-2] = 255, 255
			case "wrong target descriptor":
				ref := obj.ConstantPool[target-1].(*ConstantMethodrefInfo)
				nt := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				nt.DescriptorIndex = sourceBridgePoolString(t, obj, "(Ljava/lang/String;IJ)V")
			case "wrong referenced owner":
				obj.ConstantPool[target-1].(*ConstantMethodrefInfo).ClassIndex = obj.SuperClass
			case "nil nested name type":
				ref := obj.ConstantPool[target-1].(*ConstantMethodrefInfo)
				obj.ConstantPool[ref.NameAndTypeIndex-1] = (*ConstantNameAndTypeInfo)(nil)
			case "duplicate code":
				method.Attributes = append(method.Attributes, code)
			case "nil code":
				method.Attributes = []AttributeInfo{(*CodeAttribute)(nil)}
			case "missing code":
				method.Attributes = nil
			case "too little stack":
				code.MaxStack = 3
			case "too few locals":
				code.MaxLocals = 2
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberEnumSynthesisProof(obj, 0x4019, work) != nil; got != (variant == "original") {
				t.Fatalf("enum protocol admission=%t", got)
			}
		})
	}
}

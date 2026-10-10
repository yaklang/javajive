package javaclassparser

import (
	"context"
	"fmt"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func nativeBoundLambdaProofPacket(t *testing.T, files map[string][]byte) (*JarFS, *nativeAnonymousClass, int) {
	t.Helper()
	z := nativeArchive(t, files)
	t.Cleanup(func() { z.Close() })
	owner, err := Parse(append([]byte(nil), files["BoundLambdaOwner.class"]...))
	if err != nil {
		t.Fatal(err)
	}
	object, err := Parse(append([]byte(nil), files["BoundLambdaOwner$1.class"]...))
	if err != nil {
		t.Fatal(err)
	}
	d := z.nativeMemberReader(owner)
	packet := d.nativeAnonymousConstructorForCompiler(object, owner.GetClassName(), "make()LBoundLambdaBase;", "", nil, nil, d.buildInvocationMetadata(), nil)
	if packet == nil || packet.lambdaImplementation == nil || len(packet.lambdaImplementation.lambdaImplementations) != 1 {
		t.Fatal("original bound implementation packet")
	}
	index := 0
	for _, cp := range object.ConstantPool {
		if dynamic, ok := cp.(*ConstantInvokeDynamicInfo); ok && dynamic != nil {
			if index != 0 {
				t.Fatal("ambiguous original factory")
			}
			index = int(dynamic.NameAndTypeIndex)
		}
	}
	if index == 0 {
		t.Fatal("missing original factory")
	}
	return z, packet, index
}

func TestNativeAnonymousBoundLambdaSourceCannotSubstituteParameterForReceiver(t *testing.T) {
	files := nativeCompileClasses(t, anonymousBoundLambdaReceiverFixture)
	for _, variant := range []string{"original", "static slot zero", "same typed unsealed receiver", "changed seed", "missing source", "foreign method", "foreign code", "original snapshot", "snapshot wrong pc", "snapshot wrong ordinal", "snapshot parameter", "missing consumption", "stub"} {
		t.Run(variant, func(t *testing.T) {
			_, packet, _ := nativeBoundLambdaProofPacket(t, files)
			view := packet.lambdaImplementation
			var implementation *MemberInfo
			for method := range view.lambdaImplementations {
				implementation = method
			}
			name, _ := sourceBridgeUTF8(packet.object, implementation.NameIndex)
			desc, _ := sourceBridgeUTF8(packet.object, implementation.DescriptorIndex)
			site := view.lambdaContext.localCaptures[name+desc]
			if site == nil || len(site.operands) != 1 || site.operands[0].slot != 0 || site.method.AccessFlags&StaticFlag != 0 {
				t.Fatal("original implicit receiver source site")
			}
			typ := types.NewJavaClass(packet.object.GetClassName())
			receiver := values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, typ)
			receiver.IsParam, receiver.IsThis = true, true
			receiver.MarkOriginalParameter(0)
			receiver.MarkOriginalReceiver()
			parameter := values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, typ)
			parameter.IsParam = true
			parameter.MarkOriginalParameter(0)
			source := &nativeLambdaLocalCaptureSource{method: site.method, code: site.code, factoryPC: site.pc, hasFactoryPC: true, values: []values.JavaValue{receiver}}
			dumper := NewClassObjectDumper(packet.object)
			dumper.lambdaMethods[name] = []string{desc}
			body := &dumpedMethods{member: implementation, code: "return this.current;", bodyCode: "return this.current;"}
			dumper.dumpedMethodsSet[fmt.Sprintf("name:%s,desc:%s", name, desc)] = body
			dumper.nativeLambdaLocalSources = map[string]*nativeLambdaLocalCaptureSource{name + desc: source}
			switch variant {
			case "static slot zero":
				source.values[0] = parameter
			case "same typed unsealed receiver":
				unsealed := values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, typ)
				unsealed.IsParam, unsealed.IsThis = true, true
				unsealed.MarkOriginalParameter(0)
				source.values[0] = unsealed
			case "changed seed":
				receiver.Val = values.NewSlotValue(values.JavaNull, typ)
			case "missing source":
				delete(dumper.nativeLambdaLocalSources, name+desc)
			case "foreign method":
				copy := *site.method
				source.method = &copy
			case "foreign code":
				copy := *site.code
				source.code = &copy
			case "original snapshot", "snapshot wrong pc", "snapshot wrong ordinal", "snapshot parameter":
				var captured values.JavaValue = receiver
				if variant == "snapshot parameter" {
					captured = parameter
				}
				seed := values.NewSlotValue(captured, typ)
				snapshot := values.NewJavaRef(utils.NewRootVariableId(), seed, typ)
				pc, ordinal := site.pc, 0
				if variant == "snapshot wrong pc" {
					pc++
				}
				if variant == "snapshot wrong ordinal" {
					ordinal++
				}
				snapshot.MarkOriginalDynamicOperand(pc, ordinal, seed)
				snapshot.MarkOriginalDynamicOperandDeclaration(pc, seed)
				source.values[0] = snapshot
			case "missing consumption":
				delete(dumper.lambdaMethods, name)
			case "stub":
				body.bodyCode = "stub"
			}
			if got := nativeMemberLambdaSourceClosed(view, dumper, nil); got != (variant == "original" || variant == "original snapshot") {
				t.Fatalf("source closed=%v", got)
			}
		})
	}
}

func TestNativeAnonymousBoundLambdaFactoryRequiresOriginalReceiverOwnership(t *testing.T) {
	files := nativeCompileClasses(t, anonymousBoundLambdaReceiverFixture)
	variants := []string{"original", "no forest", "missing unit", "missing view", "foreign view", "cached site", "deleted original factory", "unused name type", "unused dynamic alias", "ordinary member alias", "condy alias", "wrong input", "static drift", "generic implementation", "missing resolver", "budget", "memory", "canceled"}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			_, packet, index := nativeBoundLambdaProofPacket(t, files)
			object := packet.object
			forest := &nativeAnonymousForest{units: map[string]*nativeAnonymousClass{object.GetClassName(): packet}}
			var work *workbudget.Budget
			view := packet.lambdaImplementation
			var method *MemberInfo
			for m := range view.lambdaImplementations {
				method = m
			}
			switch variant {
			case "no forest":
				forest = nil
			case "missing unit":
				delete(forest.units, object.GetClassName())
			case "missing view":
				packet.lambdaImplementation = nil
			case "foreign view":
				view.object = &ClassObject{}
			case "cached site", "deleted original factory":
				for _, sites := range view.lambdaContext.factorySites {
					for _, site := range sites {
						if variant == "deleted original factory" {
							site.code.Code[site.pc] = core.OP_NOP
						}
						site.pc = 65535
					}
				}
			case "unused name type":
				nt := *object.ConstantPool[index-1].(*ConstantNameAndTypeInfo)
				object.ConstantPool = append(object.ConstantPool, &nt)
				index = len(object.ConstantPool)
			case "unused dynamic alias", "condy alias":
				for _, cp := range object.ConstantPool {
					if dynamic, ok := cp.(*ConstantInvokeDynamicInfo); ok {
						if variant == "unused dynamic alias" {
							copy := *dynamic
							object.ConstantPool = append(object.ConstantPool, &copy)
						} else {
							object.ConstantPool = append(object.ConstantPool, &ConstantDynamicInfo{BootstrapMethodAttrIndex: dynamic.BootstrapMethodAttrIndex, NameAndTypeIndex: dynamic.NameAndTypeIndex})
						}
						break
					}
				}
			case "ordinary member alias":
				object.ConstantPool = append(object.ConstantPool, &ConstantMethodrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: object.ThisClass, NameAndTypeIndex: uint16(index)}})
			case "wrong input":
				nt := object.ConstantPool[index-1].(*ConstantNameAndTypeInfo)
				nt.DescriptorIndex = uint16(NewConstantPoolWithConstant(&object.ConstantPool).AddUtf8Info("(Ljava/lang/Object;)Ljava/util/function/Supplier;"))
			case "static drift":
				method.AccessFlags |= StaticFlag
			case "generic implementation":
				method.Attributes = append(method.Attributes, &SignatureAttribute{})
			case "missing resolver":
				view.lambdaContext.resolve = nil
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeAnonymousForestLambdaNameType(forest, object, index, work); got != (variant == "original" || variant == "cached site") {
				t.Fatalf("accepted=%v", got)
			}
		})
	}
}

func TestNativeAnonymousBoundLambdaHandleCannotBorrowCachedInvocationKind(t *testing.T) {
	files := nativeCompileClasses(t, anonymousBoundLambdaReceiverFixture)
	for _, variant := range []string{"original", "other bound kind", "static summary", "foreign referencer", "missing summary", "hidden extra handle", "duplicate summary hides ordinary handle"} {
		t.Run(variant, func(t *testing.T) {
			z, packet, _ := nativeBoundLambdaProofPacket(t, files)
			index := z.originalMemberIndex()
			owner := packet.object.GetClassName()
			if len(index.handleTargets[owner]) != 1 {
				t.Fatal("original single physical handle")
			}
			switch variant {
			case "other bound kind":
				kind := index.handleTargets[owner][0].kind
				if kind != 5 && kind != 7 {
					t.Fatal("original bound handle kind")
				}
				index.handleTargets[owner][0].kind = 12 - kind
			case "static summary":
				index.handleTargets[owner][0].kind = 6
			case "foreign referencer":
				index.handleTargets[owner][0].referencer = "BoundLambdaEffects"
			case "missing summary":
				index.handles[owner] = false
			case "hidden extra handle":
				for _, cp := range packet.object.ConstantPool {
					if handle, ok := cp.(*ConstantMethodHandleInfo); ok && handle.ReferenceKind == index.handleTargets[owner][0].kind {
						copy := *handle
						copy.ReferenceKind = 12 - copy.ReferenceKind
						packet.object.ConstantPool = append(packet.object.ConstantPool, &copy)
						break
					}
				}
			case "duplicate summary hides ordinary handle":
				var ordinary *MemberInfo
				for _, method := range packet.object.Methods {
					name, _ := sourceBridgeUTF8(packet.object, method.NameIndex)
					if name == "set" {
						ordinary = method
					}
				}
				if ordinary == nil {
					t.Fatal("original ordinary method")
				}
				nt := uint16(len(packet.object.ConstantPool) + 1)
				packet.object.ConstantPool = append(packet.object.ConstantPool, &ConstantNameAndTypeInfo{NameIndex: ordinary.NameIndex, DescriptorIndex: ordinary.DescriptorIndex})
				ref := uint16(len(packet.object.ConstantPool) + 1)
				packet.object.ConstantPool = append(packet.object.ConstantPool, &ConstantMethodrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: packet.object.ThisClass, NameAndTypeIndex: nt}})
				packet.object.ConstantPool = append(packet.object.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 5, ReferenceIndex: ref})
				index.handleTargets[owner] = append(index.handleTargets[owner], index.handleTargets[owner][0])
			}
			if got := nativeAnonymousLambdaHandlesClosed(packet, index, nil); got != (variant == "original") {
				t.Fatalf("accepted=%v", got)
			}
		})
	}
}

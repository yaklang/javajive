package javaclassparser

import (
	"context"
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

const nativeAnonymousLambdaProofFixture = `class AnonymousLambdaProofOwner {
 Object make(final Object capture){return new Object(){
  Object get(){java.util.function.Supplier<Object> supplier=()->capture;return supplier.get();}
 };}
} class AnonymousLambdaProofForeign {}`

func nativeAnonymousLambdaTestPacket(t *testing.T, files map[string][]byte) (*JarFS, *nativeAnonymousClass) {
	t.Helper()
	owner, e := Parse(append([]byte(nil), files["AnonymousLambdaProofOwner.class"]...))
	if e != nil {
		t.Fatal(e)
	}
	child, e := Parse(append([]byte(nil), files["AnonymousLambdaProofOwner$1.class"]...))
	if e != nil {
		t.Fatal(e)
	}
	z := nativeArchive(t, files)
	t.Cleanup(func() { z.Close() })
	d := z.nativeMemberReader(owner)
	p := d.nativeAnonymousConstructorForCompiler(child, owner.GetClassName(), "make(Ljava/lang/Object;)Ljava/lang/Object;", "", nil, nil, d.buildInvocationMetadata(), nil)
	if p == nil || p.lambdaImplementation == nil || len(p.lambdaImplementation.lambdaImplementations) != 1 {
		t.Fatal("original anonymous constructor/metafactory packet")
	}
	return z, p
}

func TestNativeAnonymousLambdaHandlesReproveOriginalTargetAndArchiveScope(t *testing.T) {
	base := nativeCompileClasses(t, nativeAnonymousLambdaProofFixture)
	for _, variant := range []string{"original", "no evidence", "foreign object", "empty targets", "missing handle index", "wrong target", "foreign lookup", "constructor handle", "field handle", "missing synthetic", "public helper", "ordinary capture field", "mutable capture field", "bootstrap arity", "missing resolver", "missing local source certificate", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z, p := nativeAnonymousLambdaTestPacket(t, base)
			index := z.originalMemberIndex()
			owner := p.object.GetClassName()
			view := p.lambdaImplementation
			var implementation *MemberInfo
			for method := range view.lambdaImplementations {
				implementation = method
			}
			var work *workbudget.Budget
			switch variant {
			case "no evidence":
				p.lambdaImplementation = nil
			case "foreign object":
				view.object = &ClassObject{}
			case "empty targets":
				index.handleTargets[owner] = nil
			case "missing handle index":
				delete(index.handles, owner)
			case "wrong target":
				index.handleTargets[owner][0].name = "ordinary"
			case "foreign lookup":
				index.handleTargets[owner][0].referencer = "AnonymousLambdaProofForeign"
			case "constructor handle":
				index.handleTargets[owner][0].kind = 8
			case "field handle":
				index.handleTargets[owner][0].methodRef = false
			case "missing synthetic":
				implementation.AccessFlags &^= 0x1000
			case "public helper":
				implementation.AccessFlags = 0x1009
			case "ordinary capture field", "mutable capture field":
				for _, field := range p.object.Fields {
					n, _ := sourceBridgeUTF8(p.object, field.NameIndex)
					if n == "val$capture" {
						if variant == "ordinary capture field" {
							field.AccessFlags &^= 0x1000
						} else {
							field.AccessFlags &^= 0x10
						}
					}
				}
			case "bootstrap arity":
				for _, attr := range p.object.Attributes {
					if b, ok := attr.(*BootstrapMethodsAttribute); ok {
						b.BootstrapMethods[0].NumBootstrapArguments++
					}
				}
			case "missing resolver":
				view.lambdaContext.resolve = nil
			case "missing local source certificate":
				view.lambdaContext.localCaptures = nil // Fresh proof must reconstruct it.
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			want := variant == "original" || variant == "missing local source certificate"
			if got := nativeAnonymousLambdaHandlesClosed(p, index, work) && z.nativeAnonymousLambdaChildArchiveClosed(p, index, work); got != want {
				t.Fatalf("closed=%v want=%v", got, want)
			}
		})
	}
}

func TestNativeAnonymousLambdaCapturedSourceCannotSubstituteSameTypedProducer(t *testing.T) {
	files := nativeCompileClasses(t, nativeAnonymousLambdaProofFixture)
	for _, variant := range []string{"original", "wrong read pc", "wrong field", "wrong receiver", "wrong receiver slot", "no receiver witness", "missing source", "foreign code", "foreign method", "wrong dynamic pc", "missing factory witness", "static parameter slot zero", "original snapshot", "snapshot wrong pc", "snapshot wrong position", "snapshot replaced seed", "missing consumption", "stub", "checked escape"} {
		t.Run(variant, func(t *testing.T) {
			_, p := nativeAnonymousLambdaTestPacket(t, files)
			view := p.lambdaImplementation
			var method *MemberInfo
			for m := range view.lambdaImplementations {
				method = m
			}
			name, _ := sourceBridgeUTF8(p.object, method.NameIndex)
			desc, _ := sourceBridgeUTF8(p.object, method.DescriptorIndex)
			site := view.lambdaContext.localCaptures[name+desc]
			if site == nil || len(site.operands) != 1 || site.operands[0].opcode != core.OP_GETFIELD {
				t.Fatal("original capture producer tree")
			}
			operand := site.operands[0]
			typ := types.NewJavaClass("java.lang.Object")
			receiver := values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, types.NewJavaClass(p.object.GetClassName()))
			receiver.IsParam = true
			receiver.IsThis = true
			receiver.MarkOriginalParameter(0)
			receiver.MarkOriginalReceiver()
			field := values.NewRefMember(receiver, operand.member, typ)
			field.OriginPC = operand.pc
			field.HasOriginPC = true
			field.MarkOriginalFieldRead(values.NewJavaClassMember(p.object.GetClassName(), operand.member, operand.descriptor, typ), operand.pc)
			source := &nativeLambdaLocalCaptureSource{method: site.method, code: site.code, factoryPC: site.pc, hasFactoryPC: true, values: []values.JavaValue{field}}
			dumper := NewClassObjectDumper(p.object)
			dumper.lambdaMethods[name] = []string{desc}
			body := &dumpedMethods{member: method, code: "return capture;", bodyCode: "return capture;"}
			dumper.dumpedMethodsSet[fmt.Sprintf("name:%s,desc:%s", name, desc)] = body
			dumper.nativeLambdaLocalSources = map[string]*nativeLambdaLocalCaptureSource{name + desc: source}
			switch variant {
			case "wrong read pc":
				field.OriginPC++
			case "wrong field":
				field.Member = "val$other"
			case "wrong receiver":
				field.Object = values.JavaNull
			case "wrong receiver slot":
				receiver.IsThis = false
			case "no receiver witness":
				receiver = values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, types.NewJavaClass(p.object.GetClassName()))
				receiver.IsThis = true
				field.Object = receiver
			case "missing source":
				delete(dumper.nativeLambdaLocalSources, name+desc)
			case "foreign code":
				copy := *site.code
				source.code = &copy
			case "foreign method":
				copy := *site.method
				source.method = &copy
			case "wrong dynamic pc":
				site.pc++
			case "missing factory witness":
				source.hasFactoryPC = false
			case "static parameter slot zero":
				parameter := values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, types.NewJavaClass(p.object.GetClassName()))
				parameter.IsParam = true
				parameter.MarkOriginalParameter(0)
				field.Object = parameter
			case "original snapshot", "snapshot wrong pc", "snapshot wrong position", "snapshot replaced seed":
				seed := values.NewSlotValue(field, typ)
				snapshot := values.NewJavaRef(utils.NewRootVariableId(), seed, typ)
				pc, index := site.pc, 0
				if variant == "snapshot wrong pc" {
					pc++
				}
				if variant == "snapshot wrong position" {
					index++
				}
				snapshot.MarkOriginalDynamicOperand(pc, index, seed)
				snapshot.MarkOriginalDynamicOperandDeclaration(pc, seed)
				if variant == "snapshot replaced seed" {
					snapshot.Val = values.NewSlotValue(field, typ)
				}
				source.values[0] = snapshot
			case "missing consumption":
				delete(dumper.lambdaMethods, name)
			case "stub":
				body.bodyCode = "stub"
			case "checked escape":
				body.checkedEscape = true
			}
			if got := nativeMemberLambdaSourceClosed(view, dumper, nil); got != (variant == "original" || variant == "original snapshot") {
				t.Fatalf("source closed=%v", got)
			}
		})
	}
}

func TestNativeAnonymousLambdaArchiveCannotGrantForeignLookupPrivilege(t *testing.T) {
	base := nativeCompileClasses(t, nativeAnonymousLambdaProofFixture)
	for _, variant := range []string{"original", "foreign method reference", "foreign handle", "unresolved physical user"} {
		t.Run(variant, func(t *testing.T) {
			files := map[string][]byte{}
			for n, raw := range base {
				files[n] = append([]byte(nil), raw...)
			}
			_, p := nativeAnonymousLambdaTestPacket(t, files)
			var target *MemberInfo
			for m := range p.lambdaImplementation.lambdaImplementations {
				target = m
			}
			if variant == "foreign method reference" || variant == "foreign handle" {
				foreign, e := Parse(files["AnonymousLambdaProofForeign.class"])
				if e != nil {
					t.Fatal(e)
				}
				cp := NewConstantPoolWithConstant(&foreign.ConstantPool)
				classIndex := uint16(cp.AddNewClassInfo(p.object.GetClassName()))
				n, _ := sourceBridgeUTF8(p.object, target.NameIndex)
				d, _ := sourceBridgeUTF8(p.object, target.DescriptorIndex)
				ni := nativeMemberLambdaTestUTF8(foreign, n)
				di := nativeMemberLambdaTestUTF8(foreign, d)
				foreign.ConstantPool = append(foreign.ConstantPool, &ConstantNameAndTypeInfo{NameIndex: ni, DescriptorIndex: di})
				nt := uint16(len(foreign.ConstantPool))
				foreign.ConstantPool = append(foreign.ConstantPool, &ConstantMethodrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: classIndex, NameAndTypeIndex: nt}})
				if variant == "foreign handle" {
					foreign.ConstantPool = append(foreign.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 6, ReferenceIndex: uint16(len(foreign.ConstantPool))})
				}
				files["AnonymousLambdaProofForeign.class"] = foreign.Bytes()
			}
			z, p := nativeAnonymousLambdaTestPacket(t, files)
			index := z.originalMemberIndex()
			if !index.valid {
				t.Fatal("original index")
			}
			if variant == "unresolved physical user" {
				index.typeUsers[p.object.GetClassName()]["MissingPhysicalUser"] = true
			}
			if got := z.nativeAnonymousLambdaChildArchiveClosed(p, index, nil); got != (variant == "original") {
				t.Fatalf("archive closed=%v", got)
			}
		})
	}
}

package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func anonymousLambdaParameterTestOriginal(t *testing.T) map[string][]byte {
	t.Helper()
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for original hidden-field parameter metadata")
	}
	return compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, nativeAnonymousLambdaProofFixture, "AnonymousLambdaProofOwner", "none", true)
}

func anonymousLambdaParameterTestDumper(t *testing.T, files map[string][]byte) (*ClassObjectDumper, *lambdaParameterMetadata, *nativeAnonymousClass) {
	t.Helper()
	z, child := nativeAnonymousLambdaTestPacket(t, files)
	d := z.nativeMemberReader(child.object)
	d.options = DecompileOptions{SourceCompiler: NativeJavac8, TargetSourceVersion: 8}
	d.nativeAnonymousLambdaCurrent = child.lambdaImplementation
	d.nativeCaptureFields = map[string]string{}
	d.nativeCapturedReads = map[string]map[int]string{}
	var method *MemberInfo
	for candidate := range child.lambdaImplementation.lambdaImplementations {
		method = candidate
	}
	name, _ := sourceBridgeUTF8(child.object, method.NameIndex)
	desc, _ := sourceBridgeUTF8(child.object, method.DescriptorIndex)
	site := child.lambdaImplementation.lambdaContext.localCaptures[name+desc]
	if site == nil || len(site.operands) != 1 || !lambdaParameterFieldCapture(site.operands[0]) {
		t.Fatal("original hidden-field factory")
	}
	operand := site.operands[0]
	parent, _ := sourceBridgeUTF8(child.object, site.method.NameIndex)
	parentDesc, _ := sourceBridgeUTF8(child.object, site.method.DescriptorIndex)
	// This query tests only the original field/source port. The actual typed
	// allocation and final emitted binding are exercised by the JVM MVPs.
	d.nativeCaptureFields[operand.member] = "capture"
	d.nativeCapturedReads[parent+parentDesc] = map[int]string{operand.pc: operand.member}
	p, known := d.originalLambdaParameterMetadata(method, nil)
	if !known || p == nil || len(p.captures) != 1 || p.captures[0].opcode != core.OP_GETFIELD || p.names[0] != "capture" || p.receiverWords != 0 {
		t.Fatal("original hidden capture/physical parameter partition")
	}
	return d, p, child
}

func TestAdversarialAnonymousLambdaParameterCaptureRequiresItsOriginalHiddenFieldPort(t *testing.T) {
	files := anonymousLambdaParameterTestOriginal(t)
	for _, variant := range []string{"original", "missing view", "foreign view", "foreign child", "detached view", "missing implementation", "ordinary field", "mutable field", "static field", "missing capture port", "wrong source binder", "missing read", "different read pc", "missing physical final", "different physical name", "foreign receiver", "static factory", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			d, p, child := anonymousLambdaParameterTestDumper(t, files)
			operand := p.captures[0]
			switch variant {
			case "missing view":
				d.nativeAnonymousLambdaCurrent = nil
			case "foreign view":
				child.lambdaImplementation.object = &ClassObject{}
			case "foreign child":
				child.object = &ClassObject{}
			case "detached view":
				child.lambdaImplementation = nil
			case "missing implementation":
				delete(child.lambdaImplementation.lambdaImplementations, p.method)
			case "ordinary field", "mutable field", "static field":
				for _, field := range d.obj.Fields {
					name, _ := sourceBridgeUTF8(d.obj, field.NameIndex)
					if name == operand.member {
						if variant == "ordinary field" {
							field.AccessFlags &^= 0x1000
						} else if variant == "mutable field" {
							field.AccessFlags &^= 0x10
						} else {
							field.AccessFlags |= 8
						}
					}
				}
			case "missing capture port":
				delete(child.fields, operand.member)
			case "wrong source binder":
				d.nativeCaptureFields[operand.member] = "different"
			case "missing read":
				d.nativeCapturedReads = nil
			case "different read pc":
				for _, reads := range d.nativeCapturedReads {
					delete(reads, operand.pc)
					reads[operand.pc+1] = operand.member
				}
			case "missing physical final":
				binary.BigEndian.PutUint16(p.table.Info[3:5], 0x1000)
			case "different physical name":
				binary.BigEndian.PutUint16(p.table.Info[1:3], sourceBridgePoolString(t, d.obj, "different"))
			case "foreign receiver":
				p.factory.code.Code[operand.operands[0].pc] = core.OP_ALOAD_1
			case "static factory":
				p.factory.method.AccessFlags |= 8
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := d.obj.Bytes()
			fresh, known := d.originalLambdaParameterMetadata(p.method, d.Work)
			if (known && fresh != nil) != (variant == "original") || !bytes.Equal(before, d.obj.Bytes()) {
				t.Fatalf("original field projection=%v known=%v", fresh != nil, known)
			}
		})
	}
}

func TestAdversarialAnonymousLambdaParameterPendingFieldIsNotCompletedSource(t *testing.T) {
	files := anonymousLambdaParameterTestOriginal(t)
	for _, variant := range []string{"original", "renamed snapshot", "wrong field pc", "wrong field", "foreign receiver", "unsealed receiver", "wrong dynamic pc", "wrong operand ordinal", "changed seed", "already declared", "already inlined", "wrong allocation binder", "foreign factory", "work"} {
		t.Run(variant, func(t *testing.T) {
			d, p, _ := anonymousLambdaParameterTestDumper(t, files)
			operand := p.captures[0]
			receiver := values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, types.NewJavaClass(d.obj.GetClassName()))
			receiver.IsParam, receiver.IsThis = true, true
			if variant != "unsealed receiver" {
				receiver.MarkOriginalParameter(0)
				receiver.MarkOriginalReceiver()
			}
			typ := types.NewJavaClass("java.lang.Object")
			field := values.NewRefMember(receiver, operand.member, typ)
			field.OriginPC, field.HasOriginPC = operand.pc, true
			field.MarkOriginalFieldRead(values.NewJavaClassMember(d.obj.GetClassName(), operand.member, operand.descriptor, typ), operand.pc)
			if variant == "wrong field pc" {
				field.OriginPC++
			} else if variant == "wrong field" {
				field.Member = "other"
			} else if variant == "foreign receiver" {
				field.Object = values.JavaNull
			}
			snapshot := values.NewJavaRef(utils.NewRootVariableId(), field, typ.Copy())
			pc, index := p.factory.pc, 0
			if variant == "wrong dynamic pc" {
				pc++
			} else if variant == "wrong operand ordinal" {
				index++
			}
			snapshot.MarkOriginalDynamicOperand(pc, index, field)
			switch variant {
			case "renamed snapshot":
				snapshot.Id.SetName("aNameIsNotAnEvaluationReceipt")
			case "changed seed":
				snapshot.Val = values.NewSlotValue(field, typ)
			case "already declared":
				snapshot.MarkOriginalDynamicOperandDeclaration(pc, field)
			case "already inlined":
				snapshot.StackVar = field
				snapshot.MarkOriginalDynamicOperandInline(pc, field)
			case "wrong allocation binder":
				d.nativeCaptureFields[operand.member] = "other"
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			d.CurrentMethod = p.factory.method
			name, _ := sourceBridgeUTF8(d.obj, p.factory.method.NameIndex)
			desc, _ := sourceBridgeUTF8(d.obj, p.factory.method.DescriptorIndex)
			d.FuncCtx = &class_context.ClassContext{ClassName: d.obj.GetClassName(), FunctionName: name, CurrentMethodDesc: desc}
			d.nativeLambdaBodyCapturedValues = []values.JavaValue{snapshot}
			request := methodSourceRequest{factory: p.factory, captureCount: 1}
			if variant == "foreign factory" {
				copy := *p.factory
				copy.pc++
				request.factory = &copy
			}
			_, captures, known := d.lambdaParameterCaptureContext(p, request)
			want := variant == "original" || variant == "renamed snapshot"
			if known != want || known && (len(captures) != 1 || captures[0] != snapshot) {
				t.Fatalf("pending original field=%v want=%v", known, want)
			}
		})
	}
}

func TestAdversarialCapturedFieldPublicationKeepsItsOriginalMethodNamespace(t *testing.T) {
	files := anonymousLambdaParameterTestOriginal(t)
	for _, variant := range []string{"same method", "sibling method", "lambda implementation", "foreign snapshot owner", "foreign snapshot method"} {
		t.Run(variant, func(t *testing.T) {
			d, p, _ := anonymousLambdaParameterTestDumper(t, files)
			operand := p.captures[0]
			d.FuncCtx = &class_context.ClassContext{ClassName: d.obj.GetClassName()}
			d.FuncCtx.FunctionName, _ = sourceBridgeUTF8(d.obj, p.factory.method.NameIndex)
			d.FuncCtx.CurrentMethodDesc, _ = sourceBridgeUTF8(d.obj, p.factory.method.DescriptorIndex)
			d.wireNativeAnonymousSource()
			receiver := values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, types.NewJavaClass(d.obj.GetClassName()))
			receiver.IsParam, receiver.IsThis = true, true
			receiver.MarkOriginalParameter(0)
			receiver.MarkOriginalReceiver()
			typ := types.NewJavaClass("java.lang.Object")
			field := values.NewRefMember(receiver, operand.member, typ)
			field.OriginPC, field.HasOriginPC = operand.pc, true
			field.MarkOriginalFieldRead(values.NewJavaClassMember(d.obj.GetClassName(), operand.member, operand.descriptor, typ), operand.pc)
			if field.String(d.FuncCtx) != "capture" {
				t.Fatal("current method's original capture binder")
			}
			saved, known := d.FuncCtx.SnapshotForSourceProof(nil)
			if !known {
				t.Fatal("source method snapshot")
			}
			if variant == "sibling method" {
				d.FuncCtx.FunctionName, d.FuncCtx.CurrentMethodDesc = "sibling", "()V"
			} else if variant == "lambda implementation" {
				d.FuncCtx.FunctionName, _ = sourceBridgeUTF8(d.obj, p.method.NameIndex)
				d.FuncCtx.CurrentMethodDesc, _ = sourceBridgeUTF8(d.obj, p.method.DescriptorIndex)
			} else if variant == "foreign snapshot owner" {
				saved.ClassName = "ForeignOwner"
			} else if variant == "foreign snapshot method" {
				saved.FunctionName = "differentMethod"
			}
			want := variant != "foreign snapshot owner" && variant != "foreign snapshot method"
			if text := field.String(saved); (text == "capture") != want || d.nativeCaptureFailed == want {
				t.Fatalf("publication borrowed live method namespace: %q failed=%v", text, d.nativeCaptureFailed)
			}
		})
	}
}

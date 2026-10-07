package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeAnonymousSharedParametersRequireOriginalUnchangedUses(t *testing.T) {
	files := anonymousSharedParameterClasses(t, anonymousSharedParameterFixture, "SharedParameterOwner", "none", map[string]bool{"LSharedParameterOwner;": true, "Ljava/lang/Object;": true, "J": true})
	for _, variant := range []string{"original", "ordinary capture", "mutable capture", "duplicate field", "foreign receiver", "wrong capture width", "changed capture parameter", "parameter overwritten", "duplicate capture store", "repeated SUPER operand", "missing SUPER operand", "foreign SUPER", "ordinary SUPER method", "unaccounted parameter", "post-SUPER parameter read", "handler", "small stack", "small locals", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, e := Parse(append([]byte(nil), files["SharedParameterOwner$1.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			var ctor *MemberInfo
			var code *CodeAttribute
			for _, m := range obj.Methods {
				if n, _ := sourceBridgeUTF8(obj, m.NameIndex); n == "<init>" {
					ctor = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if code == nil || ctor == nil {
				t.Fatal("original constructor")
			}
			ops, known := nativeEnumMethodOps(obj, ctor, nil)
			if !known {
				t.Fatal("original instructions")
			}
			var capture *MemberInfo
			store, super := -1, -1
			for _, f := range obj.Fields {
				if n, _ := sourceBridgeUTF8(obj, f.NameIndex); n == "val$keptToken" {
					capture = f
				}
			}
			for i, op := range ops {
				if member := constructorMotionMember(obj, op, core.OP_PUTFIELD); member != nil && member.Member == "val$keptToken" {
					store = i
				}
				if member := constructorMotionMember(obj, op, core.OP_INVOKESPECIAL); member != nil && member.Member == "<init>" {
					super = i
				}
			}
			if capture == nil || store < 2 || super < 3 {
				t.Fatal("original capture/SUPER")
			}
			var work *workbudget.Budget
			switch variant {
			case "ordinary capture":
				capture.AccessFlags &^= 0x1000
			case "mutable capture":
				capture.AccessFlags &^= 16
			case "duplicate field":
				obj.Fields = append(obj.Fields, capture)
			case "foreign receiver":
				code.Code[ops[store-2].CurrentOffset] = core.OP_ALOAD_1
			case "wrong capture width":
				code.Code[ops[store-1].CurrentOffset] = core.OP_ILOAD
			case "changed capture parameter":
				code.Code[ops[store-1].CurrentOffset+1] = 1
			case "parameter overwritten":
				code.Code[ops[store-1].CurrentOffset] = core.OP_ASTORE
			case "duplicate capture store":
				start, end := int(ops[store-2].CurrentOffset), int(ops[store].CurrentOffset)+3
				packet := append([]byte(nil), code.Code[start:end]...)
				code.Code = append(packet, code.Code...)
			case "repeated SUPER operand":
				code.Code[ops[super-2].CurrentOffset+1] = 1
			case "missing SUPER operand":
				start := int(ops[super-1].CurrentOffset)
				code.Code = append(code.Code[:start], code.Code[start+2:]...)
			case "foreign SUPER", "ordinary SUPER method":
				ref := obj.ConstantPool[core.Convert2bytesToInt(ops[super].Data)-1].(*ConstantMethodrefInfo)
				if variant == "foreign SUPER" {
					ref.ClassIndex = obj.ThisClass
				} else {
					nt := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
					nt.NameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("ordinary"))
				}
			case "unaccounted parameter":
				ctor.DescriptorIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("(LSharedParameterOwner;Ljava/lang/Object;JJ)V"))
			case "post-SUPER parameter read":
				code.Code = append(code.Code[:len(code.Code)-1], core.OP_LLOAD, 3, core.OP_POP2, core.OP_RETURN)
			case "handler":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 1, HandlerPc: 0}}
			case "small stack":
				code.MaxStack = 1
			case "small locals":
				code.MaxLocals = 1
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			owner, method, known := originalAnonymousOwner(obj)
			if !known {
				t.Fatal("original anonymous ownership")
			}
			plan := nativeAnonymousConstructor(obj, owner, method, work)
			if (plan != nil) != (variant == "original") {
				t.Fatalf("accepted=%v", plan != nil)
			}
			if plan != nil && (!plan.sharedParameterRoles || plan.descriptor != "(LSharedParameterOwner;Ljava/lang/Object;J)V" || len(plan.superParams) != 3 || plan.superParams[0] != 0 || plan.superParams[1] != 1 || plan.superParams[2] != 2 || plan.fields["this$0"] != 0 || plan.fields["val$keptToken"] != 1 || plan.fields["val$keptWord"] != 2) {
				t.Fatal("physical operand identities were replaced")
			}
		})
	}
}

func TestNativeAnonymousSharedParameterReportsGeneratedConstructorDifference(t *testing.T) {
	files := anonymousSharedParameterClasses(t, anonymousSharedParameterFixture, "SharedParameterOwner", "none", map[string]bool{"LSharedParameterOwner;": true, "Ljava/lang/Object;": true, "J": true})
	z := nativeArchive(t, files)
	defer z.Close()
	root, e := Parse(append([]byte(nil), files["SharedParameterOwner.class"]...))
	if e != nil {
		t.Fatal(e)
	}
	d := z.nativeMemberReader(root)
	d.nativeAnonymousRoot = d.planNativeAnonymousFamily()
	if d.nativeAnonymousRoot == nil {
		t.Fatal("original committed source plan")
	}
	r := &DecompileResult{}
	d.report = r
	source, e := d.DumpClass()
	if e != nil || strings.Contains(source, DecompileStubMarker) || !strings.Contains(source, "new SharedParameterBase") {
		t.Fatalf("source:%v\n%s", e, source)
	}
	found := false
	for _, d := range r.Diagnostics {
		if d.Code == "anonymous_shared_parameter_metadata" && d.Method == "SharedParameterOwner$1.<init>(LSharedParameterOwner;Ljava/lang/Object;J)V" && strings.Contains(d.Message, "physical descriptor") {
			found = true
		}
	}
	if !found {
		t.Fatalf("hidden constructor representation difference was not reported: %+v", r.Diagnostics)
	}
}

func TestNativeAnonymousSharedParameterSourceRefusesUnstableCapture(t *testing.T) {
	files := anonymousSharedParameterClasses(t, anonymousSharedParameterFixture, "SharedParameterOwner", "none", map[string]bool{"LSharedParameterOwner;": true, "Ljava/lang/Object;": true, "J": true})
	for _, variant := range []string{"effectful reference", "effectful wide", "ordinary enclosing parameter"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, e := Parse(append([]byte(nil), files["SharedParameterOwner.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeAnonymousFamily()
			if p == nil || p.children["SharedParameterOwner$1"] == nil {
				t.Fatal("proved original ownership")
			}
			d.nativeAnonymousRoot = p
			d.FuncCtx = &class_context.ClassContext{ClassName: "SharedParameterOwner", FunctionName: "make"}
			d.wireNativeAnonymousSource()
			child := p.children["SharedParameterOwner$1"]
			args := []class_context.SourceCaptureOperand{{Receiver: true, Text: "this"}, {Local: true, Text: "keptToken"}, {Local: true, Text: "keptWord"}}
			switch variant {
			case "effectful reference":
				args[1] = class_context.SourceCaptureOperand{Text: "effect()"}
			case "effectful wide":
				args[2] = class_context.SourceCaptureOperand{Text: "effect()"}
			case "ordinary enclosing parameter":
				args[0] = class_context.SourceCaptureOperand{Local: true, Text: "sameTypedParameter"}
			}
			if _, accepted := d.FuncCtx.SourceAnonymousAllocation(child.object.GetClassName(), child.descriptor, child.newPC, child.invokePC, args); accepted || !p.failed {
				t.Fatal("unstable or nonlexical word acquired a shared capture")
			}
		})
	}
}

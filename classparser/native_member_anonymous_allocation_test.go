package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeAnonymousMemberAllocationRequiresExactLexicalRead(t *testing.T) {
	files := nativeCompileClasses(t, anonymousMemberAllocationFixture)
	for _, variant := range []string{"original", "unique local", "foreign receiver", "wrong result type", "wrong receiver type", "missing field origin", "wrong field origin", "wrong field", "missing forest", "foreign forest", "foreign object", "failed group", "copied read", "wrong method", "missing lexical role", "captured parameter role", "cyclic read", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			archive := nativeArchive(t, files)
			defer archive.Close()
			root, err := Parse(files["AllocationOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			dumper := archive.nativeMemberReader(root)
			family := dumper.planNativeMemberFamily()
			if family == nil || !dumper.planNativeMemberAnonymousScopes(family) || family.anonymousForest == nil {
				t.Fatal("complete original lexical forest")
			}
			current := "AllocationOwner$1"
			forest := family.anonymousForest
			object := forest.units[current].object
			plans, known := archive.nativeMemberReader(object).nativeMemberAllocations(family)
			if !known {
				t.Fatal("original allocation packet")
			}
			var plan *nativeMemberAllocation
			for _, candidate := range plans["read(I)LAllocationOwner$Entry;"] {
				plan = candidate
			}
			if plan == nil || plan.anonymousEnclosingRead == nil {
				t.Fatal("original named member enclosing read")
			}
			read := plan.anonymousEnclosingRead
			receiver := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaClass(current))
			receiver.IsThis = true
			field := values.NewRefMember(receiver, read.field, types.NewJavaClass("AllocationOwner"))
			field.HasOriginPC = true
			field.OriginPC = read.pc
			call := &values.FunctionCallExpression{ClassName: plan.child.object.GetClassName(), FunctionName: "<init>", Descriptor: plan.descriptor, HasOriginPC: true, OriginPC: plan.invokePC, Arguments: []values.JavaValue{field}}
			allocation := &values.NewExpression{JavaType: types.NewJavaClass(call.ClassName), ConstructorCall: call, HasOriginPC: true, OriginPC: plan.newPC}
			use := &statements.ReturnStatement{JavaValue: allocation}
			body := []statements.Statement{use}
			var operand any = field
			var work *workbudget.Budget
			switch variant {
			case "unique local":
				local := values.NewJavaRef(coreutils.NewRootVariableId(), nil, field.Type())
				local.Id.SetName("outer")
				body = append([]statements.Statement{statements.NewAssignStatement(local, field, true)}, body...)
				operand = local
				call.Arguments[0] = local
			case "foreign receiver":
				receiver.IsThis = false
			case "wrong result type":
				field.JavaType = types.NewJavaClass("java/lang/Object")
			case "wrong receiver type":
				wrong := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaClass("AllocationOwner"))
				wrong.IsThis = true
				field.Object = wrong
			case "missing field origin":
				field.HasOriginPC = false
			case "wrong field origin":
				field.OriginPC++
			case "wrong field":
				field.Member = "val$other"
			case "missing forest":
				family.anonymousForest = nil
			case "foreign forest":
				forest.members = &nativeMemberFamily{}
			case "foreign object":
				forest.objects[current] = root
			case "failed group":
				family.anonymousUnits[current].failed = true
			case "copied read":
				copy := *read
				forest.reads[current][plan.anonymousEnclosingMethod][read.pc] = &copy
			case "wrong method":
				plan.anonymousEnclosingMethod = "read()V"
			case "missing lexical role":
				delete(forest.lexicalThis[current][plan.anonymousEnclosingMethod], read.pc)
			case "captured parameter role":
				read.parameterOwner = "AllocationOwner"
			case "cyclic read":
				read.prior = read
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			want := variant == "original" || variant == "unique local"
			if got := nativeMemberLexicalEnclosingOperand(operand, plan, family, current, body, work); got != want {
				t.Fatalf("enclosing proof=%v want=%v", got, want)
			}
		})
	}
}

// The same real anonymous class may supply a constructor-access marker and
// the nested child's enclosing instance. These are distinct descriptor roles;
// permitting one must neither erase the other nor admit an unrelated alias.
func TestNativeAnonymousMemberAllocationComposesBridgeAndCaptureRoles(t *testing.T) {
	files := nativeCompileClasses(t, anonymousNestedMemberAllocationFixture())
	for _, variant := range []string{"original", "missing forest", "foreign forest", "missing marker unit", "foreign group", "field method alias", "foreign field alias", "dynamic capture tuple", "invokedynamic capture tuple", "ordinary declaration", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, e := Parse(files["AllocationOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) || p.anonymousForest == nil {
				t.Fatal("original complete lexical forest unavailable")
			}
			f := p.anonymousForest
			leaf := f.objects["AllocationOwner$1$1"]
			if leaf == nil {
				t.Fatal("nested original object unavailable")
			}
			var field *ConstantFieldrefInfo
			for _, c := range leaf.ConstantPool {
				if ref, ok := c.(*ConstantFieldrefInfo); ok {
					nt, ok := leaf.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
					if !ok {
						continue
					}
					desc, _ := sourceBridgeUTF8(leaf, nt.DescriptorIndex)
					if desc == "LAllocationOwner$1;" {
						field = ref
						break
					}
				}
			}
			if field == nil {
				t.Fatal("original anonymous parent capture unavailable")
			}
			var work *workbudget.Budget
			switch variant {
			case "missing forest":
				p.anonymousForest = nil
			case "foreign forest":
				f.members = &nativeMemberFamily{}
			case "missing marker unit":
				delete(f.units, "AllocationOwner$1")
			case "foreign group":
				p.anonymousUnits["AllocationOwner$1$1"].forest = &nativeAnonymousForest{}
			case "field method alias":
				leaf.ConstantPool = append(leaf.ConstantPool, &ConstantMethodrefInfo{ConstantMemberrefInfo: field.ConstantMemberrefInfo})
			case "foreign field alias":
				copy := *field
				copy.ClassIndex = uint16(NewConstantPoolWithConstant(&leaf.ConstantPool).AddNewClassInfo("Foreign"))
				leaf.ConstantPool = append(leaf.ConstantPool, &copy)
			case "dynamic capture tuple":
				leaf.ConstantPool = append(leaf.ConstantPool, &ConstantDynamicInfo{NameAndTypeIndex: field.NameAndTypeIndex})
			case "invokedynamic capture tuple":
				leaf.ConstantPool = append(leaf.ConstantPool, &ConstantInvokeDynamicInfo{NameAndTypeIndex: field.NameAndTypeIndex})
			case "ordinary declaration":
				nt := leaf.ConstantPool[field.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				cp := NewConstantPoolWithConstant(&leaf.ConstantPool)
				leaf.Fields = append(leaf.Fields, &MemberInfo{NameIndex: uint16(cp.AddUtf8Info("unrelated")), DescriptorIndex: nt.DescriptorIndex})
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := z.nativeMemberJointBridgeReferencesClosed(p, z.originalMemberIndex(), work)
			if got != (variant == "original") {
				t.Fatalf("composed original capture/bridge accepted=%v", got)
			}
		})
	}
}

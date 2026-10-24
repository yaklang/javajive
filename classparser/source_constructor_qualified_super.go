package javaclassparser

import (
	"crypto/sha256"
	"reflect"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A flat physical child can inherit a still-native member declaration. Source
// spells its hidden enclosing port as qualified-super; javac adds a null check
// that the original implicit enclosing argument did not perform. Only an
// explicit reconstruction artifact may remove that extra fresh check.
// JLS 8.8.7.1 specifies the additional qualified-super null check:
// https://docs.oracle.com/javase/specs/jls/se21/html/jls-8.html#jls-8.8.7.1
type constructorLoweringQualifiedSuper struct {
	owner, descriptor string
	parentHash        [32]byte
	dependencies      map[string][32]byte
}

type constructorLoweringQualifierGuard struct {
	start, end int
	noop       bool
}

func constructorLoweringQualifiedSuperFor(parent *nativeMemberClass, plan *sourceConstructorLoweringPlan, work *workbudget.Budget) *constructorLoweringQualifiedSuper {
	if plan == nil || parent == nil || parent.static || parent.object == nil || parent.object.GetClassName() != plan.super || len(parent.accessBridges) != 0 || len(plan.arguments) == 0 || plan.arguments[0].slot <= 0 || !nativeProofWork(work, 1) {
		return nil
	}
	ctor := parent.constructors[plan.superDescriptor]
	args, result, err := callbinding.Descriptor(plan.superDescriptor)
	if ctor == nil || err != nil || result != "V" || len(args) == 0 || args[0] != "L"+parent.owner+";" {
		return nil
	}
	projected := "("
	for _, arg := range args[1:] {
		projected += arg
	}
	projected += ")V"
	if ctor.sourceDescriptor != projected {
		return nil
	}
	// Generic enclosing/super views need an additional binder proof. This
	// source projection handles exact erased declaration ports only.
	sig, valid := nativeMethodLocalOriginalSignature(parent.object, parent.object.Attributes, work)
	if !valid || sig != "" {
		return nil
	}
	for _, method := range parent.object.Methods {
		name, _ := sourceBridgeUTF8(parent.object, method.NameIndex)
		desc, _ := sourceBridgeUTF8(parent.object, method.DescriptorIndex)
		if name == "<init>" && desc == plan.superDescriptor {
			sig, valid := nativeMethodLocalOriginalSignature(parent.object, method.Attributes, work)
			if !valid || sig != "" {
				return nil
			}
		}
	}
	raw := parent.object.Bytes()
	if !nativeProofWork(work, int64(len(raw))+1) {
		return nil
	}
	return &constructorLoweringQualifiedSuper{owner: parent.owner, descriptor: projected, parentHash: sha256.Sum256(raw)}
}

func (c *ClassObjectDumper) constructorLoweringQualifiedSuperView(plan *sourceConstructorLoweringPlan) *constructorLoweringQualifiedSuper {
	if c == nil || c.FuncCtx == nil || c.FuncCtx.DeclarationSourceName == nil {
		return nil
	}
	if _, known := c.FuncCtx.DeclarationSourceName(plan.super); !known {
		return nil
	}
	return c.constructorLoweringQualifiedSuperPhysical(plan)
}

func (c *ClassObjectDumper) constructorLoweringQualifiedSuperPhysical(plan *sourceConstructorLoweringPlan) *constructorLoweringQualifiedSuper {
	if c == nil || c.obj == nil || plan == nil || !nativeProofWork(c.Work, 1) {
		return nil
	}
	scope := &nativeMemberForeignPhysicalScope{objects: map[string]*ClassObject{}, originals: map[*ClassObject][32]byte{}, definitions: map[string][32]byte{}, resolve: c.nativeAnnotationDeclarationResolver(), work: c.Work}
	parent, known := scope.load(plan.super)
	if !known {
		return nil
	}
	owner, _, flags, known := originalMemberOwner(parent)
	if !known || flags&8 != 0 {
		return nil
	}
	outer, known := scope.load(owner)
	if !known {
		return nil
	}
	sig, valid := nativeMethodLocalOriginalSignature(outer, outer.Attributes, c.Work)
	if !valid || sig != "" {
		return nil
	}
	lexical, known := scope.lexical(outer)
	if !known {
		return nil
	}
	member := c.proveNativeMemberDeclaration(parent, outer, nil, lexical, scope.load)
	view := constructorLoweringQualifiedSuperFor(member, plan, c.Work)
	if view == nil || !scope.unchanged() || c.Work.CheckAlloc(int64(len(scope.originals)+1)*128) != nil {
		return nil
	}
	view.dependencies = map[string][32]byte{}
	for object, hash := range scope.originals {
		view.dependencies[object.GetClassName()] = hash
	}
	return view
}

func (a *sourceConstructorLoweringAuthority) constructorLoweringQualifiedSuperUnchanged(original *ClassObject, plan *sourceConstructorLoweringPlan, work *workbudget.Budget) bool {
	if plan.qualified == nil {
		return true
	}
	if a == nil || a.jar == nil {
		return false
	}
	reader := a.jar.nativeMemberReader(original)
	reader.Work = work
	return reflect.DeepEqual(plan.qualified, reader.constructorLoweringQualifiedSuperPhysical(plan))
}

func constructorLoweringExecutablePlanEqual(expected, actual *sourceConstructorLoweringPlan) bool {
	if expected == nil || actual == nil {
		return expected == actual
	}
	a, b := *expected, *actual
	a.qualified, b.qualified = nil, nil
	a.checkedSuperExceptions, b.checkedSuperExceptions = "", ""
	return reflect.DeepEqual(a, b)
}

func constructorLoweringReadQualifierGuard(object *ClassObject, ops []*core.OpCode, at int) (constructorLoweringQualifierGuard, int, bool) {
	if at < 0 || at >= len(ops) {
		return constructorLoweringQualifierGuard{}, 0, false
	}
	start := int(ops[at].CurrentOffset)
	if ops[at].Instr.OpCode == core.OP_NOP && at+5 <= len(ops) {
		for i := 0; i < 5; i++ {
			op := ops[at+i]
			if op.Instr.OpCode != core.OP_NOP || op.IsWide || len(op.Data) != 0 || int(op.CurrentOffset) != start+i {
				return constructorLoweringQualifierGuard{}, 0, false
			}
		}
		return constructorLoweringQualifierGuard{start, start + 5, true}, 5, true
	}
	if at+3 > len(ops) || ops[at].Instr.OpCode != core.OP_DUP || ops[at].IsWide || len(ops[at].Data) != 0 {
		return constructorLoweringQualifierGuard{}, 0, false
	}
	call, pop := ops[at+1], ops[at+2]
	if call.IsWide || len(call.Data) != 2 || int(call.CurrentOffset) != start+1 || pop.Instr.OpCode != core.OP_POP || pop.IsWide || len(pop.Data) != 0 || int(pop.CurrentOffset) != start+4 {
		return constructorLoweringQualifierGuard{}, 0, false
	}
	index := int(core.Convert2bytesToInt(call.Data))
	if index <= 0 || index > len(object.ConstantPool) {
		return constructorLoweringQualifierGuard{}, 0, false
	}
	if _, classMethod := object.ConstantPool[index-1].(*ConstantMethodrefInfo); !classMethod {
		return constructorLoweringQualifierGuard{}, 0, false
	}
	member := constructorMotionMember(object, call, core.OP_INVOKEVIRTUAL)
	known := member != nil && member.Name == "java/lang/Object" && member.Member == "getClass" && member.Description == "()Ljava/lang/Class;"
	if !known {
		member = constructorMotionMember(object, call, core.OP_INVOKESTATIC)
		known = member != nil && member.Name == "java/util/Objects" && member.Member == "requireNonNull" && member.Description == "(Ljava/lang/Object;)Ljava/lang/Object;"
	}
	if !known {
		return constructorLoweringQualifierGuard{}, 0, false
	}
	return constructorLoweringQualifierGuard{start, start + 5, false}, 3, true
}

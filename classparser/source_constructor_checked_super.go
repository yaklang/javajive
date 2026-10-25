package javaclassparser

import (
	"encoding/binary"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A JVM constructor need not declare its direct SUPER's checked exceptions.
// An independently emitted Java declaration does. This source-only contract
// belongs to the exact capture/SUPER/return packet and its returned artifact;
// FinishClass removes only the certified fresh Exceptions table. No original
// executable code, catch, or inferred caller declaration is substituted.
func constructorLoweringCheckedSuperExceptions(object *ClassObject, method *MemberInfo, plan *sourceConstructorLoweringPlan, metadata callbinding.Provider, work *workbudget.Budget) (string, bool) {
	if object == nil || method == nil || plan == nil || metadata == nil || plan.body || len(plan.captures) == 0 || !nativeProofWork(work, int64(len(object.Methods))+1) {
		return "", false
	}
	owned := 0
	for _, m := range object.Methods {
		if m == method {
			owned++
		}
	}
	if owned != 1 {
		return "", false
	}
	if _, present, valid := sourceAnonymousExceptionNames(object, method, work); !valid || present {
		return "", false
	}
	if _, present, valid := sourceMethodSignatureKey(object, method); !valid || present {
		return "", false
	}
	packet := constructorLoweringTypedPacketFor(object, method, true, true, work, metadata)
	if packet == nil || !constructorLoweringExecutablePlanEqual(plan, packet.plan) {
		return "", false
	}
	for _, argument := range packet.plan.arguments {
		if argument.invocation != nil || argument.checked != nil {
			return "", false // An eager argument call owns a separate throws boundary.
		}
	}
	parent, known := metadata(plan.super)
	if !known || parent.Name != plan.super || !parent.MembersComplete || parent.IsInterface || parent.Signature != "" || len(parent.Methods) > 4096 || !nativeProofWork(work, int64(len(parent.Methods))+1) {
		return "", false
	}
	var target callbinding.Method
	matches := 0
	for _, m := range parent.Methods {
		if m.Name == "<init>" && m.Desc == plan.superDescriptor {
			target, matches = m, matches+1
		}
	}
	if matches != 1 || target.Generic || target.Signature != "" || target.Static || target.Bridge || !target.ExceptionsKnown || len(target.Exceptions) > 128 {
		return "", false
	}
	var checked []string
	seen, text := map[string]bool{}, 0
	for _, exception := range target.Exceptions {
		text += len(exception) + 1
		if text > 8192 || seen[exception] || !nativeSourceBinaryName(exception) || !nativeProofWork(work, int64(len(exception))+1) || !sourceAnonymousExceptionHierarchyClosed(exception, metadata, work) {
			return "", false
		}
		seen[exception] = true
		if !originalExceptionUnchecked(exception, metadata) {
			checked = append(checked, exception)
		}
	}
	if len(checked) == 0 || work.CheckAlloc(int64(text)*2+int64(len(checked))*64) != nil {
		return "", false
	}
	return strings.Join(checked, "\x00"), true
}

func (c *ClassObjectDumper) constructorCheckedSuperSource(plan *constructorSourceBoundary, method *MemberInfo) ([]string, bool) {
	if c == nil || c.FuncCtx == nil || c.options.sourceConstructorLowering == nil || !c.reconstructionPhysicalHeader() || plan == nil || plan.lowering == nil || plan.lowering.checkedSuperExceptions == "" {
		return nil, false
	}
	exceptions, known := constructorLoweringCheckedSuperExceptions(c.obj, method, plan.lowering, c.FuncCtx.InvocationMetadata, c.Work)
	if !known || exceptions != plan.lowering.checkedSuperExceptions {
		return nil, false
	}
	return strings.Split(exceptions, "\x00"), true
}

// Work on the already verified fresh method span, after its Code permutation.
// Every byte outside this one method attribute remains untouched. Absence is
// accepted only on an independently verified already-lowered packet.
func constructorLoweringCheckedSuperMethodEdit(edit compiledMetadataEdit, object *ClassObject, method *MemberInfo, exceptions string, permute bool, work *workbudget.Budget) (compiledMetadataEdit, error) {
	if exceptions == "" {
		return edit, nil
	}
	fail := func() (compiledMetadataEdit, error) {
		return compiledMetadataEdit{}, constructorLoweringFailure(work, "fresh checked SUPER exception declaration changed")
	}
	actual, present, valid := sourceAnonymousExceptionNames(object, method, work)
	if !valid || present && actual != exceptions || !present && permute {
		return fail()
	}
	if _, present, valid := sourceMethodSignatureKey(object, method); !valid || present {
		return fail()
	}
	if !present {
		return edit, nil
	}
	raw := edit.data
	if len(raw) < 8 {
		return fail()
	}
	count := int(binary.BigEndian.Uint16(raw[6:]))
	for pos, index := 8, 0; index < count; index++ {
		if len(raw)-pos < 6 {
			return fail()
		}
		size := int64(binary.BigEndian.Uint32(raw[pos+2:]))
		if size > int64(len(raw)-pos-6) || !nativeProofWork(work, size+1) {
			return fail()
		}
		end := pos + 6 + int(size)
		name, known := sourceBridgeUTF8(object, binary.BigEndian.Uint16(raw[pos:]))
		if !known {
			return fail()
		}
		if name == "Exceptions" {
			edit.data = append(raw[:pos:pos], raw[end:]...)
			binary.BigEndian.PutUint16(edit.data[6:], uint16(count-1))
			return edit, nil
		}
		pos = end
	}
	return fail()
}

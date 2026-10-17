package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

type nativeLambdaCaptureNameDemand struct {
	site  *nativeLambdaLocalCaptureSite
	index int
	name  string
	value values.JavaValue
}

// An anonymous class nested in a lambda can capture a parameter of the lambda
// implementation which is itself a captured parameter of the original factory.
// Its field spelling constrains that outer source binder. Keep the LCAP value
// transport unchanged; never substitute another value or invent an alias.
// This initial domain admits only one proved original factory and its unchanged
// descriptor parameter LOAD. Local/field operands and shared factories retain
// their existing handling until they have a corresponding name-constraint proof.
func (c *ClassObjectDumper) recordNativeLambdaCaptureParameterName(ref *values.JavaRef, name string) bool {
	if c == nil || c.FuncCtx == nil || c.CurrentMethod == nil || !class_context.PreservesJava8Identifier(name) {
		return true
	}
	index, known := c.nativeLambdaParameterCaptureIndex(ref)
	if !known {
		return true
	}
	var sites []*nativeLambdaLocalCaptureSite
	key := c.FuncCtx.FunctionName + c.FuncCtx.CurrentMethodDesc
	if view := c.nativeLambdaSourceView(); view != nil {
		sites = view.lambdaContext.factorySites[key]
	}
	if len(sites) == 0 {
		// A root with only anonymous children need not own a named-member
		// transaction. Query the same original bootstrap graph without granting
		// it membership, access or any source publication permission.
		context := nativeLambdaImplementationContext{
			resolve: c.nativeAnnotationDeclarationResolver(), metadata: c.FuncCtx.InvocationMetadata,
			localCaptures: map[string]*nativeLambdaLocalCaptureSite{},
			factorySites:  map[string][]*nativeLambdaLocalCaptureSite{},
		}
		if !nativeLambdaImplementationScope(c.obj, c.FuncCtx.FunctionName, c.FuncCtx.CurrentMethodDesc, "", false, c.Work, context) {
			return true
		}
		sites = context.factorySites[key]
	}
	if len(sites) != 1 {
		return true
	}
	site := sites[0]
	if site == nil || site.method == nil || site.code == nil || site.method == c.CurrentMethod || index < 0 || index >= len(site.operands) {
		return true
	}
	if site.method.AccessFlags&0x1000 != 0 || index >= len(c.nativeLambdaBodyCapturedValues) {
		return true
	}
	operand := site.operands[index]
	if operand == nil || operand.local != nil || operand.owner != "" || operand.member != "" || operand.storage != nil || len(operand.operands) != 0 || operand.slot < 0 {
		return true
	}
	// Factory-site planning already proves the descriptor LOAD, unchanged parameter
	// words, exact bootstrap/implementation and exclusive use. Do not derive any
	// of those identities from a source name or a matching value type here.
	if !nativeProofWork(c.Work, 1) || c.Work != nil && c.Work.CheckAlloc(256) != nil {
		return false
	}
	if len(c.nativeLambdaCaptureParameterNames) > 4096 || len(c.nativeLambdaCaptureParameterNames[site.method]) >= 64 {
		return false
	}
	if c.nativeLambdaCaptureParameterNames == nil {
		c.nativeLambdaCaptureParameterNames = map[*MemberInfo][]nativeLambdaCaptureNameDemand{}
	}
	for _, prior := range c.nativeLambdaCaptureParameterNames[site.method] {
		if prior.site == site && prior.index == index {
			return prior.name == name && prior.value == c.nativeLambdaBodyCapturedValues[index]
		}
	}
	c.nativeLambdaCaptureParameterNames[site.method] = append(c.nativeLambdaCaptureParameterNames[site.method], nativeLambdaCaptureNameDemand{site: site, index: index, name: name, value: c.nativeLambdaBodyCapturedValues[index]})
	return true
}

// Bind the retained factory's actual source operand. Stack simulation may have
// materialized a separate dynamic snapshot; naming the original JVM parameter
// would leave that snapshot (and hence the generated capture field) unchanged.
func (c *ClassObjectDumper) bindNativeLambdaCaptureParameterNames(v values.JavaValue, body []statements.Statement, params, visible map[*coreutils.VariableId]bool, names *nativeCaptureNameBindings) bool {
	lambda, ok := v.(*values.CustomValue)
	if !ok || lambda == nil || lambda.Flag != "lambda" || !lambda.HasOriginPC {
		return true
	}
	for _, demand := range c.nativeLambdaCaptureParameterNames[c.CurrentMethod] {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		if demand.site == nil || demand.site.method != c.CurrentMethod || lambda.OriginPC != demand.site.pc {
			continue
		}
		if lambda.IsMethodRef || !lambda.CapturesKnown || demand.index < 0 || demand.index >= len(demand.site.operands) || demand.index >= len(lambda.Captures) || lambda.Captures[demand.index] != demand.value {
			return false
		}
		value, known := nativeMemberEnclosingUnpack(demand.value, c.Work)
		ref, knownRef := value.(*values.JavaRef)
		if !known || !knownRef || ref == nil || ref.Id == nil || ref.IsThis || !visible[ref.Id] {
			return false
		}
		declaration, stable := nativeCaptureDeclaration(body, ref, params[ref.Id], c.Work)
		if !stable {
			return false
		}
		if !params[ref.Id] {
			if declaration == nil {
				return false
			}
			owner, ok := declaration.LeftValue.(*values.JavaRef)
			if !ok || !owner.OriginalDynamicOperandDeclarationOf(ref, declaration.JavaValue) {
				return false
			}
			pc, index, sealed := ref.OriginalDynamicOperandWitness(ref.Val)
			if !sealed || pc != demand.site.pc || index != demand.index {
				return false
			}
		} else {
			slot, sealed := ref.OriginalParameterSlot()
			if !sealed || slot != demand.site.operands[demand.index].slot {
				return false
			}
		}
		if prior, known := c.FuncCtx.LocalNames[ref.Id]; known && prior != demand.name {
			return false
		}
		if !names.bind(demand.name, ref.Id, visible, c.Work) {
			return false
		}
		c.FuncCtx.LocalNames[ref.Id] = demand.name
	}
	return true
}

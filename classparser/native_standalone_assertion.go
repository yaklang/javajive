package javaclassparser

import (
	"fmt"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// A flat top-level declaration still owns its assertion-status protocol.
// Emitting the compiler flag as an ordinary source field loses ACC_SYNTHETIC
// and can collide with javac's assertion support in a derived lexical scope.
// Reuse the physical packet proof, but publish only after every original read
// has a complete typed source projection. Failure retries the untouched flat
// representation in a fresh dumper; no partially projected cache is reused.
type nativeStandaloneAssertion struct {
	packet   *nativeMemberAssertion
	consumed map[string]bool
	report   *DecompileResult
}

func (c *ClassObjectDumper) prepareNativeStandaloneAssertion() {
	if c.nativeStandaloneAssertionOff || c.nativeStandaloneAssertion != nil || c.nativeAssertionProtocol() != nil || c.nativeCaptureFields != nil || c.nativeMethodLocalCurrent != nil || c.nativeMemberCurrent != nil || c.nativeEnumConstantCurrent != nil || c.obj == nil || c.obj.AccessFlags&0x6200 != 0 || c.options.TargetSourceVersion != 0 && c.options.TargetSourceVersion != 8 {
		return
	}
	// Avoid a whole-method packet scan for classes without an assertion flag.
	present := false
	for _, field := range c.obj.Fields {
		if field == nil || !nativeProofWork(c.Work, 1) {
			return
		}
		name, known := sourceBridgeUTF8(c.obj, field.NameIndex)
		if !known {
			return
		}
		present = present || name == nativeAssertionField
	}
	if !present || !nativeMemberTopLevelEvidence(c.obj, c.Work) {
		return
	}
	packet, known := nativeMemberAssertionProof(c.obj, c.obj.GetClassName(), c.Work)
	if !known || packet == nil || !packet.pureInitializer {
		return
	}
	// Lambda and bridge bodies are not ordinary source declarations. Their
	// lexical status owner requires a separate source-scope certificate.
	for _, method := range c.obj.Methods {
		if method == nil || !nativeProofWork(c.Work, 1) {
			return
		}
		name, nk := sourceBridgeUTF8(c.obj, method.NameIndex)
		desc, dk := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
		if !nk || !dk || len(packet.reads[name+desc]) != 0 && method.AccessFlags&0x1040 != 0 {
			return
		}
	}
	state := &nativeStandaloneAssertion{packet: packet, consumed: map[string]bool{}}
	if c.report != nil {
		saved := *c.report
		state.report = &saved
	}
	c.nativeStandaloneAssertion = state
	c.nativeSourceAssertions = packet
}

func (c *ClassObjectDumper) nativeStandaloneAssertionClosed(source string) bool {
	state := c.nativeStandaloneAssertion
	if state == nil {
		return true
	}
	if strings.Contains(source, DecompileStubMarker) || strings.Contains(source, values.EmptySlotValuePlaceholder) {
		return false
	}
	for method, reads := range state.packet.reads {
		if !nativeProofWork(c.Work, 1) || len(reads) != 0 && !state.consumed[method] {
			return false
		}
	}
	return true
}

func (c *ClassObjectDumper) retryWithoutStandaloneAssertion() (string, error) {
	// A lexical transaction must reject its entire source certificate. Its
	// archive fallback creates a separate flat dumper; retaining half a family
	// here would discard its original ownership and registration schedule.
	if c.nativeMemberRoot != nil || c.nativeAnonymousRoot != nil {
		return "", fmt.Errorf("top-level assertion source closure unproved")
	}
	if c.report != nil && c.nativeStandaloneAssertion.report != nil {
		*c.report = *c.nativeStandaloneAssertion.report
	}
	d := NewClassObjectDumper(c.obj)
	d.options, d.report, d.Work = c.options, c.report, c.Work
	d.foldSiblingResolver, d.declarationResolver = c.foldSiblingResolver, c.declarationResolver
	d.sourceDeclarationAccess, d.sourceDeclarationAccessKnown = c.sourceDeclarationAccess, c.sourceDeclarationAccessKnown
	d.nativeStandaloneAssertionOff = true
	return d.DumpClass()
}

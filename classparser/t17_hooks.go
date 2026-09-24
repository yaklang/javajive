package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// RecordLayout is filled by T20 when a class can be emitted as a Java record.
type RecordLayout struct {
	Keyword           string
	Components        string
	DropExtendsRecord bool
	SkipFieldNames    map[string]bool
	SkipMethodKeys    map[string]bool
}

// TryRecordLayout is set by T20. Nil means records are not reconstructed.
var TryRecordLayout func(c *ClassObjectDumper) *RecordLayout

// RecordReconstructionComplete is set by T20 when the declaration was reconstructed losslessly.
var RecordReconstructionComplete func(obj *ClassObject) bool

// PatternSwitchReconstructionComplete is set by T21.
var PatternSwitchReconstructionComplete func(obj *ClassObject) bool

// AfterMembersDumped is set by T20/T21 to rewrite compact constructors and
// pattern-switch method bodies after DumpMethods/DumpFields have filled caches.
var AfterMembersDumped func(c *ClassObjectDumper)

func classLooksLikeRecord(obj *ClassObject) bool {
	if obj == nil {
		return false
	}
	super := strings.ReplaceAll(obj.GetSupperClassName(), "/", ".")
	if super == "java.lang.Record" {
		return true
	}
	for _, a := range obj.Attributes {
		if v, ok := a.(*UnparsedAttribute); ok && v.Name == "Record" {
			return true
		}
	}
	return false
}

func classBootstrapIdentities(obj *ClassObject) []core.BootstrapIdentity {
	if obj == nil {
		return nil
	}
	var methods []*BootstrapMethod
	for _, a := range obj.Attributes {
		if b, ok := a.(*BootstrapMethodsAttribute); ok && b != nil {
			methods = b.BootstrapMethods
			break
		}
	}
	out := make([]core.BootstrapIdentity, 0, len(methods))
	for _, m := range methods {
		v := GetValueFromCP(obj.ConstantPool, int(m.BootstrapMethodRef))
		member, ok := v.(*values.JavaClassMember)
		if !ok || member == nil {
			continue
		}
		id, err := core.IdentityFromMember(member)
		if err != nil {
			continue
		}
		out = append(out, id.Normalized())
	}
	return out
}

func classHasFamily(obj *ClassObject, fam core.FeatureFamily) bool {
	for _, id := range classBootstrapIdentities(obj) {
		if got, ok := core.LookupBuiltin(id); ok && got == fam {
			return true
		}
	}
	return false
}

func collectPoolCondyReports(obj *ClassObject) []core.DispatchResult {
	if obj == nil {
		return nil
	}
	bsmCount := 0
	for _, a := range obj.Attributes {
		if b, ok := a.(*BootstrapMethodsAttribute); ok && b != nil {
			bsmCount = len(b.BootstrapMethods)
			break
		}
	}
	var out []core.DispatchResult
	for _, c := range obj.ConstantPool {
		dyn, ok := c.(*ConstantDynamicInfo)
		if !ok || dyn == nil {
			continue
		}
		name, desc := "", ""
		idx := int(dyn.NameAndTypeIndex)
		if idx > 0 && idx <= len(obj.ConstantPool) {
			if nt, ok := obj.ConstantPool[idx-1].(*ConstantNameAndTypeInfo); ok && nt != nil {
				if int(nt.NameIndex) > 0 && int(nt.NameIndex) <= len(obj.ConstantPool) {
					if u, ok := obj.ConstantPool[nt.NameIndex-1].(*ConstantUtf8Info); ok && u != nil {
						name = u.Value
					}
				}
				if int(nt.DescriptorIndex) > 0 && int(nt.DescriptorIndex) <= len(obj.ConstantPool) {
					if u, ok := obj.ConstantPool[nt.DescriptorIndex-1].(*ConstantUtf8Info); ok && u != nil {
						desc = u.Value
					}
				}
			}
		}
		out = append(out, core.ClassifyCondy(name, desc, int(dyn.BootstrapMethodAttrIndex), bsmCount))
	}
	return out
}

func applyBootstrapCapabilities(result *DecompileResult, d *ClassObjectDumper) {
	if result == nil || d == nil {
		return
	}
	reports := append([]core.DispatchResult{}, d.bootstrapReports...)
	reports = append(reports, collectPoolCondyReports(d.obj)...)

	target := d.options.TargetSourceVersion
	var major uint16
	if d.obj != nil {
		major = d.obj.MajorVersion
	}

	if classLooksLikeRecord(d.obj) {
		cap := core.EvaluateCapability(core.FamilyRecord, target, major)
		complete := RecordReconstructionComplete != nil && RecordReconstructionComplete(d.obj)
		if !cap.Lossless {
			reports = append(reports, core.DispatchResult{
				Status:         "unsupported",
				Family:         core.FamilyRecord,
				DiagnosticCode: core.DiagBootstrapVersion,
				Reason:         cap.Notes,
				Capability:     cap,
			})
		} else if !complete {
			reports = append(reports, core.DispatchResult{
				Status:         "unsupported",
				Family:         core.FamilyRecord,
				DiagnosticCode: core.DiagBootstrapPartial,
				Reason:         "record class is representable at this language level but declaration was not reconstructed",
				Capability:     cap,
			})
		}
	}
	if classHasFamily(d.obj, core.FamilyTypeSwitch) || classHasFamily(d.obj, core.FamilyEnumSwitch) {
		fam := core.FamilyTypeSwitch
		if classHasFamily(d.obj, core.FamilyEnumSwitch) && !classHasFamily(d.obj, core.FamilyTypeSwitch) {
			fam = core.FamilyEnumSwitch
		}
		cap := core.EvaluateCapability(fam, target, major)
		complete := PatternSwitchReconstructionComplete != nil && PatternSwitchReconstructionComplete(d.obj)
		if !cap.Lossless {
			reports = append(reports, core.DispatchResult{
				Status:         "unsupported",
				Family:         fam,
				DiagnosticCode: core.DiagBootstrapVersion,
				Reason:         cap.Notes,
				Capability:     cap,
			})
		} else if !complete {
			reports = append(reports, core.DispatchResult{
				Status:         "unsupported",
				Family:         fam,
				DiagnosticCode: core.DiagBootstrapPartial,
				Reason:         "pattern switch is representable at this language level but was not reconstructed",
				Capability:     cap,
			})
		}
	}

	status := result.Status
	for _, rep := range reports {
		if rep.DiagnosticCode == "" && rep.Reason == "" && rep.Status == "" {
			continue
		}
		code := rep.DiagnosticCode
		if code == "" {
			code = "bootstrap"
		}
		result.Diagnostics = append(result.Diagnostics, DecompileDiagnostic{
			Code:    code,
			Message: rep.Reason + " identity=" + rep.Identity.Format(),
		})
		switch rep.Status {
		case "invalid_input":
			status = "invalid_input"
		case "unsupported":
			if status != "invalid_input" {
				status = "unsupported"
			}
		case "partial":
			if status == "complete" {
				status = "partial"
			}
		}
	}
	result.Status = status
}

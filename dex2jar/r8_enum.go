package dex2jar

import (
	"fmt"

	"github.com/yaklang/javajive/dex2jar/ir"
	"github.com/yaklang/javajive/dex2jar/model"
)

const (
	r8EnumDexPrivateStaticFinalSynth = uint32(0x101a)
	r8EnumDexVarargs                 = uint32(0x0080)
)

// normalizeR8EnumDeclarations restores canonical Java enum member flags that R8
// widened when merging/un-privatizing enum members (FailureRecover only):
//   - widened `$VALUES` backing arrays (e.g. 0x1019 -> 0x101a) when zero
//     external classes reference that field and `values()` reads it
//   - public/synthetic `<init>(String, int, ...)` on `final` enums when no
//     subclasses exist and zero external classes invoke the constructor.
func normalizeR8EnumDeclarations(app *model.Application, hierarchy map[string]*model.Class) []ConversionDiagnostic {
	if app == nil {
		return nil
	}
	type enumTarget struct {
		file *model.File
		cls  *model.Class
	}
	candidates := make(map[string]enumTarget)
	for _, f := range app.Files {
		if f == nil {
			continue
		}
		for ci := range f.Classes {
			cls := &f.Classes[ci]
			if cls.AccessFlags&r8DexEnum == 0 || cls.Superclass != "Ljava/lang/Enum;" {
				continue
			}
			candidates[cls.Descriptor] = enumTarget{file: f, cls: cls}
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	hasSubclass := make(map[string]bool)
	for _, cls := range hierarchy {
		if cls != nil && cls.Superclass != "" {
			hasSubclass[cls.Superclass] = true
		}
	}

	externalFieldRef := make(map[string]bool)
	valuesReadsField := make(map[string]bool)
	externalCtorRef := make(map[string]bool)

	scanMethods := func(f *model.File, cls *model.Class, methods []model.EncodedMethod) {
		for mi := range methods {
			m := &methods[mi]
			if m.Code == nil {
				continue
			}
			insns, err := ir.Decode(m.Code, ir.Context{Entry: f.Entry, Class: cls.Descriptor, Method: m.Method.Name})
			if err != nil {
				continue
			}
			isOwnValues := false
			if _, ok := candidates[cls.Descriptor]; ok && m.Method.Name == "values" && int(m.Method.ProtoIndex) < len(f.Protos) {
				p := f.Protos[m.Method.ProtoIndex]
				if len(p.Parameters) == 0 && p.ReturnType == "["+cls.Descriptor {
					isOwnValues = true
				}
			}
			for i := range insns {
				ins := &insns[i]
				op := ins.Opcode
				switch {
				case op >= 0x60 && op <= 0x6d:
					if int(ins.Index) >= len(f.Fields) {
						continue
					}
					fld := f.Fields[ins.Index]
					if _, tracked := candidates[fld.Class]; !tracked {
						continue
					}
					key := fld.Class + "\x00" + fld.Name
					if fld.Class != cls.Descriptor {
						externalFieldRef[key] = true
					} else if isOwnValues && op <= 0x66 {
						valuesReadsField[key] = true
					}
				case (op >= 0x6e && op <= 0x72) || (op >= 0x74 && op <= 0x78):
					if int(ins.Index) >= len(f.Methods) {
						continue
					}
					callee := f.Methods[ins.Index]
					if callee.Name != "<init>" {
						continue
					}
					if _, tracked := candidates[callee.Class]; tracked && callee.Class != cls.Descriptor {
						externalCtorRef[callee.Class] = true
					}
				}
			}
		}
	}

	for _, f := range app.Files {
		if f == nil {
			continue
		}
		for ci := range f.Classes {
			cls := &f.Classes[ci]
			scanMethods(f, cls, cls.DirectMethods)
			scanMethods(f, cls, cls.VirtualMethods)
		}
	}

	var diags []ConversionDiagnostic
	for _, f := range app.Files {
		if f == nil {
			continue
		}
		for ci := range f.Classes {
			cls := &f.Classes[ci]
			if cls.AccessFlags&r8DexEnum == 0 || cls.Superclass != "Ljava/lang/Enum;" {
				continue
			}

			// 1. Normalize widened $VALUES backing array when provably internal-only.
			arrType := "[" + cls.Descriptor
			annotated := make(map[uint32]bool, len(cls.FieldAnnotations))
			for _, fa := range cls.FieldAnnotations {
				annotated[fa.FieldIndex] = true
			}
			backingCount := 0
			backingIdx := -1
			for fi := range cls.StaticFields {
				sf := &cls.StaticFields[fi]
				if sf.Field.Class != cls.Descriptor || sf.Field.Type != arrType {
					continue
				}
				if (sf.AccessFlags & (r8DexStatic | r8DexFinal)) != (r8DexStatic | r8DexFinal) {
					continue
				}
				if annotated[sf.FieldIndex] {
					continue
				}
				backingCount++
				backingIdx = fi
			}
			if backingCount == 1 && backingIdx >= 0 {
				sf := &cls.StaticFields[backingIdx]
				switch uint16(sf.AccessFlags & 0xffff) {
				case 0x1019, 0x0019, 0x001a:
					key := cls.Descriptor + "\x00" + sf.Field.Name
					if !externalFieldRef[key] && valuesReadsField[key] {
						sf.AccessFlags = r8EnumDexPrivateStaticFinalSynth
						diags = append(diags, ConversionDiagnostic{
							Entry:      f.Entry,
							Class:      cls.Descriptor,
							Method:     sf.Field.Name,
							Descriptor: sf.Field.Type,
							Category:   "recovery",
							Action:     "normalize_enum_backing",
							Feature:    "r8-enum-normalization",
							Code:       "r8.enum_backing_normalized",
							Kind:       "recovery",
							Scope:      "field",
							Stage:      "recover",
							Severity:   "warning",
							Offset:     -1,
							Message:    fmt.Sprintf("restored private synthetic flags on R8-widened enum backing array %s", sf.Field.Name),
						})
					}
				}
			}

			// 2. Normalize widened/synthetic <init> on final enums with zero external refs.
			if cls.AccessFlags&r8DexFinal != 0 && !hasSubclass[cls.Descriptor] && !externalCtorRef[cls.Descriptor] {
				for mi := range cls.DirectMethods {
					m := &cls.DirectMethods[mi]
					if m.Method.Name != "<init>" || int(m.Method.ProtoIndex) >= len(f.Protos) {
						continue
					}
					p := f.Protos[m.Method.ProtoIndex]
					if p.ReturnType != "V" || len(p.Parameters) < 2 || p.Parameters[0] != "Ljava/lang/String;" || p.Parameters[1] != "I" {
						continue
					}
					switch uint16(m.AccessFlags & 0xffff) {
					case 0x0001, 0x1001, 0x0081, 0x1081, 0x1002, 0x1082:
						m.AccessFlags = r8DexPrivate | r8DexConstructor | (m.AccessFlags & r8EnumDexVarargs)
						diags = append(diags, ConversionDiagnostic{
							Entry:      f.Entry,
							Class:      cls.Descriptor,
							Method:     "<init>",
							Descriptor: protoSignature(p),
							Category:   "recovery",
							Action:     "normalize_enum_ctor",
							Feature:    "r8-enum-normalization",
							Code:       "r8.enum_ctor_normalized",
							Kind:       "recovery",
							Scope:      "method",
							Stage:      "recover",
							Severity:   "warning",
							Offset:     -1,
							Message:    fmt.Sprintf("restored private flags on R8 enum constructor %s.<init>%s", cls.Descriptor, protoSignature(p)),
						})
					}
				}
			}
		}
	}
	return diags
}

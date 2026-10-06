package javaclassparser

import (
	"encoding/binary"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"sort"
	"strings"
)

func nativeAnonymousForestCaptureReference(forest *nativeAnonymousForest, object *ClassObject, index int, work *workbudget.Budget) bool {
	if forest == nil || object == nil || forest.objects[object.GetClassName()] == nil || index < 1 || index > len(object.ConstantPool) {
		return false
	}
	field, ok := object.ConstantPool[index-1].(*ConstantFieldrefInfo)
	if !ok || field == nil {
		return false
	}
	owner, known := sourceBridgeClassName(object, field.ClassIndex)
	if !known || forest.units[owner] == nil {
		return false
	}
	if field.NameAndTypeIndex < 1 || int(field.NameAndTypeIndex) > len(object.ConstantPool) {
		return false
	}
	nt, ok := object.ConstantPool[field.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
	if !ok || nt == nil {
		return false
	}
	name, known := sourceBridgeUTF8(object, nt.NameIndex)
	if !known {
		return false
	}
	if _, captured := forest.units[owner].fields[name]; !captured {
		return false
	}
	desc, known := sourceBridgeUTF8(object, nt.DescriptorIndex)
	if !known {
		return false
	}
	read := forest.captureReferences[object.GetClassName()][index]
	return read != nil && read.owner == owner && read.field == name && read.descriptor == desc

}

func nativeAnonymousForestCaptureNameType(forest *nativeAnonymousForest, object *ClassObject, index int, work *workbudget.Budget) bool {
	if forest == nil || object == nil {
		return false
	}
	used := false
	for i, constant := range object.ConstantPool {
		if !nativeProofWork(work, 1) {
			return false
		}
		if member := nativeConstantMember(constant); member != nil && int(member.NameAndTypeIndex) == index {
			if !nativeAnonymousForestCaptureReference(forest, object, i+1, work) {
				return false
			}
			used = true
		}
		switch dynamic := constant.(type) {
		case *ConstantInvokeDynamicInfo:
			if int(dynamic.NameAndTypeIndex) == index {
				return false
			}
		case *ConstantDynamicInfo:
			if int(dynamic.NameAndTypeIndex) == index {
				return false
			}
		}
	}
	return used
}

func nativeAnonymousForestVersion(object *ClassObject, work *workbudget.Budget) bool {
	if !nativeAccessorVersion(object, work) {
		return false
	}
	if object.MajorVersion >= 52 {
		return true
	}
	// Named and anonymous ownership metadata first became authoritative in
	// version 49. Before 51 there are no dynamic CP entries, and before 52
	// static/special interface calls are not valid instructions. Check the
	// original instruction boundaries too; editing the version grants nothing.
	for _, m := range object.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return false
		}
		for _, a := range m.Attributes {
			if !nativeProofWork(work, 1) {
				return false
			}
			if code, ok := a.(*CodeAttribute); ok {
				if code == nil || !nativeLegacyForestCode(object, code, work) {
					return false
				}
			}
		}
	}
	return true
}

func nativeAnonymousForestCaptureMetadata(child *nativeAnonymousClass, work *workbudget.Budget) bool {
	if child == nil {
		return false
	}
	for _, field := range child.object.Fields {
		if field == nil {
			return false
		}
		name, nok := sourceBridgeUTF8(child.object, field.NameIndex)
		if !nok {
			return false
		}
		if _, captured := child.fields[name]; !captured {
			continue
		}
		flags, only, valid := nativeMemberEffectiveFieldFlags(field, work)
		if !valid || !only || flags != 0x1010 {
			return false
		}
	}
	return true
}

// A capture variable shared with an enclosing anonymous object must regenerate
// the same physical chain. Only consecutive original THIS/enclosing-field reads
// ending at a proved local capture or named lexical THIS qualify. Intermediate
// anonymous THIS has no Java source spelling and cannot be emitted on its own.
func nativeAnonymousForestCaptureReads(forest *nativeAnonymousForest, work *workbudget.Budget, providers ...callbinding.Provider) bool {
	var metadata callbinding.Provider
	if len(providers) != 0 {
		metadata = providers[0]
	}
	for owner, object := range forest.objects {
		forest.reads[owner] = map[string]map[int]*nativeMemberLexicalRead{}
		forest.readPCs[owner] = map[string]map[int]bool{}
		forest.lexicalThis[owner] = map[string]map[int]bool{}
		forest.captureReferences[owner] = map[int]*nativeMemberLexicalRead{}
		for _, method := range object.Methods {
			if method == nil {
				return false
			}
			name, nok := sourceBridgeUTF8(object, method.NameIndex)
			desc, dok := sourceBridgeUTF8(object, method.DescriptorIndex)
			if !nok || !dok {
				return false
			}
			key := name + desc
			reads := map[int]*nativeMemberLexicalRead{}
			approved := map[int]bool{}
			paths := map[int]*nativeMemberLexicalRead{}
			forest.reads[owner][key] = reads
			forest.readPCs[owner][key] = approved
			forest.lexicalThis[owner][key] = map[int]bool{}
			for _, attribute := range method.Attributes {
				code, ok := attribute.(*CodeAttribute)
				if !ok {
					continue
				}
				if !nativeProofWork(work, int64(len(code.Code))) {
					return false
				}
				decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(object.ConstantPool, i) })
				decoder.Work = work
				if decoder.ParseOpcode() != nil {
					return false
				}
				entries, valid := nativeMemberLexicalControlEntries(decoder, code, work)
				if !valid {
					return false
				}
				ops := constructorMotionOps(decoder)
				stable := true
				for _, op := range ops {
					if core.GetStoreIdx(op) == 0 {
						stable = false
					}
				}
				for i, op := range ops {
					if method.AccessFlags&8 != 0 || core.GetRetrieveIdx(op) != 0 || !constructorMotionLoad(op, "Ljava/lang/Object;") {
						continue
					}
					current := owner
					var prior *nativeMemberLexicalRead
					seen := map[string]bool{}
					for j := i + 1; j < len(ops); j++ {
						if !nativeProofWork(work, 1) || seen[current] || len(seen) >= 64 {
							return false
						}
						seen[current] = true
						field := constructorMotionMember(object, ops[j], core.OP_GETFIELD)
						if field == nil || field.Name != current {
							break
						}
						lexicalOwner, lexical, next := "", false, ""
						if child := forest.units[current]; child != nil {
							parameter, captured := child.fields[field.Member]
							if !captured {
								break
							}
							ps, _, e := callbinding.Descriptor(child.descriptor)
							if e != nil || parameter >= len(ps) || parameter < 0 || field.Description != ps[parameter] {
								return false
							}
							if field.Member == child.enclosingField {
								parent, _, known := originalAnonymousOwner(child.object)
								if !known || field.Description != "L"+parent+";" {
									return false
								}
								next = parent
								if !child.parentAnonymous {
									lexicalOwner, lexical = parent, parent == forest.root && forest.objects[parent] != nil || forest.members != nil && forest.members.children[parent] != nil
								}
							} else if prior != nil && !strings.HasPrefix(field.Member, "val$") {
								return false
							}
						} else if forest.members != nil {
							child := forest.members.children[current]
							if child == nil || child.static || field.Member != child.field {
								break
							}
							if field.Description != "L"+child.owner+";" {
								return false
							}
							lexicalOwner, lexical, next = child.owner, true, child.owner
						} else {
							break
						}
						entry := sort.SearchInts(entries, int(ops[j-1].CurrentOffset)+1)
						if entry < len(entries) && entries[entry] <= int(ops[j].CurrentOffset) {
							break
						}
						read := &nativeMemberLexicalRead{owner: field.Name, field: field.Member, descriptor: field.Description, pc: int(ops[j].CurrentOffset), prior: prior}
						if lexical || next == "" && prior != nil {
							if !stable || reads[read.pc] != nil {
								return false
							}
							reads[read.pc] = read
							if lexical {
								if read.descriptor != "L"+lexicalOwner+";" {
									return false
								}
								forest.lexicalThis[owner][key][read.pc] = true
							}
							for node := read; node != nil; node = node.prior {
								approved[node.pc] = true
								paths[node.pc] = node
							}
						}
						if next == "" {
							break
						}
						prior, current = read, next
					}
				}
				// Named constructors may supply SUPER's outer object through slot 1.
				// Compose the same physical constructor proof after THIS discovery;
				// parameter provenance is retained for the later source/IR check.
				if name == "<init>" && forest.members != nil {
					if child := forest.members.children[owner]; child != nil {
						if ctor := child.constructors[desc]; ctor != nil && ctor.enclosingSuperPath != nil {
							path, closed := nativeMemberConstructorSuperRead(child, forest.members, desc, ops, entries, work, metadata)
							if !closed {
								return false
							}
							for node := path; node != nil; node = node.prior {
								if reads[node.pc] != nil || approved[node.pc] {
									return false
								}
								reads[node.pc], paths[node.pc], approved[node.pc] = node, node, true
								forest.lexicalThis[owner][key][node.pc] = true
							}
						}
					}
				}
				for _, op := range ops {
					for _, kind := range []int{core.OP_GETFIELD, core.OP_PUTFIELD, core.OP_GETSTATIC, core.OP_PUTSTATIC} {
						field := constructorMotionMember(object, op, kind)
						if field == nil {
							continue
						}
						// Named enclosing reads are covered by the same original chain;
						// a second foreign read cannot borrow one proved THIS path.
						if forest.members != nil {
							if named := forest.members.children[field.Name]; named != nil && !named.static && field.Member == named.field {
								if field.Description != "L"+named.owner+";" {
									return false
								}
								pc := int(op.CurrentOffset)
								if kind == core.OP_PUTFIELD {
									ctor := named.constructors[desc]
									if owner != field.Name || name != "<init>" || ctor == nil || ctor.capturePC != pc {
										return false
									}
								} else if kind != core.OP_GETFIELD || !stable || !approved[pc] {
									return false
								}
							}
						}
						child := forest.units[field.Name]
						if child == nil {
							continue
						}
						parameter, captured := child.fields[field.Member]
						if !captured {
							continue
						}
						ps, _, bad := callbinding.Descriptor(child.descriptor)
						if bad != nil || parameter < 0 || parameter >= len(ps) || ps[parameter] != field.Description {
							return false
						}
						pc := int(op.CurrentOffset)
						switch kind {
						case core.OP_PUTFIELD:
							if owner != field.Name || name != "<init>" || desc != child.descriptor || child.capturePCs[field.Member] != pc {
								return false
							}
						case core.OP_GETFIELD:
							if !stable {
								return false
							}
							if approved[pc] {
								if len(op.Data) != 2 {
									return false
								}
								index := int(binary.BigEndian.Uint16(op.Data))
								read := paths[pc]
								prior := forest.captureReferences[owner][index]
								if prior != nil && (prior.owner != read.owner || prior.field != read.field || prior.descriptor != read.descriptor) {
									return false
								}
								forest.captureReferences[owner][index] = read
								continue
							}
							if owner != field.Name || child.parentAnonymous && field.Member == child.enclosingField {
								return false
							}
						default:
							return false
						}
					}
				}
			}
		}
	}
	return true
}

func (c *ClassObjectDumper) wireNativeAnonymousForestCaptures(ctx *class_context.ClassContext) {
	forest := c.nativeAnonymousForest
	if forest == nil {
		return
	}
	for _, field := range c.obj.Fields {
		name, known := sourceBridgeUTF8(c.obj, field.NameIndex)
		if !known {
			c.nativeCaptureFailed = true
			continue
		}
		if _, captured := c.nativeCaptureFields[name]; captured {
			continue
		}
		for _, text := range c.nativeAnonymousBindings {
			if text == name {
				c.nativeCaptureFailed = true
			}
		}
	}
	ctx.SourceLexicalCapturedField = func(value any, pc int, name string) (string, bool) {
		read := forest.reads[c.obj.GetClassName()][ctx.FunctionName+ctx.CurrentMethodDesc][pc]
		if read != nil {
			text, known := c.nativeAnonymousBindings[nativeMemberCaptureIndexKey(read.owner, read.field)]
			validText := known && class_context.SafeIdentifier(text) == text
			if forest.lexicalThis[c.obj.GetClassName()][ctx.FunctionName+ctx.CurrentMethodDesc][pc] {
				validText = false
				lexicalOwner := ""
				if child := forest.units[read.owner]; child != nil && !child.parentAnonymous {
					owner, _, original := originalAnonymousOwner(child.object)
					if original && read.field == child.enclosingField && (owner == forest.root && forest.objects[owner] != nil || forest.members != nil && forest.members.children[owner] != nil) {
						lexicalOwner = owner
					}
				} else if forest.members != nil {
					if named := forest.members.children[read.owner]; named != nil && !named.static && read.field == named.field {
						lexicalOwner = named.owner
					}
				}
				if lexicalOwner != "" && read.descriptor == "L"+lexicalOwner+";" {
					validText = known && text == ctx.ShortTypeName(strings.ReplaceAll(lexicalOwner, "/", "."))+".this"
				}

			}
			if !validText || name != read.field || !nativeMemberLexicalReadOperand(value, read, c.Work, ctx) {
				c.nativeCaptureFailed = true
				return "", false
			}
			return text, true
		}
		field, valid := value.(*values.RefMember)
		if !valid || field == nil || sourceProofNil(field.Object) {
			return "", false
		}
		erased, known := values.SourceTypeErasure(field.Object.Type(), ctx)
		if known && strings.HasPrefix(erased, "L") && strings.HasSuffix(erased, ";") {
			owner := erased[1 : len(erased)-1]
			child := forest.units[owner]
			if child != nil {
				if _, captured := child.fields[name]; captured && (owner != c.obj.GetClassName() || child.parentAnonymous && name == child.enclosingField) {
					c.nativeCaptureFailed = true
				}
			}
			if forest.members != nil {
				if named := forest.members.children[owner]; named != nil && !named.static && name == named.field {
					c.nativeCaptureFailed = true
				}
			}

		}
		return "", false
	}
}

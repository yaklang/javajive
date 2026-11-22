package lower

import (
	"fmt"
	"strings"

	"github.com/yaklang/javajive/classparser/classfilebuilder"
	"github.com/yaklang/javajive/dex2jar/ir"
)

type slotEntry struct {
	vt       classfilebuilder.VerificationType
	cat2Tail bool
}

type slotFrame []slotEntry

type stackMapState struct {
	entryFrame       slotFrame
	in               []slotFrame
	visited          []bool
	blockByStart     map[uint32]int
	catchClassByAddr map[uint32]string
	deferred         map[int]*deferredStore
}

func topSlotEntry() slotEntry {
	return slotEntry{vt: classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyTop}}
}

func cloneSlotFrame(src slotFrame) slotFrame {
	dst := make(slotFrame, len(src))
	copy(dst, src)
	return dst
}

func slotEntriesEqual(a, b slotEntry) bool {
	if a.cat2Tail != b.cat2Tail || a.vt.Tag != b.vt.Tag || a.vt.NewAt != b.vt.NewAt {
		return false
	}
	if a.vt.Tag == classfilebuilder.VerifyObject && a.vt.Class != b.vt.Class {
		return false
	}
	return true
}

func slotFramesEqual(a, b slotFrame) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !slotEntriesEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func isCat2Tag(tag classfilebuilder.VerifyTag) bool {
	return tag == classfilebuilder.VerifyLong || tag == classfilebuilder.VerifyDouble
}

func setSlot(frame slotFrame, slot int, vt classfilebuilder.VerificationType) {
	if slot < 0 || slot >= len(frame) {
		return
	}
	if frame[slot].cat2Tail && slot > 0 {
		frame[slot-1] = topSlotEntry()
	}
	if isCat2Tag(frame[slot].vt.Tag) && slot+1 < len(frame) {
		frame[slot+1] = topSlotEntry()
	}
	frame[slot] = slotEntry{vt: vt}
	if isCat2Tag(vt.Tag) && slot+1 < len(frame) {
		if isCat2Tag(frame[slot+1].vt.Tag) && slot+2 < len(frame) {
			frame[slot+2] = topSlotEntry()
		}
		frame[slot+1] = slotEntry{
			vt:       classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyTop},
			cat2Tail: true,
		}
	}
}

func verifyTypeFromDescriptor(descriptor string) (classfilebuilder.VerificationType, bool, error) {
	switch descriptor {
	case "Z", "B", "C", "S", "I":
		return classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyInt}, false, nil
	case "F":
		return classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyFloat}, false, nil
	case "J":
		return classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyLong}, true, nil
	case "D":
		return classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyDouble}, true, nil
	}
	name, err := ir.InternalName(descriptor)
	if err != nil {
		return classfilebuilder.VerificationType{}, false, err
	}
	return classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyObject, Class: name}, false, nil
}

func verifyTypeFromCategory(cat ir.Category) classfilebuilder.VerificationType {
	switch cat.Kind {
	case ir.Float:
		return classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyFloat}
	case ir.Long, ir.Bits64:
		return classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyLong}
	case ir.Double:
		return classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyDouble}
	case ir.Null:
		return classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyNull}
	case ir.Reference:
		if cat.Descriptor != "" && cat.Descriptor != nullDescriptor {
			if vt, _, err := verifyTypeFromDescriptor(cat.Descriptor); err == nil {
				return vt
			}
		}
		return classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyObject, Class: "java/lang/Object"}
	default:
		return classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyInt}
	}
}

func verifyTagForLocalKind(lk classfilebuilder.LocalKind) classfilebuilder.VerifyTag {
	switch lk {
	case classfilebuilder.IntLocal:
		return classfilebuilder.VerifyInt
	case classfilebuilder.FloatLocal:
		return classfilebuilder.VerifyFloat
	default:
		return classfilebuilder.VerifyNull
	}
}

func (l *methodLowerer) definitionCategoryAt(index int, register uint16) ir.Category {
	if definition, ok := l.method.DefinitionAt(index); ok && definition.Register == int32(register) {
		return definition.Category
	}
	return l.method.CategoryAt(index, register)
}

func (l *methodLowerer) commonVerifyClass(a, b string) string {
	if a == b {
		return a
	}
	if a == "" || b == "" {
		return "java/lang/Object"
	}
	if strings.HasPrefix(a, "[") || strings.HasPrefix(b, "[") {
		return "java/lang/Object"
	}
	index := l.classIndex()
	ancestorsA := map[string]bool{a: true}
	cur := "L" + a + ";"
	for step := 0; step < maxAssignabilityDepth; step++ {
		c, ok := index[cur]
		if !ok || c.Superclass == "" {
			break
		}
		superInternal, err := objectInternalName(c.Superclass)
		if err != nil {
			break
		}
		ancestorsA[superInternal] = true
		cur = c.Superclass
	}
	if ancestorsA[b] {
		return b
	}
	cur = "L" + b + ";"
	for step := 0; step < maxAssignabilityDepth; step++ {
		c, ok := index[cur]
		if !ok || c.Superclass == "" {
			break
		}
		superInternal, err := objectInternalName(c.Superclass)
		if err != nil {
			break
		}
		if ancestorsA[superInternal] {
			return superInternal
		}
		cur = c.Superclass
	}
	return "java/lang/Object"
}

func (l *methodLowerer) isVerifySubtype(sub, sup string) bool {
	if sub == sup || sup == "java/lang/Object" {
		return true
	}
	if sub == "" || sup == "" || strings.HasPrefix(sub, "[") || strings.HasPrefix(sup, "[") {
		return false
	}
	index := l.classIndex()
	cur := "L" + sub + ";"
	for step := 0; step < maxAssignabilityDepth; step++ {
		c, ok := index[cur]
		if !ok || c.Superclass == "" {
			break
		}
		superInternal, err := objectInternalName(c.Superclass)
		if err != nil {
			break
		}
		if superInternal == sup {
			return true
		}
		cur = c.Superclass
	}
	return false
}

func (l *methodLowerer) slotFrameCompatibleWithHandler(cur, handler slotFrame) bool {
	for s := 0; s < len(handler); s++ {
		h := handler[s]
		if h.cat2Tail || h.vt.Tag == classfilebuilder.VerifyTop {
			continue
		}
		if s >= len(cur) {
			return false
		}
		c := cur[s]
		if c.cat2Tail || c.vt.Tag == classfilebuilder.VerifyTop {
			return false
		}
		switch h.vt.Tag {
		case classfilebuilder.VerifyObject:
			if c.vt.Tag == classfilebuilder.VerifyNull {
				continue
			}
			if c.vt.Tag != classfilebuilder.VerifyObject || !l.isVerifySubtype(c.vt.Class, h.vt.Class) {
				return false
			}
		case classfilebuilder.VerifyUninitialized:
			if c.vt.Tag != classfilebuilder.VerifyUninitialized || c.vt.NewAt != h.vt.NewAt {
				return false
			}
		default:
			if c.vt.Tag != h.vt.Tag {
				return false
			}
		}
	}
	return true
}

func (l *methodLowerer) isProvablyThrowable(internalName string) bool {
	if internalName == "java/lang/Throwable" || internalName == "java/lang/Exception" ||
		internalName == "java/lang/RuntimeException" || internalName == "java/lang/Error" {
		return true
	}
	index := l.classIndex()
	cur := "L" + internalName + ";"
	for step := 0; step < maxAssignabilityDepth; step++ {
		c, ok := index[cur]
		if !ok || c.Superclass == "" {
			break
		}
		if c.Superclass == "Ljava/lang/Throwable;" || c.Superclass == "Ljava/lang/Exception;" ||
			c.Superclass == "Ljava/lang/RuntimeException;" || c.Superclass == "Ljava/lang/Error;" {
			return true
		}
		cur = c.Superclass
	}
	return false
}

func (l *methodLowerer) mergeCatchClass(cur, next string) string {
	if cur == "" {
		return next
	}
	if next == "" || cur == "java/lang/Throwable" || next == "java/lang/Throwable" {
		return "java/lang/Throwable"
	}
	if cur == next {
		return cur
	}
	lca := l.commonVerifyClass(cur, next)
	if l.isProvablyThrowable(lca) {
		return lca
	}
	return "java/lang/Throwable"
}

func (l *methodLowerer) joinSlotFrames(dst slotFrame, visitedDst bool, src slotFrame) (slotFrame, bool) {
	if !visitedDst {
		return cloneSlotFrame(src), true
	}
	out := cloneSlotFrame(dst)
	for i := range out {
		a, b := out[i], src[i]
		if slotEntriesEqual(a, b) {
			continue
		}
		if a.cat2Tail || b.cat2Tail || a.vt.Tag == classfilebuilder.VerifyTop || b.vt.Tag == classfilebuilder.VerifyTop {
			out[i] = topSlotEntry()
			continue
		}
		switch {
		case a.vt.Tag == classfilebuilder.VerifyNull && b.vt.Tag == classfilebuilder.VerifyObject:
			out[i] = b
		case a.vt.Tag == classfilebuilder.VerifyObject && b.vt.Tag == classfilebuilder.VerifyNull:
			// keep a
		case a.vt.Tag == classfilebuilder.VerifyObject && b.vt.Tag == classfilebuilder.VerifyObject:
			out[i] = slotEntry{vt: classfilebuilder.VerificationType{
				Tag:   classfilebuilder.VerifyObject,
				Class: l.commonVerifyClass(a.vt.Class, b.vt.Class),
			}}
		default:
			out[i] = topSlotEntry()
		}
	}
	// If the low half of a wide local joined to VerifyTop, clear its paired tail.
	for i := 0; i+1 < len(out); i++ {
		if out[i+1].cat2Tail && !isCat2Tag(out[i].vt.Tag) {
			out[i+1] = topSlotEntry()
		}
	}
	return out, !slotFramesEqual(dst, out)
}

func compactStackMapLocals(frame slotFrame) []classfilebuilder.VerificationType {
	lastNonTop := -1
	for i := 0; i < len(frame); i++ {
		if frame[i].cat2Tail {
			continue
		}
		if frame[i].vt.Tag != classfilebuilder.VerifyTop {
			lastNonTop = i
		}
	}
	if lastNonTop < 0 {
		return nil
	}
	out := make([]classfilebuilder.VerificationType, 0, lastNonTop+1)
	for i := 0; i <= lastNonTop; i++ {
		if frame[i].cat2Tail {
			continue
		}
		out = append(out, frame[i].vt)
	}
	return out
}

func (l *methodLowerer) transferSlotFrame(blockIdx, insnIdx int, frame slotFrame, deferred map[int]*deferredStore, catchClassByAddr map[uint32]string) {
	if l.fusedNoLocals[insnIdx] {
		return
	}
	insn := &l.method.Instructions[insnIdx]
	if store, ok := deferred[insnIdx]; ok {
		if !store.direct {
			lk := localKind(store.category.Kind)
			vt := verifyTypeFromCategory(store.category)
			srcSlot := int(l.loadSlotAt(insnIdx, store.register, lk))
			if store.category.Kind == ir.Reference && srcSlot >= 0 && srcSlot < len(frame) && frame[srcSlot].vt.Tag != classfilebuilder.VerifyTop {
				vt = frame[srcSlot].vt
			}
			setSlot(frame, int(store.temp), vt)
		}
		return
	}
	if alloc, ok := l.allocationByNew[insnIdx]; ok {
		if alloc.Fused {
			return
		}
		newLabel := l.allocationNewLabels[alloc.ID]
		dstSlot := int(l.storeSlotAt(insnIdx, insn.Regs[0], classfilebuilder.RefLocal))
		setSlot(frame, dstSlot, classfilebuilder.VerificationType{
			Tag:   classfilebuilder.VerifyUninitialized,
			NewAt: newLabel,
		})
		return
	}
	if l.allocationAliasOps[insnIdx] {
		return
	}
	if alloc, ok := l.allocationByCtor[insnIdx]; ok {
		className, err := objectInternalName(alloc.Descriptor)
		if err != nil {
			className = "java/lang/Object"
		}
		objVT := classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyObject, Class: className}
		if alloc.Fused {
			for _, reg := range alloc.AliasRegisters {
				setSlot(frame, int(l.storeSlotAt(insnIdx, reg, classfilebuilder.RefLocal)), objVT)
			}
			return
		}
		newLabel := l.allocationNewLabels[alloc.ID]
		for i := range frame {
			if frame[i].vt.Tag == classfilebuilder.VerifyUninitialized && frame[i].vt.NewAt == newLabel {
				frame[i].vt = objVT
			}
		}
		return
	}
	if l.isDelegationSite(insnIdx) {
		thisName, err := objectInternalName(l.class.Descriptor)
		if err != nil {
			thisName = "java/lang/Object"
		}
		thisVT := classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyObject, Class: thisName}
		for i := range frame {
			if frame[i].vt.Tag == classfilebuilder.VerifyUninitializedThis {
				frame[i].vt = thisVT
			}
		}
		return
	}

	switch {
	case insn.Opcode >= 0x01 && insn.Opcode <= 0x09:
		kind := l.resolvedDefCategoryAt(insnIdx, insn.Regs[0]).Kind
		if kind == ir.Unknown {
			kind = l.resolvedUseCategoryAt(insnIdx, insn.Regs[1]).Kind
		}
		if kind == ir.Unknown {
			return
		}
		if insn.Opcode <= 0x03 {
			if rawSrc := l.method.CategoryAt(insnIdx, insn.Regs[1]); rawSrc.Kind == ir.Null {
				dst, src := insn.Regs[0], insn.Regs[1]
				hasDstPoly := false
				for _, lk := range []classfilebuilder.LocalKind{classfilebuilder.RefLocal, classfilebuilder.IntLocal, classfilebuilder.FloatLocal} {
					if _, ok := l.slotIfCompanion(insnIdx, dst, lk, true); ok {
						hasDstPoly = true
						break
					}
				}
				srcResolvedLK := localKind(l.resolvedUseCategoryAt(insnIdx, src).Kind)
				if hasDstPoly {
					for _, lk := range []classfilebuilder.LocalKind{classfilebuilder.RefLocal, classfilebuilder.IntLocal, classfilebuilder.FloatLocal} {
						dstSlot, ok := l.slotIfCompanion(insnIdx, dst, lk, true)
						if !ok {
							continue
						}
						vt := classfilebuilder.VerificationType{Tag: verifyTagForLocalKind(lk)}
						if srcSlot, srcOK := l.slotIfCompanion(insnIdx, src, lk, false); srcOK {
							if int(srcSlot) < len(frame) && frame[srcSlot].vt.Tag != classfilebuilder.VerifyTop {
								vt = frame[srcSlot].vt
							}
						} else if srcResolvedLK == lk && rawSrc.Allocation != ir.PolyZeroAllocation {
							srcSlot := int(l.loadSlotAt(insnIdx, src, lk))
							if srcSlot >= 0 && srcSlot < len(frame) && frame[srcSlot].vt.Tag != classfilebuilder.VerifyTop {
								vt = frame[srcSlot].vt
							}
						}
						setSlot(frame, int(dstSlot), vt)
					}
					return
				}
				dstLK := localKind(kind)
				dstSlot := int(l.storeSlotAt(insnIdx, dst, dstLK))
				vt := classfilebuilder.VerificationType{Tag: verifyTagForLocalKind(dstLK)}
				if srcSlot, srcOK := l.slotIfCompanion(insnIdx, src, dstLK, false); srcOK {
					if int(srcSlot) < len(frame) && frame[srcSlot].vt.Tag != classfilebuilder.VerifyTop {
						vt = frame[srcSlot].vt
					}
				} else if srcResolvedLK == dstLK && rawSrc.Allocation != ir.PolyZeroAllocation {
					srcSlot := int(l.loadSlotAt(insnIdx, src, dstLK))
					if srcSlot >= 0 && srcSlot < len(frame) && frame[srcSlot].vt.Tag != classfilebuilder.VerifyTop {
						vt = frame[srcSlot].vt
					}
				}
				setSlot(frame, dstSlot, vt)
				return
			}
		}
		srcCat := l.resolvedUseCategoryAt(insnIdx, insn.Regs[1])
		srcLK := localKind(srcCat.Kind)
		srcSlot := int(l.loadSlotAt(insnIdx, insn.Regs[1], srcLK))
		dstCat := l.resolvedDefCategoryAt(insnIdx, insn.Regs[0])
		dstLK := localKind(dstCat.Kind)
		dstSlot := int(l.storeSlotAt(insnIdx, insn.Regs[0], dstLK))
		vt := verifyTypeFromCategory(dstCat)
		if srcSlot >= 0 && srcSlot < len(frame) && frame[srcSlot].vt.Tag != classfilebuilder.VerifyTop {
			vt = frame[srcSlot].vt
		}
		setSlot(frame, dstSlot, vt)

	case insn.Opcode == 0x0d:
		catchCls := catchClassByAddr[l.method.Blocks[blockIdx].Start]
		if catchCls == "" {
			catchCls = "java/lang/Throwable"
		}
		dstLK := localKind(l.definitionCategoryAt(insnIdx, insn.Regs[0]).Kind)
		dstSlot := int(l.storeSlotAt(insnIdx, insn.Regs[0], dstLK))
		setSlot(frame, dstSlot, classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyObject, Class: catchCls})

	case insn.Opcode >= 0x12 && insn.Opcode <= 0x15:
		defCat := l.resolvedDefCategoryAt(insnIdx, insn.Regs[0])
		if defCat.Kind == ir.Null && insn.Literal == 0 {
			stored := false
			for _, entry := range []struct {
				lk  classfilebuilder.LocalKind
				tag classfilebuilder.VerifyTag
			}{
				{classfilebuilder.RefLocal, classfilebuilder.VerifyNull},
				{classfilebuilder.IntLocal, classfilebuilder.VerifyInt},
				{classfilebuilder.FloatLocal, classfilebuilder.VerifyFloat},
			} {
				if slot, ok := l.slotIfCompanion(insnIdx, insn.Regs[0], entry.lk, true); ok {
					setSlot(frame, int(slot), classfilebuilder.VerificationType{Tag: entry.tag})
					stored = true
				}
			}
			if stored {
				return
			}
		}
		vt := classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyInt}
		switch {
		case defCat.Kind == ir.Float:
			vt = classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyFloat}
		case isReferenceKind(defCat.Kind):
			vt = classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyNull}
		}
		setSlot(frame, int(l.storeSlotAt(insnIdx, insn.Regs[0], localKind(defCat.Kind))), vt)

	case insn.Opcode >= 0x16 && insn.Opcode <= 0x19:
		defCat := l.definitionCategoryAt(insnIdx, insn.Regs[0])
		vt := classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyLong}
		if defCat.Kind == ir.Double {
			vt = classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyDouble}
		}
		setSlot(frame, int(l.storeSlotAt(insnIdx, insn.Regs[0], localKind(defCat.Kind))), vt)

	case insn.Opcode == 0x1a || insn.Opcode == 0x1b:
		defCat := l.definitionCategoryAt(insnIdx, insn.Regs[0])
		setSlot(frame, int(l.storeSlotAt(insnIdx, insn.Regs[0], localKind(defCat.Kind))),
			classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyObject, Class: "java/lang/String"})

	case insn.Opcode == 0x1c:
		defCat := l.definitionCategoryAt(insnIdx, insn.Regs[0])
		setSlot(frame, int(l.storeSlotAt(insnIdx, insn.Regs[0], localKind(defCat.Kind))),
			classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyObject, Class: "java/lang/Class"})

	case insn.Opcode == 0xfe:
		defCat := l.definitionCategoryAt(insnIdx, insn.Regs[0])
		setSlot(frame, int(l.storeSlotAt(insnIdx, insn.Regs[0], localKind(defCat.Kind))),
			classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyObject, Class: "java/lang/invoke/MethodHandle"})

	case insn.Opcode == 0xff:
		defCat := l.definitionCategoryAt(insnIdx, insn.Regs[0])
		setSlot(frame, int(l.storeSlotAt(insnIdx, insn.Regs[0], localKind(defCat.Kind))),
			classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyObject, Class: "java/lang/invoke/MethodType"})

	case insn.Opcode == 0x1f || insn.Opcode == 0x23:
		vt, _, err := verifyTypeFromDescriptor(l.file.Types[insn.Index].Descriptor)
		if err != nil {
			vt = classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyObject, Class: "java/lang/Object"}
		}
		defCat := l.definitionCategoryAt(insnIdx, insn.Regs[0])
		setSlot(frame, int(l.storeSlotAt(insnIdx, insn.Regs[0], localKind(defCat.Kind))), vt)

	case insn.Opcode == 0x20 || insn.Opcode == 0x21 ||
		(insn.Opcode >= 0x2d && insn.Opcode <= 0x31) ||
		(insn.Opcode >= 0x47 && insn.Opcode <= 0x4a):
		defCat := l.definitionCategoryAt(insnIdx, insn.Regs[0])
		setSlot(frame, int(l.storeSlotAt(insnIdx, insn.Regs[0], localKind(defCat.Kind))),
			classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyInt})

	case insn.Opcode == 0x24 || insn.Opcode == 0x25:
		if insnIdx+1 < len(l.method.Instructions) && l.method.Instructions[insnIdx+1].Opcode == 0x0c {
			next := &l.method.Instructions[insnIdx+1]
			vt, _, err := verifyTypeFromDescriptor(l.file.Types[insn.Index].Descriptor)
			if err != nil {
				vt = classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyObject, Class: "java/lang/Object"}
			}
			defCat := l.definitionCategoryAt(insnIdx+1, next.Regs[0])
			setSlot(frame, int(l.storeSlotAt(insnIdx+1, next.Regs[0], localKind(defCat.Kind))), vt)
		}

	case insn.Opcode == 0x44:
		vt := classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyInt}
		arr := l.method.CategoryAt(insnIdx, insn.Regs[1])
		if strings.HasPrefix(arr.Descriptor, "[") && arr.Descriptor[1:] == "F" {
			vt = classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyFloat}
		}
		defCat := l.definitionCategoryAt(insnIdx, insn.Regs[0])
		setSlot(frame, int(l.storeSlotAt(insnIdx, insn.Regs[0], localKind(defCat.Kind))), vt)

	case insn.Opcode == 0x45:
		vt := classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyLong}
		arr := l.method.CategoryAt(insnIdx, insn.Regs[1])
		if strings.HasPrefix(arr.Descriptor, "[") && arr.Descriptor[1:] == "D" {
			vt = classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyDouble}
		}
		defCat := l.definitionCategoryAt(insnIdx, insn.Regs[0])
		setSlot(frame, int(l.storeSlotAt(insnIdx, insn.Regs[0], localKind(defCat.Kind))), vt)

	case insn.Opcode == 0x46:
		vt := classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyObject, Class: "java/lang/Object"}
		arrSlot := int(l.loadSlotAt(insnIdx, insn.Regs[1], classfilebuilder.RefLocal))
		if arrSlot >= 0 && arrSlot < len(frame) && frame[arrSlot].vt.Tag == classfilebuilder.VerifyNull {
			vt = classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyNull}
		} else if arrSlot >= 0 && arrSlot < len(frame) && frame[arrSlot].vt.Tag == classfilebuilder.VerifyObject &&
			strings.HasPrefix(frame[arrSlot].vt.Class, "[") && len(frame[arrSlot].vt.Class) >= 2 {
			if elemVT, _, err := verifyTypeFromDescriptor(frame[arrSlot].vt.Class[1:]); err == nil {
				vt = elemVT
			}
		} else if arr := l.method.CategoryAt(insnIdx, insn.Regs[1]); arr.Kind == ir.Null || (arr.Kind == ir.Reference && arr.Descriptor == "") {
			vt = classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyNull}
		} else if strings.HasPrefix(arr.Descriptor, "[") && len(arr.Descriptor) >= 2 {
			if elemVT, _, err := verifyTypeFromDescriptor(arr.Descriptor[1:]); err == nil {
				vt = elemVT
			}
		}
		defCat := l.definitionCategoryAt(insnIdx, insn.Regs[0])
		setSlot(frame, int(l.storeSlotAt(insnIdx, insn.Regs[0], localKind(defCat.Kind))), vt)

	case (insn.Opcode >= 0x52 && insn.Opcode <= 0x58) || (insn.Opcode >= 0x60 && insn.Opcode <= 0x66):
		field := l.file.Fields[insn.Index]
		vt, _, err := verifyTypeFromDescriptor(field.Type)
		if err != nil {
			vt = verifyTypeFromCategory(l.definitionCategoryAt(insnIdx, insn.Regs[0]))
		}
		defCat := l.definitionCategoryAt(insnIdx, insn.Regs[0])
		setSlot(frame, int(l.storeSlotAt(insnIdx, insn.Regs[0], localKind(defCat.Kind))), vt)

	case isInvoke(insn.Opcode) || (insn.Opcode >= 0xfa && insn.Opcode <= 0xfd):
		if insnIdx+1 < len(l.method.Instructions) {
			next := &l.method.Instructions[insnIdx+1]
			if next.Opcode == 0x0a || next.Opcode == 0x0b || next.Opcode == 0x0c {
				var retType string
				switch {
				case insn.Opcode == 0xfa || insn.Opcode == 0xfb:
					retType = l.file.Protos[insn.ProtoIndex].ReturnType
				case insn.Opcode == 0xfc || insn.Opcode == 0xfd:
					retType = l.file.Protos[l.file.CallSites[insn.Index].MethodTypeIndex].ReturnType
				default:
					retType = l.file.Protos[l.file.Methods[insn.Index].ProtoIndex].ReturnType
				}
				if retType != "" && retType != "V" {
					vt, _, err := verifyTypeFromDescriptor(retType)
					if err != nil {
						vt = verifyTypeFromCategory(l.definitionCategoryAt(insnIdx+1, next.Regs[0]))
					}
					defCat := l.definitionCategoryAt(insnIdx+1, next.Regs[0])
					setSlot(frame, int(l.storeSlotAt(insnIdx+1, next.Regs[0], localKind(defCat.Kind))), vt)
				}
			}
		}

	case (insn.Opcode >= 0x7b && insn.Opcode <= 0x8f) || (insn.Opcode >= 0x90 && insn.Opcode <= 0xe2):
		defCat := l.definitionCategoryAt(insnIdx, insn.Regs[0])
		vt := verifyTypeFromCategory(defCat)
		setSlot(frame, int(l.storeSlotAt(insnIdx, insn.Regs[0], localKind(defCat.Kind))), vt)
	}
}

func (l *methodLowerer) computeStackMapFrames() (*stackMapState, error) {
	if len(l.method.Blocks) == 0 {
		return &stackMapState{}, nil
	}
	totalSlots := int(l.homeBase + l.homeLimit + l.tempLimit)
	entryFrame := make(slotFrame, totalSlots)
	for i := range entryFrame {
		entryFrame[i] = topSlotEntry()
	}

	slot := 0
	if l.encoded.AccessFlags&dexStatic == 0 {
		if l.encoded.Method.Name == "<init>" {
			setSlot(entryFrame, slot, classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyUninitializedThis})
		} else {
			thisName, err := objectInternalName(l.class.Descriptor)
			if err != nil {
				thisName = "java/lang/Object"
			}
			setSlot(entryFrame, slot, classfilebuilder.VerificationType{Tag: classfilebuilder.VerifyObject, Class: thisName})
		}
		slot++
	}
	for _, descriptor := range l.method.Proto.Parameters {
		vt, wide, err := verifyTypeFromDescriptor(descriptor)
		if err != nil {
			return nil, methodDiagnostic(l.method.Context, nil, "stackmap", err)
		}
		setSlot(entryFrame, slot, vt)
		slot++
		if wide {
			slot++
		}
	}

	// Mirror emitPrologue() stores before block 0 runs.
	paramReg := l.method.Code.RegistersSize - l.method.Code.InsSize
	jvmLocal := 0
	if l.encoded.AccessFlags&dexStatic == 0 {
		if int(paramReg) >= len(l.directParamSlots) || l.directParamSlots[paramReg] < 0 {
			setSlot(entryFrame, int(l.prologueStoreSlot(paramReg, classfilebuilder.RefLocal)), entryFrame[jvmLocal].vt)
		}
		paramReg++
		jvmLocal++
	}
	for _, descriptor := range l.method.Proto.Parameters {
		kind, err := fieldKind(descriptor)
		if err != nil {
			return nil, methodDiagnostic(l.method.Context, nil, "stackmap", err)
		}
		words := kind.Words()
		if int(paramReg) >= len(l.directParamSlots) || l.directParamSlots[paramReg] < 0 {
			setSlot(entryFrame, int(l.prologueStoreSlot(paramReg, localKind(kind))), entryFrame[jvmLocal].vt)
		}
		paramReg += uint16(words)
		jvmLocal += words
	}

	deferred := make(map[int]*deferredStore, len(l.deferredStores))
	for i := range l.deferredStores {
		deferred[l.deferredStores[i].index] = &l.deferredStores[i]
	}

	blockByStart := make(map[uint32]int, len(l.method.Blocks))
	for bi, block := range l.method.Blocks {
		blockByStart[block.Start] = bi
	}

	catchClassByAddr := make(map[uint32]string)
	handlersForInsn := make([][]int, len(l.method.Instructions))
	for _, try := range l.method.Code.Tries {
		var tryHandlers []uint32
		seenAddr := make(map[uint32]bool)
		addAddr := func(addr uint32, catchDesc string) {
			if catchDesc == "" {
				catchClassByAddr[addr] = "java/lang/Throwable"
			} else {
				name, err := objectInternalName(catchDesc)
				if err != nil {
					name = "java/lang/Throwable"
				}
				if existing, ok := catchClassByAddr[addr]; !ok {
					catchClassByAddr[addr] = name
				} else {
					catchClassByAddr[addr] = l.mergeCatchClass(existing, name)
				}
			}
			if !seenAddr[addr] {
				seenAddr[addr] = true
				tryHandlers = append(tryHandlers, addr)
			}
		}
		for _, handler := range try.Handler.Handlers {
			addAddr(handler.Address, handler.Type)
		}
		if try.Handler.CatchAllAddress != nil {
			addAddr(*try.Handler.CatchAllAddress, "")
		}
		startAddr := try.StartAddress
		endAddr := try.StartAddress + uint32(try.InstructionCount)
		for idx := range l.method.Instructions {
			insn := &l.method.Instructions[idx]
			off := insn.Offset
			alloc, isNew := l.allocationByNew[idx]
			if off >= startAddr && off < endAddr && ir.MayThrow(insn.Opcode) && !(isNew && alloc.Fused) && !l.deferredInsn[idx] {
				for _, addr := range tryHandlers {
					if hb, ok := blockByStart[addr]; ok {
						handlersForInsn[idx] = append(handlersForInsn[idx], hb)
					}
				}
			}
		}
	}

	numBlocks := len(l.method.Blocks)
	in := make([]slotFrame, numBlocks)
	out := make([]slotFrame, numBlocks)
	visited := make([]bool, numBlocks)
	in[0] = cloneSlotFrame(entryFrame)
	visited[0] = true

	inQueue := make([]bool, numBlocks)
	worklist := []int{0}
	inQueue[0] = true

	maxSteps := numBlocks*64 + 1024
	steps := 0
	for len(worklist) > 0 {
		steps++
		if steps > maxSteps {
			return nil, methodDiagnostic(l.method.Context, nil, "stackmap", fmt.Errorf("stack map frame analysis did not converge"))
		}
		b := worklist[0]
		worklist = worklist[1:]
		inQueue[b] = false

		st := cloneSlotFrame(in[b])
		for _, insnIdx := range l.method.Blocks[b].Instructions {
			if handlers := handlersForInsn[insnIdx]; len(handlers) > 0 {
				exState := st
				if alloc, ok := l.allocationByCtor[insnIdx]; ok && !alloc.Fused {
					newLabel := l.allocationNewLabels[alloc.ID]
					exState = cloneSlotFrame(st)
					for i := range exState {
						if exState[i].vt.Tag == classfilebuilder.VerifyUninitialized && exState[i].vt.NewAt == newLabel {
							exState[i] = topSlotEntry()
						}
					}
				} else if l.isDelegationSite(insnIdx) {
					exState = cloneSlotFrame(st)
					for i := range exState {
						if exState[i].vt.Tag == classfilebuilder.VerifyUninitializedThis {
							exState[i] = topSlotEntry()
						}
					}
				}
				for _, hb := range handlers {
					joined, changed := l.joinSlotFrames(in[hb], visited[hb], exState)
					visited[hb] = true
					in[hb] = joined
					if changed && !inQueue[hb] {
						inQueue[hb] = true
						worklist = append(worklist, hb)
					}
				}
			}
			l.transferSlotFrame(b, insnIdx, st, deferred, catchClassByAddr)
		}
		out[b] = st
		for _, succ := range l.method.Blocks[b].Successors {
			if !l.blockHasNormalEdgeTo(b, succ) {
				continue
			}
			joined, changed := l.joinSlotFrames(in[succ], visited[succ], st)
			visited[succ] = true
			in[succ] = joined
			if changed && !inQueue[succ] {
				inQueue[succ] = true
				worklist = append(worklist, succ)
			}
		}
	}

	return &stackMapState{
		entryFrame:       entryFrame,
		in:               in,
		visited:          visited,
		blockByStart:     blockByStart,
		catchClassByAddr: catchClassByAddr,
		deferred:         deferred,
	}, nil
}

func (l *methodLowerer) emitStackMapTable(state *stackMapState) error {
	if len(l.method.Blocks) == 0 {
		return nil
	}
	if state == nil {
		var err error
		state, err = l.computeStackMapFrames()
		if err != nil {
			return err
		}
	}
	entryFrame := state.entryFrame
	in := state.in
	visited := state.visited
	blockByStart := state.blockByStart
	catchClassByAddr := state.catchClassByAddr
	deferred := state.deferred
	numBlocks := len(l.method.Blocks)

	blockEmitsBytes := make([]bool, numBlocks)
	for b, block := range l.method.Blocks {
		for _, insnIdx := range block.Instructions {
			if store, ok := deferred[insnIdx]; ok {
				if !store.direct {
					blockEmitsBytes[b] = true
					break
				}
				continue
			}
			if alloc, ok := l.allocationByNew[insnIdx]; ok && alloc.Fused {
				continue
			}
			if l.allocationAliasOps[insnIdx] {
				continue
			}
			op := l.method.Instructions[insnIdx].Opcode
			if op >= 0x0a && op <= 0x0c {
				continue
			}
			if op >= 0x01 && op <= 0x09 {
				kind := l.method.CategoryAt(insnIdx, l.method.Instructions[insnIdx].Regs[0]).Kind
				if def, ok := l.method.DefinitionAt(insnIdx); ok && def.Register == int32(l.method.Instructions[insnIdx].Regs[0]) {
					kind = def.Category.Kind
				}
				if kind == ir.Unknown {
					kind = l.method.CategoryAt(insnIdx, l.method.Instructions[insnIdx].Regs[1]).Kind
				}
				if kind == ir.Unknown {
					continue
				}
			}
			blockEmitsBytes[b] = true
			break
		}
	}

	targetBlocks := make([]bool, numBlocks)
	for _, insn := range l.method.Instructions {
		if isBranchOpcode(insn.Opcode) && insn.Target >= 0 {
			if tb, ok := blockByStart[uint32(insn.Target)]; ok {
				targetBlocks[tb] = true
			}
		} else if insn.Opcode == 0x2b || insn.Opcode == 0x2c {
			if tb, ok := blockByStart[insn.End()]; ok {
				targetBlocks[tb] = true
			}
			for _, t := range insn.Targets {
				if tb, ok := blockByStart[uint32(t)]; ok {
					targetBlocks[tb] = true
				}
			}
		}
	}
	handlerBlocks := make([]bool, numBlocks)
	for b, block := range l.method.Blocks {
		if _, isHandler := catchClassByAddr[block.Start]; isHandler {
			handlerBlocks[b] = true
			if _, hasTrampoline := l.handlerTrampolines[block.Start]; hasTrampoline {
				// Trampoline itself branches to block.Start.
				targetBlocks[b] = true
			}
		}
		if len(block.Instructions) > 0 && b+1 < numBlocks {
			lastOp := l.method.Instructions[block.Instructions[len(block.Instructions)-1]].Opcode
			if lastOp == 0x27 || (lastOp >= 0x0e && lastOp <= 0x11) || (lastOp >= 0x28 && lastOp <= 0x2c) {
				targetBlocks[b+1] = true
			}
		}
	}

	resolvedTarget := make([]int, numBlocks)
	for i := range resolvedTarget {
		resolvedTarget[i] = -1
	}
	resolvedLocals := make([]slotFrame, numBlocks)
	resolvedVisited := make([]bool, numBlocks)
	resolveBlock := func(b int) int {
		for k := b; k < numBlocks; k++ {
			if blockEmitsBytes[k] {
				return k
			}
		}
		return -1
	}

	for b := 0; b < numBlocks; b++ {
		if !targetBlocks[b] && !handlerBlocks[b] {
			continue
		}
		frame := in[b]
		hasFrame := visited[b]
		if !hasFrame {
			frame = cloneSlotFrame(entryFrame)
		}
		if handlerBlocks[b] {
			block := l.method.Blocks[b]
			catchCls := catchClassByAddr[block.Start]
			if catchCls == "" {
				catchCls = "java/lang/Throwable"
			}
			stackTop := []classfilebuilder.VerificationType{{Tag: classfilebuilder.VerifyObject, Class: catchCls}}
			if trampoline, ok := l.handlerTrampolines[block.Start]; ok {
				l.code.AddFrame(trampoline, compactStackMapLocals(frame), stackTop)
			} else if len(block.Instructions) > 0 && l.method.Instructions[block.Instructions[0]].Opcode == 0x0d {
				k := resolveBlock(b)
				if k >= 0 {
					l.code.AddFrame(l.labels[l.method.Blocks[k].Start], compactStackMapLocals(frame), stackTop)
				}
				continue
			}
		}
		if !targetBlocks[b] {
			continue
		}
		k := resolveBlock(b)
		if k < 0 {
			continue
		}
		resolvedTarget[k] = k
		joined, _ := l.joinSlotFrames(resolvedLocals[k], resolvedVisited[k], frame)
		resolvedVisited[k] = true
		resolvedLocals[k] = joined
	}

	for k := 0; k < numBlocks; k++ {
		if resolvedTarget[k] < 0 {
			continue
		}
		l.code.AddFrame(l.labels[l.method.Blocks[k].Start], compactStackMapLocals(resolvedLocals[k]), nil)
	}
	return nil
}

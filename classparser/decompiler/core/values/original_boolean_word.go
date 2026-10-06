package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Field descriptors distinguish an inherently normalized Z read from an int
// value that only acquired a Boolean source type through inference.
type originalFieldRead struct {
	pc                      int
	owner, name, descriptor string
}

func originalRead(member *JavaClassMember, pc int) *originalFieldRead {
	if member == nil || pc < 0 {
		return nil
	}
	return &originalFieldRead{pc, member.Name, member.Member, member.Description}
}
func (f *RefMember) MarkOriginalFieldRead(member *JavaClassMember, pc int) {
	if f != nil && f.originalFieldRead == nil {
		f.originalFieldRead = originalRead(member, pc)
	}
}
func (f *JavaClassMember) MarkOriginalFieldRead(member *JavaClassMember, pc int) {
	if f != nil && f.originalFieldRead == nil {
		f.originalFieldRead = originalRead(member, pc)
	}
}

// OriginalStaticFieldRead binds the rendering view to the decoded GETSTATIC
// witness. A later name/descriptor/origin mutation cannot manufacture this proof.
func (f *JavaClassMember) OriginalStaticFieldRead(pc int, owner, name, descriptor string) bool {
	if f == nil || f.originalFieldRead == nil || !f.HasOriginPC || f.RefKind != 0 {
		return false
	}
	w := f.originalFieldRead
	return f.OriginPC == pc && w.pc == pc && w.owner == owner && w.name == name && w.descriptor == descriptor && f.Name == owner && f.Member == name && f.Description == descriptor
}

// OriginalBooleanStackWord proves a source Boolean whose JVM value is0/1.
// A shared DUP value is followed only through its immutable producer witness;
// arbitrary Boolean-typed locals do not supply evidence. This view adds no
// runtime check and evaluates the same value once at an int consumer.
func OriginalBooleanStackWord(value JavaValue) bool {
	active := map[JavaValue]bool{}
	work := 0
	var visit func(JavaValue) bool
	visit = func(v JavaValue) bool {
		if isNilJavaValue(v) || work >= 64 || active[v] || !isBooleanTyped(v) {
			return false
		}
		work++
		active[v] = true
		defer delete(active, v)
		switch x := UnpackSoltValue(v).(type) {
		case *RefMember:
			w := x.originalFieldRead
			return w != nil && w.descriptor == "Z" && x.HasOriginPC && x.OriginPC == w.pc && x.Member == w.name && !isNilJavaValue(x.Object)
		case *JavaClassMember:
			w := x.originalFieldRead
			return w != nil && w.descriptor == "Z" && x.HasOriginPC && x.OriginPC == w.pc && x.Name == w.owner && x.Member == w.name && x.Description == w.descriptor
		case *JavaRef:
			_, _, known := x.OriginalStackMaterializationWitness(x.Val)
			return known && visit(x.Val)
		case *JavaLiteral:
			switch word := x.Data.(type) {
			case bool:
				return true
			case int:
				return word == 0 || word == 1
			}
		case *FunctionCallExpression:
			_, result, err := callbinding.Descriptor(x.Descriptor)
			return err == nil && result == "Z" && x.HasOriginPC && x.OriginPC >= 0 && x.FuncType != nil && x.FuncType.ReturnType != nil && booleanSourceType(x.FuncType.ReturnType)
		}
		return false
	}
	return visit(value)
}
func booleanSourceType(t types.JavaType) bool {
	if t == nil {
		return false
	}
	p, ok := t.RawType().(*types.JavaPrimer)
	return ok && p != nil && p.Name == types.JavaBoolean
}

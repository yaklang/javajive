package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/fieldbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A store target must never acquire a GETFIELD/boolean-value witness. Its
// separate immutable anchor only identifies the physical field selection.
func (f *RefMember) MarkOriginalFieldWrite(member *JavaClassMember, pc int) {
	if f != nil && f.originalFieldWrite == nil && f.originalFieldRead == nil && member != nil && f.Member == member.Member {
		f.originalFieldWrite = originalRead(member, pc)
		f.OriginPC, f.HasOriginPC = pc, true
	}
}
func (f *RefMember) sourceFieldReceiver(ctx *class_context.ClassContext) JavaValue {
	if f == nil || ctx == nil || ctx.OriginalFieldDeclarations == nil || ctx.InvocationMetadata == nil || isNilJavaValue(f.Object) || !f.HasOriginPC || f.OriginPC < 0 || f.OriginPC > 65535 || class_context.SafeIdentifier(f.Member) != f.Member {
		return nil
	}
	w := f.originalFieldRead
	if w == nil {
		w = f.originalFieldWrite
	}
	if w == nil || w.pc != f.OriginPC || w.name != f.Member {
		return nil
	}
	receiver, known := types.RawClassFQN(f.Object.Type())
	if !known {
		return nil
	}
	receiver = strings.ReplaceAll(receiver, ".", "/")
	owner := strings.ReplaceAll(w.owner, ".", "/")
	caller := strings.ReplaceAll(ctx.ClassName, ".", "/")
	if receiver == owner {
		return nil
	}
	widen := func(a, b string) bool { return callbinding.Assignable("L"+a+";", "L"+b+";", ctx.InvocationMetadata) }
	cast, known := fieldbinding.Selection(ctx.OriginalFieldDeclarations, widen, receiver, owner, w.name, w.descriptor, caller, ctx.Work)
	if !known || cast == "" || !strings.Contains(cast, "/") && ctx.IsTypeParam(cast) {
		return nil
	}
	return &CastExpression{Value: f.Object, TargetType: types.NewJavaClass(strings.ReplaceAll(cast, "/", ".")), Binding: true}
}

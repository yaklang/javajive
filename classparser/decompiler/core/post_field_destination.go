package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"reflect"
)

// A retained old word proves post-update syntax only when the write targets
// the same original field and receiver as its read. A legal JVM packet may
// read one field/receiver, write another, and still return the retained word.
// Keep that packet as an explicit assignment instead of inventing an update.
func originalPostFieldTarget(old, destination values.JavaValue, member *values.JavaClassMember) values.JavaValue {
	if member == nil {
		return nil
	}
	target := safeSeedOperand(old)
	if ref, ok := target.(*values.JavaRef); ok {
		if ref == nil {
			return nil
		}
		seed := ref.Val
		_, code, known := ref.OriginalStackMaterializationWitness(seed)
		if !known || code != OP_DUP && code != OP_DUP_X1 && code != OP_DUP2 && code != OP_DUP2_X1 {
			return nil
		}
		target = safeSeedOperand(seed)
	}
	switch read := target.(type) {
	case *values.RefMember:
		write, ok := destination.(*values.RefMember)
		if !ok || read == nil || write == nil || !read.HasOriginPC || !read.OriginalInstanceFieldRead(read.OriginPC, member.Name, member.Member, member.Description) || write.Member != member.Member || read.Object == nil || write.Object == nil {
			return nil
		}
		// Exhausting the bounded forwarding proof yields no identity. Two
		// unresolved operands are not evidence of the same receiver.
		receiver := values.OriginalStackLifetimeUse(read.Object)
		if receiver == nil || reflect.ValueOf(receiver).Kind() == reflect.Ptr && reflect.ValueOf(receiver).IsNil() || receiver != values.OriginalStackLifetimeUse(write.Object) {
			return nil
		}
	case *values.JavaClassMember:
		write, ok := destination.(*values.JavaClassMember)
		if !ok || read == nil || write == nil || !read.HasOriginPC || !read.OriginalStaticFieldRead(read.OriginPC, member.Name, member.Member, member.Description) || write.Name != member.Name || write.Member != member.Member || write.Description != member.Description {
			return nil
		}
	default:
		return nil
	}
	return target
}

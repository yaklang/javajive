package javaclassparser

import (
	"encoding/binary"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
)

// Regenerating the capturing class's helper is a class-wide operation. Its
// other emitted factories must not add serialization routes missing from the
// verified original decision automaton. Unused CP constants are irrelevant.
func (c *ClassObjectDumper) serializableOtherFactoriesClosed(plan *serializableReferenceProtocol) bool {
	if plan == nil || plan.object != c.obj {
		return false
	}
	var bootstrap *BootstrapMethodsAttribute
	for _, attribute := range c.obj.Attributes {
		if table, ok := attribute.(*BootstrapMethodsAttribute); ok {
			if table == nil || bootstrap != nil {
				return false
			}
			bootstrap = table
		}
	}
	if bootstrap == nil {
		return false
	}
	provider := c.buildInvocationMetadata()
	totalBytes := 0
	for _, method := range c.obj.Methods {
		if method == nil {
			return false
		}
		if method == plan.helper {
			continue
		}
		code, ops, known := serializableOriginalCode(c.obj, method, c.Work)
		if !known {
			return false
		}
		if code == nil {
			continue
		}
		totalBytes += len(code.Code)
		if totalBytes > 1<<20 {
			return false
		}
		for _, op := range ops {
			if !nativeEnumOpcode(op, core.OP_INVOKEDYNAMIC) {
				continue
			}
			if !nativeProofWork(c.Work, int64(len(plan.sites))+1) || len(op.Data) != 4 {
				return false
			}
			owned := false
			for _, site := range plan.sites {
				owned = owned || site.method == method && site.code == code && site.pc == int(op.CurrentOffset)
			}
			if owned {
				continue
			}
			dynamic, known := serializableConstant(c.obj, binary.BigEndian.Uint16(op.Data)).(*ConstantInvokeDynamicInfo)
			if !known || dynamic == nil || int(dynamic.BootstrapMethodAttrIndex) >= len(bootstrap.BootstrapMethods) {
				return false
			}
			method := bootstrap.BootstrapMethods[dynamic.BootstrapMethodAttrIndex]
			if method == nil {
				return false
			}
			handle, _ := serializableConstant(c.obj, method.BootstrapMethodRef).(*ConstantMethodHandleInfo)
			target, known := nativeLambdaOriginalHandleTarget(c.obj, handle)
			if !known {
				return false
			}
			if target.owner != "java/lang/invoke/LambdaMetafactory" {
				continue
			}
			// Other alt-metafactory domains require their own helper/source
			// certificate; accepting their flags here would widen this one.
			if target.name != "metafactory" || target.interfaceRef || handle.ReferenceKind != 6 || target.descriptor != "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodHandle;Ljava/lang/invoke/MethodType;)Ljava/lang/invoke/CallSite;" || len(method.BootstrapArguments) != 3 {
				return false
			}
			nat, known := serializableConstant(c.obj, dynamic.NameAndTypeIndex).(*ConstantNameAndTypeInfo)
			if !known || nat == nil {
				return false
			}
			descriptor, known := sourceBridgeUTF8(c.obj, nat.DescriptorIndex)
			_, result, err := callbinding.Descriptor(descriptor)
			if !known || err != nil || len(result) < 3 || result[0] != 'L' || result[len(result)-1] != ';' {
				return false
			}
			owner := strings.TrimSuffix(strings.TrimPrefix(result, "L"), ";")
			serializable, complete := core.LambdaInterfaceSerializable(provider, owner, c.Work)
			if serializable || !complete {
				return false
			}
		}
	}
	return true
}

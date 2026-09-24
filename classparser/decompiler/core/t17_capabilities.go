package core

import "fmt"

// Frozen JDK bootstrap identities. RefKind is always invokeStatic (6).

var IdentityMakeConcat = BootstrapIdentity{
	Owner:      "java.lang.invoke.StringConcatFactory",
	Name:       "makeConcat",
	Descriptor: "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;)Ljava/lang/invoke/CallSite;",
	RefKind:    RefInvokeStatic,
}

var IdentityMakeConcatWithConstants = BootstrapIdentity{
	Owner:      "java.lang.invoke.StringConcatFactory",
	Name:       "makeConcatWithConstants",
	Descriptor: "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;Ljava/lang/String;[Ljava/lang/Object;)Ljava/lang/invoke/CallSite;",
	RefKind:    RefInvokeStatic,
}

var IdentityLambdaMetafactory = BootstrapIdentity{
	Owner:      "java.lang.invoke.LambdaMetafactory",
	Name:       "metafactory",
	Descriptor: "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodHandle;Ljava/lang/invoke/MethodType;)Ljava/lang/invoke/CallSite;",
	RefKind:    RefInvokeStatic,
}

var IdentityLambdaAltMetafactory = BootstrapIdentity{
	Owner:      "java.lang.invoke.LambdaMetafactory",
	Name:       "altMetafactory",
	Descriptor: "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;[Ljava/lang/Object;)Ljava/lang/invoke/CallSite;",
	RefKind:    RefInvokeStatic,
}

var IdentityObjectMethods = BootstrapIdentity{
	Owner:      "java.lang.runtime.ObjectMethods",
	Name:       "bootstrap",
	Descriptor: "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/TypeDescriptor;Ljava/lang/Class;Ljava/lang/String;[Ljava/lang/invoke/MethodHandle;)Ljava/lang/Object;",
	RefKind:    RefInvokeStatic,
}

var IdentityTypeSwitch = BootstrapIdentity{
	Owner:      "java.lang.runtime.SwitchBootstraps",
	Name:       "typeSwitch",
	Descriptor: "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;[Ljava/lang/Object;)Ljava/lang/invoke/CallSite;",
	RefKind:    RefInvokeStatic,
}

var IdentityEnumSwitch = BootstrapIdentity{
	Owner:      "java.lang.runtime.SwitchBootstraps",
	Name:       "enumSwitch",
	Descriptor: "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;[Ljava/lang/Object;)Ljava/lang/invoke/CallSite;",
	RefKind:    RefInvokeStatic,
}

// Capability describes whether a family can be reconstructed losslessly at a source level.
type Capability struct {
	Family              FeatureFamily
	MinSourceVersion    int
	MinClassMajor       uint16
	Lossless            bool
	TargetSourceVersion int
	Notes               string
}

// MinLosslessSource is the language level required to express the feature without loss.
func MinLosslessSource(family FeatureFamily) int {
	switch family {
	case FamilyConcat:
		return 9
	case FamilyLambda:
		return 8
	case FamilyRecord:
		return 16
	case FamilyTypeSwitch, FamilyEnumSwitch:
		return 21
	case FamilyCondy:
		return 11
	default:
		return 1 << 30
	}
}

func MinClassMajor(family FeatureFamily) uint16 {
	src := MinLosslessSource(family)
	if src <= 8 {
		return 52
	}
	return uint16(src + 44)
}

// ClassMajorToSourceVersion maps a class-file major to javac --release. Majors
// below 52 still map to 8. Unknown future majors are returned as major-44 and
// are NOT treated as "latest known Java" for semantic interpretation.
func ClassMajorToSourceVersion(major uint16) int {
	if major == 0 || major < 52 {
		return 8
	}
	return int(major) - 44
}

// EffectiveTargetSource is TargetSourceVersion, or the class major mapping when zero.
func EffectiveTargetSource(target int, classMajor uint16) int {
	if target > 0 {
		return target
	}
	return ClassMajorToSourceVersion(classMajor)
}

// EvaluateCapability reports whether reconstruction at target is lossless.
// Concat `+` is representable at 8 even though indy concat is a 9+ class shape;
// the family is still listed min 9 for indy-faithful form, but emitting `+` is
// allowed at 8 (T18 compiler-shape variants). Record and pattern-switch are not.
func EvaluateCapability(family FeatureFamily, targetSource int, classMajor uint16) Capability {
	target := EffectiveTargetSource(targetSource, classMajor)
	min := MinLosslessSource(family)
	cap := Capability{
		Family:              family,
		MinSourceVersion:    min,
		MinClassMajor:       MinClassMajor(family),
		TargetSourceVersion: target,
		Lossless:            true,
	}
	switch family {
	case FamilyConcat:
		// Source `+` exists at 8; indy recipe exists at 9+. Both are allowed.
		cap.Lossless = target >= 8
		if !cap.Lossless {
			cap.Notes = fmt.Sprintf("string concat not representable at source %d", target)
		}
	case FamilyLambda:
		cap.Lossless = target >= 8
		if !cap.Lossless {
			cap.Notes = fmt.Sprintf("lambda metafactory not representable at source %d", target)
		}
	case FamilyRecord:
		cap.Lossless = target >= 16
		if !cap.Lossless {
			cap.Notes = fmt.Sprintf("record/ObjectMethods cannot be lossless at source %d (need 16+); isRecord would differ", target)
		}
	case FamilyTypeSwitch, FamilyEnumSwitch:
		cap.Lossless = target >= 21
		if !cap.Lossless {
			cap.Notes = fmt.Sprintf("pattern/enum switch cannot be lossless at source %d (need 21+)", target)
		}
	case FamilyCondy:
		cap.Lossless = false
		cap.Notes = "ConstantDynamic has no proven reconstruction; legal condy is unsupported"
	default:
		cap.Lossless = false
		cap.Notes = "unknown bootstrap family"
	}
	return cap
}

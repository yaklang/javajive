package javaclassparser

// This is a metadata-format permission, not executable lowering. Java8 source
// can be implicitly recompiled in an MR9/10 pass while its hidden constructor
// roles still use the legacy, pre-nest access ABI. Only the explicit physical
// reconstruction client can select that output target. Ordinary source APIs
// retain the original target52 contract. JVMS4.7.24 defines the same parameter
// roles from52; JVM nests start at55 and need their own source/access proof.
type sourceCaptureMetadataTarget struct{ major uint16 }

func (target sourceCaptureMetadataTarget) matches(original, fresh *ClassObject) bool {
	major := target.major
	if major == 0 {
		major = 52
	}
	return original != nil && fresh != nil && major >= 52 && major < 55 &&
		original.MajorVersion == 52 && fresh.MajorVersion == major &&
		original.MinorVersion == 0 && fresh.MinorVersion == 0 &&
		!hasOriginalNestAttribute(original, "NestHost") && !hasOriginalNestAttribute(original, "NestMembers") &&
		!hasOriginalNestAttribute(fresh, "NestHost") && !hasOriginalNestAttribute(fresh, "NestMembers")
}

func reconstructionCaptureMetadataTarget(jar *JarFS) sourceCaptureMetadataTarget {
	if jar == nil || jar.sourceConstructorLowering == nil || jar.sourceCompiler.effective() != ModernJavac || jar.targetSourceVersion != 8 || jar.jarReleaseRole != jarReleasePhysicalSource || jar.jarPhysicalRelease < 9 || jar.jarPhysicalRelease >= 11 {
		return sourceCaptureMetadataTarget{}
	}
	return sourceCaptureMetadataTarget{major: uint16(jar.jarPhysicalRelease + 44)}
}

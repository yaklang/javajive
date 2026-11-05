package javaclassparser

import "fmt"

// Supported classfile major versions. The lower bound is Java 1.1 (45); 44 and
// below are not valid JVM classfiles. The upper bound matches the newest
// language level the decompiler's source-recovery guards accept (Java 21, 65).
// Parse() stays permissive for unknown versions so callers can report a
// capability boundary; the decompile request rejects them explicitly.
const (
	MinSupportedClassMajor = 45
	MaxSupportedClassMajor = 65
)

// ValidateSupportedClassMajor rejects classfiles outside the range whose
// semantics the decompiler claims to reconstruct.
func ValidateSupportedClassMajor(major uint16) error {
	if major < MinSupportedClassMajor || major > MaxSupportedClassMajor {
		return fmt.Errorf("unsupported_classfile_version: major %d is outside supported range %d..%d",
			major, MinSupportedClassMajor, MaxSupportedClassMajor)
	}
	return nil
}

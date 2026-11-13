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

// ValidateClassMinor reports malformed major/minor combinations while keeping
// Parse permissive for future classfile versions. Since Java 12, 65535 marks a
// preview class; preview semantics are a decompiler capability boundary rather
// than malformed input.
func ValidateClassMinor(major, minor uint16) error {
	if major == 45 {
		if minor <= 3 {
			return nil
		}
		return fmt.Errorf("invalid_classfile_version: major 45 minor %d exceeds 3", minor)
	}
	if minor == 0 {
		return nil
	}
	if major >= 56 && minor == 0xffff {
		return fmt.Errorf("unsupported_classfile_preview: major %d uses preview minor 65535", major)
	}
	return fmt.Errorf("invalid_classfile_version: major %d requires minor 0, got %d", major, minor)
}

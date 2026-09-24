package mutf8

import (
	"fmt"
	"unicode"
)

// UseKind classifies CONSTANT_Utf8 contents. String fidelity is not name legality.
type UseKind int

const (
	UseString UseKind = iota
	UseInternalName
	UseUnqualifiedName
	UseFieldDescriptor
	UseMethodDescriptor
	// UseDescriptor accepts a field OR method descriptor (not a binary class name).
	// Prefer UseFieldDescriptor / UseMethodDescriptor at CP use sites.
	UseDescriptor
	UseMethodName
)

const maxArrayDimensions = 255
const maxMethodSlots = 255

// ValidateUse checks units against the grammar for kind.
func ValidateUse(units []uint16, kind UseKind) error {
	switch kind {
	case UseString:
		return nil
	case UseInternalName:
		return ValidateInternalName(units)
	case UseUnqualifiedName:
		return ValidateUnqualifiedName(units)
	case UseFieldDescriptor:
		return ValidateFieldDescriptor(units)
	case UseMethodDescriptor:
		return ValidateMethodDescriptor(units)
	case UseDescriptor:
		if ValidateFieldDescriptor(units) == nil || ValidateMethodDescriptor(units) == nil {
			return nil
		}
		return fmt.Errorf("mutf8: not a field or method descriptor")
	case UseMethodName:
		return ValidateMethodName(units)
	default:
		return fmt.Errorf("mutf8: unknown use kind %d", kind)
	}
}

// ValidateUnqualifiedName implements JVMS 4.2.2: no '.' ';' '[' '/'.
// NUL and other odd code units are legal in the class file.
func ValidateUnqualifiedName(units []uint16) error {
	if len(units) == 0 {
		return fmt.Errorf("mutf8: empty unqualified name")
	}
	for _, u := range units {
		switch u {
		case '.', ';', '[', '/':
			return fmt.Errorf("mutf8: unqualified name contains U+%04X", u)
		}
	}
	return nil
}

// ValidateMethodName is an unqualified name, or exactly <init>/<clinit>.
func ValidateMethodName(units []uint16) error {
	if unitsEqualASCII(units, "<init>") || unitsEqualASCII(units, "<clinit>") {
		return nil
	}
	if err := ValidateUnqualifiedName(units); err != nil {
		return err
	}
	for _, u := range units {
		if u == '<' || u == '>' {
			return fmt.Errorf("mutf8: method name contains '<' or '>'")
		}
	}
	return nil
}

// ValidateInternalName is a CONSTANT_Class name: slash-separated binary name
// or an array field descriptor. NUL in a segment is JVM-legal.
func ValidateInternalName(units []uint16) error {
	if len(units) == 0 {
		return fmt.Errorf("mutf8: empty internal name")
	}
	if units[0] == '[' {
		return ValidateFieldDescriptor(units)
	}
	return validateBinaryName(units)
}

func validateBinaryName(units []uint16) error {
	start := 0
	for i := 0; i <= len(units); i++ {
		if i < len(units) && units[i] != '/' {
			continue
		}
		if i == start {
			return fmt.Errorf("mutf8: empty binary-name segment")
		}
		if err := ValidateUnqualifiedName(units[start:i]); err != nil {
			return err
		}
		if i == len(units) {
			return nil
		}
		start = i + 1
	}
	return nil
}

// ValidateFieldDescriptor is JVMS 4.3.2 with at most 255 array dimensions.
func ValidateFieldDescriptor(units []uint16) error {
	n, ok := parseFieldType(units, 0)
	if !ok || n != len(units) {
		return fmt.Errorf("mutf8: invalid field descriptor")
	}
	return nil
}

// ValidateMethodDescriptor is JVMS 4.3.3 syntax plus parameter-slot count ≤ 255
// (receiver is not in the descriptor; add it at the invoke site).
func ValidateMethodDescriptor(units []uint16) error {
	slots, end, err := parseMethodDescriptor(units)
	if err != nil {
		return err
	}
	if end != len(units) {
		return fmt.Errorf("mutf8: trailing units in method descriptor")
	}
	if slots > maxMethodSlots {
		return fmt.Errorf("mutf8: method descriptor parameter slots %d > 255", slots)
	}
	return nil
}

// MethodParamSlots returns the word count of parameters in a method descriptor
// (long/double = 2). Receiver is not included.
func MethodParamSlots(units []uint16) (int, error) {
	slots, end, err := parseMethodDescriptor(units)
	if err != nil {
		return 0, err
	}
	if end != len(units) {
		return 0, fmt.Errorf("mutf8: trailing units in method descriptor")
	}
	return slots, nil
}

// InvokeSlots is MethodParamSlots plus 1 for a non-static receiver.
func InvokeSlots(units []uint16, instance bool) (int, error) {
	slots, err := MethodParamSlots(units)
	if err != nil {
		return 0, err
	}
	if instance {
		sum, ok := CheckedAdd(slots, 1)
		if !ok || sum > maxMethodSlots {
			return 0, fmt.Errorf("mutf8: invoke slots %d exceed 255", sum)
		}
		return sum, nil
	}
	if slots > maxMethodSlots {
		return 0, fmt.Errorf("mutf8: invoke slots %d exceed 255", slots)
	}
	return slots, nil
}

func parseMethodDescriptor(units []uint16) (slots, end int, err error) {
	if len(units) == 0 || units[0] != '(' {
		return 0, 0, fmt.Errorf("mutf8: method descriptor must start with '('")
	}
	i := 1
	for i < len(units) && units[i] != ')' {
		n, ok := parseFieldType(units, i)
		if !ok {
			return 0, i, fmt.Errorf("mutf8: invalid method parameter descriptor")
		}
		w := fieldSlots(units[i:n])
		sum, addOK := CheckedAdd(slots, w)
		if !addOK {
			return 0, i, fmt.Errorf("mutf8: method descriptor parameter slots overflow")
		}
		slots = sum
		i = n
	}
	if i >= len(units) || units[i] != ')' {
		return 0, i, fmt.Errorf("mutf8: unterminated method descriptor")
	}
	i++
	if i >= len(units) {
		return 0, i, fmt.Errorf("mutf8: method descriptor missing return type")
	}
	if units[i] == 'V' {
		i++
	} else {
		n, ok := parseFieldType(units, i)
		if !ok {
			return 0, i, fmt.Errorf("mutf8: invalid method return descriptor")
		}
		i = n
	}
	return slots, i, nil
}

func parseFieldType(units []uint16, i int) (int, bool) {
	if i >= len(units) {
		return i, false
	}
	dims := 0
	for i < len(units) && units[i] == '[' {
		dims++
		if dims > maxArrayDimensions {
			return i, false
		}
		i++
	}
	if i >= len(units) {
		return i, false
	}
	switch units[i] {
	case 'B', 'C', 'D', 'F', 'I', 'J', 'S', 'Z':
		return i + 1, true
	case 'L':
		i++
		start := i
		for i < len(units) && units[i] != ';' {
			i++
		}
		if i >= len(units) || i == start {
			return i, false
		}
		if validateBinaryName(units[start:i]) != nil {
			return i, false
		}
		return i + 1, true
	default:
		return i, false
	}
}

func fieldSlots(units []uint16) int {
	i := 0
	for i < len(units) && units[i] == '[' {
		i++
	}
	if i < len(units) && (units[i] == 'J' || units[i] == 'D') && i == 0 {
		return 2
	}
	return 1
}

func unitsEqualASCII(units []uint16, s string) bool {
	if len(units) != len(s) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if units[i] != uint16(s[i]) {
			return false
		}
	}
	return true
}

// JavaSourceIdentifierOK reports whether units can appear as a Java identifier
// in source. This is NOT class-file validity (NUL and '-' are legal JVM names).
func JavaSourceIdentifierOK(units []uint16) bool {
	if len(units) == 0 {
		return false
	}
	for i, u := range units {
		if u == 0 || (u >= 0xD800 && u <= 0xDFFF) {
			return false
		}
		r := rune(u)
		if i == 0 {
			if r != '_' && r != '$' && !unicode.IsLetter(r) {
				return false
			}
		} else if r != '_' && r != '$' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

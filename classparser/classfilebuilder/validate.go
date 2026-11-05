package classfilebuilder

import (
	"fmt"
	"strings"
)

const maxArrayDimensions = 255

func validateInternalName(name string) error {
	if name == "" {
		return fmt.Errorf("internal name is empty")
	}
	start := 0
	for index := 0; index <= len(name); index++ {
		if index != len(name) && name[index] != '/' {
			continue
		}
		if index == start {
			return fmt.Errorf("internal name %q has an empty component", name)
		}
		if err := validateNamePart(name[start:index], "internal name component", false); err != nil {
			return err
		}
		start = index + 1
	}
	return nil
}

func validateNamePart(name, kind string, method bool) error {
	if name == "" {
		return fmt.Errorf("%s is empty", kind)
	}
	for _, forbidden := range ".;[/" {
		if strings.ContainsRune(name, forbidden) {
			return fmt.Errorf("%s %q contains forbidden character %q", kind, name, forbidden)
		}
	}
	if method && (strings.ContainsRune(name, '<') || strings.ContainsRune(name, '>')) {
		return fmt.Errorf("%s %q contains forbidden angle bracket", kind, name)
	}
	return nil
}

func validateFieldName(name string) error {
	return validateNamePart(name, "field name", false)
}

func validateMethodName(name string) error {
	if name == "<init>" || name == "<clinit>" {
		return nil
	}
	return validateNamePart(name, "method name", true)
}

func validateClassConstantName(name string) error {
	if strings.HasPrefix(name, "[") {
		end, _, err := parseFieldType(name, 0, false)
		if err != nil {
			return fmt.Errorf("invalid array class name %q: %w", name, err)
		}
		if end != len(name) {
			return fmt.Errorf("invalid array class name %q: trailing data", name)
		}
		return nil
	}
	return validateInternalName(name)
}

func fieldDescriptorWords(descriptor string) (int, error) {
	end, words, err := parseFieldType(descriptor, 0, false)
	if err != nil {
		return 0, fmt.Errorf("invalid field descriptor %q: %w", descriptor, err)
	}
	if end != len(descriptor) {
		return 0, fmt.Errorf("invalid field descriptor %q: trailing data", descriptor)
	}
	return words, nil
}

func methodDescriptorWords(descriptor string) (args, result int, err error) {
	if len(descriptor) < 3 || descriptor[0] != '(' {
		return 0, 0, fmt.Errorf("invalid method descriptor %q", descriptor)
	}
	i := 1
	for i < len(descriptor) && descriptor[i] != ')' {
		var words int
		i, words, err = parseFieldType(descriptor, i, false)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid method descriptor %q: %w", descriptor, err)
		}
		args += words
		if args > 255 {
			return 0, 0, fmt.Errorf("invalid method descriptor %q: parameter word count exceeds 255", descriptor)
		}
	}
	if i >= len(descriptor) || descriptor[i] != ')' {
		return 0, 0, fmt.Errorf("invalid method descriptor %q: missing ')'", descriptor)
	}
	i++
	if i < len(descriptor) && descriptor[i] == 'V' {
		i++
	} else {
		i, result, err = parseFieldType(descriptor, i, false)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid method descriptor %q: %w", descriptor, err)
		}
	}
	if i != len(descriptor) {
		return 0, 0, fmt.Errorf("invalid method descriptor %q: trailing data", descriptor)
	}
	return args, result, nil
}

func parseFieldType(descriptor string, at int, array bool) (end, words int, err error) {
	if at >= len(descriptor) {
		return at, 0, fmt.Errorf("missing type")
	}
	switch descriptor[at] {
	case 'B', 'C', 'F', 'I', 'S', 'Z':
		return at + 1, 1, nil
	case 'J', 'D':
		if array {
			return at + 1, 1, nil
		}
		return at + 1, 2, nil
	case 'L':
		semicolon := strings.IndexByte(descriptor[at+1:], ';')
		if semicolon < 0 {
			return at, 0, fmt.Errorf("unterminated object type")
		}
		semicolon += at + 1
		name := descriptor[at+1 : semicolon]
		if err := validateInternalName(name); err != nil {
			return at, 0, fmt.Errorf("invalid object type: %w", err)
		}
		return semicolon + 1, 1, nil
	case '[':
		i := at
		for i < len(descriptor) && descriptor[i] == '[' {
			i++
			if i-at > maxArrayDimensions {
				return at, 0, fmt.Errorf("array type has more than %d dimensions", maxArrayDimensions)
			}
		}
		if i >= len(descriptor) || descriptor[i] == 'V' {
			return at, 0, fmt.Errorf("invalid array component")
		}
		end, _, err := parseFieldType(descriptor, i, true)
		return end, 1, err
	default:
		return at, 0, fmt.Errorf("invalid type tag %q", descriptor[at])
	}
}

func validateFieldSymbol(name, descriptor string) error {
	if err := validateFieldName(name); err != nil {
		return err
	}
	_, err := fieldDescriptorWords(descriptor)
	return err
}

func validateMethodSymbol(name, descriptor string) error {
	if err := validateMethodName(name); err != nil {
		return err
	}
	_, result, err := methodDescriptorWords(descriptor)
	if err != nil {
		return err
	}
	if name == "<init>" && result != 0 {
		return fmt.Errorf("constructor descriptor %q does not return void", descriptor)
	}
	if name == "<clinit>" && descriptor != "()V" {
		return fmt.Errorf("class initializer descriptor must be ()V, got %q", descriptor)
	}
	return nil
}

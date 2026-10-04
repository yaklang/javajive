package javaclassparser

// maskDeclarationAnnotations lets method-range recognition see the declaration
// tokens instead of annotation argument parentheses. It preserves offsets and
// skips quoted content: @A(value="){") must not open or close a method body.
func maskDeclarationAnnotations(source string) string {
	masked := []byte(source)
	for i := 0; i < len(source); {
		if next, skip := skipJavaNonCode(source, i); skip {
			i = next
			continue
		}
		if source[i] != '@' {
			i++
			continue
		}
		start := i
		i++
		for i < len(source) && (isWordByteDump(source[i]) || source[i] == '.') {
			i++
		}
		if i == start+1 {
			continue
		}
		for i < len(source) && (source[i] == ' ' || source[i] == '\t') {
			i++
		}
		if i < len(source) && source[i] == '(' {
			depth := 1
			i++
			for i < len(source) && depth > 0 {
				if next, skip := skipJavaNonCode(source, i); skip {
					i = next
					continue
				}
				if source[i] == '(' {
					depth++
				} else if source[i] == ')' {
					depth--
				}
				i++
			}
			if depth != 0 {
				return source
			}
		}
		for j := start; j < i; j++ {
			if masked[j] != '\t' {
				masked[j] = ' '
			}
		}
	}
	return string(masked)
}

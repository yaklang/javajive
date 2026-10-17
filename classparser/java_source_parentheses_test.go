package javaclassparser

// javaMatchParen returns the index of the ')' matching the '(' at openIdx, comment/quote aware.
func javaMatchParen(src string, openIdx int) int {
	depth := 0
	st := scanNormal
	for i := openIdx; i < len(src); i++ {
		st = scanAdvance(src, &i, st, &depth)
		if st != scanNormal || i >= len(src) {
			continue
		}
		switch src[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

package frametransfer

func ParseDescriptor(desc string) (args []Type, ret Type, hasRet bool, err error) {
	if desc == "" {
		return nil, Type{}, false, nil
	}
	if desc[0] != '(' {
		return nil, Type{}, false, invalidf("bad descriptor %q", desc)
	}
	i := 1
	for i < len(desc) && desc[i] != ')' {
		t, n, e := parseField(desc[i:])
		if e != nil {
			return nil, Type{}, false, e
		}
		args = append(args, t)
		i += n
	}
	if i >= len(desc) || desc[i] != ')' {
		return nil, Type{}, false, invalidf("unterminated descriptor %q", desc)
	}
	i++
	if i >= len(desc) {
		return args, Type{}, false, nil
	}
	if desc[i] == 'V' {
		return args, Type{}, false, nil
	}
	t, _, e := parseField(desc[i:])
	if e != nil {
		return nil, Type{}, false, e
	}
	return args, t, true, nil
}

func parseField(s string) (Type, int, error) {
	if s == "" {
		return Type{}, 0, invalidf("empty field descriptor")
	}
	switch s[0] {
	case 'B', 'C', 'I', 'S', 'Z':
		return T(Int), 1, nil
	case 'F':
		return T(Float), 1, nil
	case 'J':
		return T(Long), 1, nil
	case 'D':
		return T(Double), 1, nil
	case 'L':
		end := 1
		for end < len(s) && s[end] != ';' {
			end++
		}
		if end >= len(s) {
			return Type{}, 0, invalidf("unterminated class descriptor")
		}
		return RefOf(s[1:end]), end + 1, nil
	case '[':
		n := 0
		for n < len(s) && s[n] == '[' {
			n++
		}
		if n >= len(s) {
			return Type{}, 0, invalidf("bad array descriptor")
		}
		if s[n] == 'L' {
			end := n
			for end < len(s) && s[end] != ';' {
				end++
			}
			if end >= len(s) {
				return Type{}, 0, invalidf("unterminated array class")
			}
			return RefOf(s[:end+1]), end + 1, nil
		}
		return RefOf(s[:n+1]), n + 1, nil
	default:
		return Type{}, 0, invalidf("unknown field type %c", s[0])
	}
}

func newArrayClass(atype byte) string {
	switch atype {
	case 4:
		return "[Z"
	case 5:
		return "[C"
	case 6:
		return "[F"
	case 7:
		return "[D"
	case 8:
		return "[B"
	case 9:
		return "[S"
	case 10:
		return "[I"
	case 11:
		return "[J"
	default:
		return "[Ljava/lang/Object;"
	}
}

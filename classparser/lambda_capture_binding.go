package javaclassparser

import (
	"regexp"
	"strings"
)

type lambdaCaptureDeclaration struct {
	name, typ string
	line      int
	scope     *lambdaCaptureScope
}

type lambdaCaptureScope struct {
	parent       *lambdaCaptureScope
	open, end    int
	isLambda     bool
	declarations map[string]*lambdaCaptureDeclaration
	captures     map[*lambdaCaptureDeclaration]*lambdaCaptureBinding
}

type lambdaCaptureRead struct{ line, start, end int }

type lambdaCaptureBinding struct {
	declaration *lambdaCaptureDeclaration
	lambda      *lambdaCaptureScope
	reads       []lambdaCaptureRead
}

var lambdaCaptureDeclarationRe = regexp.MustCompile(`^(\t+)([A-Za-z_$][\w$.<>\[\]?, ]*?)\s+((?:var|lv)\d+(?:_\d+)*)\s*(?:=[^;]*)?;(\s*)$`)

// lambdaCaptureCodeLines preserves source offsets while hiding literals and
// comments. They are neither lexical scopes nor variable reads. Block comments
// carry their state across lines; escaped quotes never end a literal early.
func lambdaCaptureCodeLines(lines []string) []string {
	code := make([]string, len(lines))
	block := false
	var quote byte
	escaped := false
	for line, raw := range lines {
		masked := []byte(raw)
		for i := 0; i < len(raw); i++ {
			c := raw[i]
			if block {
				masked[i] = ' '
				if c == '*' && i+1 < len(raw) && raw[i+1] == '/' {
					masked[i+1] = ' '
					i++
					block = false
				}
				continue
			}
			if quote != 0 {
				masked[i] = ' '
				if escaped {
					escaped = false
				} else if c == '\\' {
					escaped = true
				} else if c == quote {
					quote = 0
				}
				continue
			}
			if c == '"' || c == '\'' {
				quote = c
				masked[i] = ' '
			} else if c == '/' && i+1 < len(raw) && raw[i+1] == '/' {
				for j := i; j < len(raw); j++ {
					masked[j] = ' '
				}
				break
			} else if c == '/' && i+1 < len(raw) && raw[i+1] == '*' {
				masked[i], masked[i+1] = ' ', ' '
				block = true
				i++
			}
		}
		code[line] = string(masked)
	}
	return code
}

// lambdaCaptureBindings resolves reads to declarations in lexical order. A
// slot-derived spelling can be reused in sibling blocks, but it is not a
// declaration identity. Only an already encountered declaration in an ancestor
// scope can supply a capture's type and value.
func lambdaCaptureBindings(lines []string) []*lambdaCaptureBinding {
	code := lambdaCaptureCodeLines(lines)
	scope := &lambdaCaptureScope{declarations: map[string]*lambdaCaptureDeclaration{}}
	var bindings []*lambdaCaptureBinding
	for line, ln := range code {
		match := lambdaCaptureDeclarationRe.FindStringSubmatchIndex(ln)
		for i := 0; i < len(ln); {
			switch ln[i] {
			case '{':
				parent := scope
				scope = &lambdaCaptureScope{
					parent: parent, open: line,
					isLambda:     strings.HasSuffix(strings.TrimSpace(ln[:i]), "->"),
					declarations: map[string]*lambdaCaptureDeclaration{},
					captures:     map[*lambdaCaptureDeclaration]*lambdaCaptureBinding{},
				}
				i++
				continue
			case '}':
				if scope.parent != nil {
					scope.end = line + 1
					scope = scope.parent
				}
				i++
				continue
			}
			if !isWordByteDump(ln[i]) {
				i++
				continue
			}
			start := i
			for i < len(ln) && isWordByteDump(ln[i]) {
				i++
			}
			name := ln[start:i]
			if match != nil && start == match[6] {
				typ := strings.TrimSpace(ln[match[4]:match[5]])
				typ = strings.TrimPrefix(typ, "final ")
				if !prevTokenIsControlKeyword(typ) {
					scope.declarations[name] = &lambdaCaptureDeclaration{name: name, typ: typ, line: line, scope: scope}
					continue
				}
				// `return varN;` / `throw varN;` also match the declaration
				// shape. Their operand remains a read of the visible binding.
			}
			if !lambdaCaptureTokenIsRead(ln, start, i) {
				continue
			}
			var declaration *lambdaCaptureDeclaration
			for visible := scope; visible != nil; visible = visible.parent {
				if declaration = visible.declarations[name]; declaration != nil {
					break
				}
			}
			if declaration == nil {
				continue
			}
			// The first lambda that crosses this declaration's scope owns the
			// snapshot. Nested lambdas read that same snapshot, rather than
			// re-capturing a mutable original from inside the outer lambda.
			var lambda *lambdaCaptureScope
			for inner := scope; inner != declaration.scope; inner = inner.parent {
				if inner.isLambda {
					lambda = inner
				}
			}
			if lambda == nil || !lambdaLineIsStmtStart(code, lambda.open) {
				continue
			}
			binding := lambda.captures[declaration]
			if binding == nil {
				binding = &lambdaCaptureBinding{declaration: declaration, lambda: lambda}
				lambda.captures[declaration] = binding
				bindings = append(bindings, binding)
			}
			binding.reads = append(binding.reads, lambdaCaptureRead{line: line, start: start, end: i})
		}
	}
	return bindings
}

func lambdaCaptureTokenIsRead(line string, start, end int) bool {
	before := strings.TrimRight(line[:start], " \t")
	after := strings.TrimLeft(line[end:], " \t")
	if len(before) > 0 {
		c := before[len(before)-1]
		if c == '.' || ((isWordByteDump(c) || c == ']' || c == '>' || c == '?') && !prevTokenIsControlKeyword(before)) {
			return false
		}
		if strings.Contains(after, "->") && (c == '(' || c == ',') &&
			len(after) > 0 && (after[0] == ')' || after[0] == ',') {
			return false // lambda parameter, not an enclosing-local read
		}
	}
	return !strings.HasPrefix(after, "=") || strings.HasPrefix(after, "==")
}

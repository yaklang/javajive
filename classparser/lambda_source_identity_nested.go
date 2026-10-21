package javaclassparser

import (
	"sort"
	"strconv"
	"strings"
)

type nativeLambdaBlockScope struct {
	arrow, open, end int
}

// The source writer emits block lambdas. A quote/comment-aware brace interval
// distinguishes a direct child from that child's descendants; text equality
// cannot do so when two distinct factories print identical packets. The
// intervals only locate already certified emissions, never authorize a body.
func (c *ClassObjectDumper) nativeLambdaDirectBlockScopes(source string, charge func(int64) bool) ([]nativeLambdaBlockScope, bool) {
	if c == nil || charge == nil || !charge(int64(len(source))+1) {
		return nil, false
	}
	arrows, known := nativeLambdaSourceArrows(source, c.Work)
	if !known {
		return nil, false
	}
	var scopes []nativeLambdaBlockScope
	end := -1
	for _, arrow := range arrows {
		if arrow < end {
			continue
		}
		open := arrow + 2
		for open < len(source) {
			if source[open] == ' ' || source[open] == '\n' || source[open] == '\r' || source[open] == '\t' {
				open++
				continue
			}
			if open+1 < len(source) && source[open] == '/' && (source[open+1] == '/' || source[open+1] == '*') {
				next, skipped := skipJavaNonCode(source, open)
				if !skipped || next <= open {
					return nil, false
				}
				open = next
				continue
			}
			break
		}
		if open >= len(source) || source[open] != '{' || !charge(int64(len(source)-open)) {
			return nil, false
		}
		end = javaMatchBrace(source, open)
		if end < 0 {
			return nil, false
		}
		scopes = append(scopes, nativeLambdaBlockScope{arrow: arrow, open: open, end: end})
	}
	return scopes, true
}

// javac8 registers an inner implementation while translating its enclosing
// body, then registers that enclosing implementation. Thus registration is a
// postorder of actual lexical factories, not a left-to-right arrow counter.
// Close both graphs: each original receipt must occur once, at a direct child
// event of its actual factory method, and every final source arrow must occur.
func (c *ClassObjectDumper) native8NestedLambdaSourceIdentityEvents(source string, arrows []int, methods []*dumpedMethods, charge func(int64) bool) ([]nativeLambdaSourceIdentityEvent, bool) {
	return c.nestedLambdaSourceFactoryEvents(source, arrows, methods, charge, false)
}

func (c *ClassObjectDumper) nestedLambdaSourceFactoryEvents(source string, arrows []int, methods []*dumpedMethods, charge func(int64) bool, references bool) ([]nativeLambdaSourceIdentityEvent, bool) {
	type rootDeclaration struct {
		body *dumpedMethods
		text string
		at   int
	}
	var roots []rootDeclaration
	for _, method := range methods {
		if method == nil || method.lambdaEmission == nil || len(method.lambdaEmission.children) == 0 {
			continue
		}
		if method.member == nil || method.sourceRequest.role != methodSourceDeclaration || method.sourceDumper != c || method.sourceObject != c.obj || c.dumpedMethodsSet[method.sourceCacheKey] != method || method.sourceReconstructionFailed() || !c.methodParameterNameCacheClosed(method.parameterNameStamp) || !charge(int64(len(source))+int64(len(method.code))+1) {
			return nil, false
		}
		text, at, known := c.lambdaParentDeclaration(source, method)
		if !known {
			return nil, false
		}
		roots = append(roots, rootDeclaration{body: method, text: text, at: at})
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].at < roots[j].at })
	used := map[*MemberInfo]bool{}
	positions := map[int]bool{}
	events := make([]nativeLambdaSourceIdentityEvent, 0, len(arrows))
	var walk func(*dumpedMethods, *dumpedMethods, string, int, int) bool
	walk = func(parent, root *dumpedMethods, content string, offset, depth int) bool {
		if depth > methodSourceLambdaDepthLimit || parent == nil || !nativeProofWork(c.Work, 1) {
			return false
		}
		frame := parent.lambdaEmission
		if !frame.closed() || frame.body != parent || frame.request != parent.sourceRequest || len(frame.children) > methodSourceLambdaRequestLimit || !charge(int64(len(content))+int64(len(frame.children))+1) {
			return false
		}
		scopes, known := c.nativeLambdaDirectFactoryScopes(content, references, charge)
		if !known || len(scopes) != len(frame.children) {
			return false
		}
		for i, occurrence := range frame.children {
			child := occurrence.body
			if child == nil || used[child.member] {
				return false
			}
			receipt := c.lambdaMethodSourceBodies[child.member]
			if receipt == nil || receipt.object != c.obj || receipt.method != child.member || receipt.body != child || receipt.request != occurrence.request || child.sourceRequest != occurrence.request || receipt.key != child.sourceCacheKey || c.dumpedMethodsSet[receipt.key] != child || !c.lambdaMethodSourcePacketClosed(child.member, child, c.Work) {
				return false
			}
			factory := receipt.request.factory
			if !factory.closed() || factory.object != c.obj || factory.method != parent.member {
				return false
			}
			scope := scopes[i]
			position := offset + scope.arrow
			if positions[position] {
				return false
			}
			if scope.reference {
				local, known := nativeLambdaSourceImplementationTokens(occurrence.packet, c.Work, true)
				if !known || len(local) != 1 || occurrence.packet[local[0]:local[0]+2] != "::" || child.compilerReference == nil || !c.compilerReferenceFinalCaptureClosed(child, parent) {
					return false
				}
				start := scope.arrow - local[0]
				if start < 0 || len(occurrence.packet) > len(content)-start || !charge(int64(len(occurrence.packet))) || content[start:start+len(occurrence.packet)] != occurrence.packet {
					return false
				}
			} else {
				local, known := c.nativeLambdaDirectBlockScopes(occurrence.packet, charge)
				if !known || len(local) != 1 || child.compilerReference != nil {
					return false
				}
				header := occurrence.packet[:local[0].arrow+2]
				start := scope.arrow + 2 - len(header)
				if start < 0 || !charge(int64(len(header))) || content[start:scope.arrow+2] != header {
					return false
				}
				if strings.IndexByte(occurrence.packet, 0) >= 0 {
					if !c.composedLambdaOccurrenceClosed(receipt, parent, content, content[start:scope.end+1], charge) {
						return false
					}
				} else if !c.native8LambdaRenderedIdentityPacketClosed(receipt, source, charge, parent) {
					return false
				}
			}
			used[child.member], positions[position] = true, true
			if !scope.reference && !walk(child, root, content[scope.open+1:scope.end], offset+scope.open+1, depth+1) {
				return false
			}
			factoryName, named := c.originalLambdaFactoryNameComponent(factory, receipt.method)
			implementation, known := sourceBridgeUTF8(c.obj, receipt.method.NameIndex)
			prefix := "lambda$" + factoryName + "$"
			if !named || !known || len(implementation) <= len(prefix) || implementation[:len(prefix)] != prefix {
				return false
			}
			suffix := implementation[len(prefix):]
			ordinal, err := strconv.Atoi(suffix)
			if err != nil || ordinal < 0 || suffix != strconv.Itoa(ordinal) {
				return false
			}
			events = append(events, nativeLambdaSourceIdentityEvent{ordinal: ordinal, parent: root, body: child})
		}
		return true
	}
	for _, root := range roots {
		open := javaIndexTopBrace(root.text)
		end := -1
		if open >= 0 && charge(int64(len(root.text))) {
			end = javaMatchBrace(root.text, open)
		}
		if open < 0 || end < 0 || !walk(root.body, root.body, root.text[open+1:end], root.at+open+1, 0) {
			return nil, false
		}
	}
	if len(used) != len(c.lambdaMethodSourceBodies) || len(positions) != len(arrows) {
		return nil, false
	}
	for _, arrow := range arrows {
		if !positions[arrow] {
			return nil, false
		}
	}
	return events, true
}

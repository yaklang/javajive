package javaclassparser

import (
	"github.com/yaklang/javajive/internal/workbudget"
	"strconv"
	"strings"
)

type nativeAccessorSourceEvent struct {
	getter      *nativeMemberPrivateGetter
	constructor string
	start, end  int
}

// These are proof markers emitted by original call-site projections. Quoted
// strings and unrelated comments cannot manufacture a registration event.
func nativeMemberAccessorEvents(p *nativeMemberFamily, source string, constructors map[string]bool, work *workbudget.Budget) ([]nativeAccessorSourceEvent, bool) {
	if p == nil || p.failed || !nativeProofWork(work, int64(len(source))) ||
		work != nil && work.CheckAlloc(int64(len(source))*4+int64(len(p.getters))*128) != nil {
		return nil, false
	}
	expected := map[string]*nativeMemberPrivateGetter{}
	for _, getter := range p.getters {
		key := strconv.Itoa(getter.ordinal) + ":" + getter.owner + ":" + getter.field
		if getter.setter {
			key += ":put"
		} else if getter.update != nil {
			key += ":update:" + strconv.Itoa(getter.update.accessCode)
		}
		expected[key] = getter
	}
	const prefix = "jdec-owned-getter:"
	var events []nativeAccessorSourceEvent
	for i := 0; i < len(source); {
		ch := source[i]
		if ch == '\'' || ch == '"' {
			quote := ch
			i++
			closed := false
			for i < len(source) {
				if source[i] == '\\' {
					i += 2
					continue
				}
				if source[i] == quote {
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, false
			}
			continue
		}
		if i+1 < len(source) && source[i:i+2] == "//" {
			end := strings.IndexByte(source[i:], '\n')
			if end < 0 {
				break
			}
			i += end + 1
			continue
		}
		if i+1 < len(source) && source[i:i+2] == "/*" {
			end := strings.Index(source[i+2:], "*/")
			if end < 0 {
				return nil, false
			}
			comment := source[i+2 : i+2+end]
			if strings.HasPrefix(comment, prefix) {
				getter := expected[strings.TrimPrefix(comment, prefix)]
				if getter == nil {
					return nil, false
				}
				events = append(events, nativeAccessorSourceEvent{getter: getter, start: i, end: i + end + 4})
			} else if strings.HasPrefix(comment, nativeConstructorRegistrationPrefix) {
				key := strings.TrimPrefix(comment, nativeConstructorRegistrationPrefix)
				if !constructors[key] {
					return nil, false
				}
				events = append(events, nativeAccessorSourceEvent{constructor: key, start: i, end: i + end + 4})
			}
			i += end + 4
			continue
		}
		i++
	}
	return events, true
}

type nativeAccessorOrderState struct {
	getters      map[*nativeMemberPrivateGetter]bool
	fields       map[string]int
	constructors map[string]bool
}

func newNativeAccessorOrderState() *nativeAccessorOrderState {
	return &nativeAccessorOrderState{getters: map[*nativeMemberPrivateGetter]bool{}, fields: map[string]int{}, constructors: map[string]bool{}}
}
func (s *nativeAccessorOrderState) clone() *nativeAccessorOrderState {
	out := newNativeAccessorOrderState()
	for g, v := range s.getters {
		out.getters[g] = v
	}
	for k, v := range s.fields {
		out.fields[k] = v
	}
	for k, v := range s.constructors {
		out.constructors[k] = v
	}
	return out
}
func (s *nativeAccessorOrderState) apply(events []nativeAccessorSourceEvent) bool {
	for _, event := range events {
		if g := event.getter; g != nil {
			// javac clones inherited protected symbols for each source occurrence.
			if s.getters[g] && nativeMemberAccessorClonedSymbol(g) {
				return false
			}
			if !s.getters[g] {
				key := nativeMemberAccessorSymbolKey(g)
				prior, known := s.fields[key]
				if known && prior != g.ordinal || !known && g.ordinal != (len(s.fields)+len(s.constructors))*100 {
					return false
				}
				s.fields[key] = g.ordinal
				s.getters[g] = true
			}
		} else {
			if event.constructor == "" {
				return false
			}
			s.constructors[event.constructor] = true
		}
	}
	return true
}

// The typed invocation renderer calls this on its receiver subtree only. Keep
// every executable token and ordinary comment in place, and defer the proved
// compiler events until after the argument subtree. Nested calls have already
// arranged their own events in that same javac lowering order.
func nativeInvocationReceiverSource(p *nativeMemberFamily, source string, constructors map[string]bool, work *workbudget.Budget) (string, string, bool) {
	events, known := nativeMemberAccessorEvents(p, source, constructors, work)
	if !known {
		return source, "", false
	}
	if len(events) == 0 {
		return source, "", true
	}
	var receiver, registration strings.Builder
	receiver.Grow(len(source))
	cursor := 0
	for _, event := range events {
		receiver.WriteString(source[cursor:event.start])
		registration.WriteString(source[event.start:event.end])
		cursor = event.end
	}
	receiver.WriteString(source[cursor:])
	return receiver.String(), registration.String(), true
}

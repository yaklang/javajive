package javaclassparser

import (
	"fmt"
	"testing"
)

// The Java boxing/unboxing contract needs public denotable wrapper types,
// independently of whichever application first references a particular one.
func TestAdversarialPlatformBoxingCarriersHaveCompleteOriginalDeclarations(t *testing.T) {
	for _, release := range []int{8, 9, 11, 16, 17, 21} {
		t.Run(fmt.Sprint(release), func(t *testing.T) {
			for _, tc := range []struct{ name, primitive, accessor string }{{"Boolean", "Z", "booleanValue"}, {"Byte", "B", "byteValue"}, {"Short", "S", "shortValue"}, {"Character", "C", "charValue"}, {"Integer", "I", "intValue"}, {"Long", "J", "longValue"}, {"Float", "F", "floatValue"}, {"Double", "D", "doubleValue"}, {"Void", "V", ""}} {
				t.Run(tc.name, func(t *testing.T) {
					name := "java/lang/" + tc.name
					cl, known := jdkInvocationMetadata(name, release)
					if !known || cl.Name != name || !cl.Public || !cl.Final || cl.IsInterface || !cl.MembersComplete || !cl.ParentsComplete {
						t.Fatalf("missing original source declaration for%s/%d: %+v", name, release, cl)
					}
					for _, parent := range cl.Parents {
						if _, ok := jdkInvocationMetadata(parent, release); !ok {
							t.Fatalf("unknown parent %s", parent)
						}
					}
					if tc.accessor == "" {
						return
					}
					boxing, unboxing := 0, 0
					for _, m := range cl.Methods {
						if m.Name == "valueOf" && m.Desc == "("+tc.primitive+")L"+name+";" && m.Public && m.Static && !m.Varargs && m.ExceptionsKnown && len(m.Exceptions) == 0 {
							boxing++
						}
						if m.Name == tc.accessor && m.Desc == "()"+tc.primitive && m.Public && !m.Static && !m.Varargs && m.ExceptionsKnown && len(m.Exceptions) == 0 {
							unboxing++
						}
					}
					if boxing != 1 || unboxing != 1 {
						t.Fatalf("original language conversion ports:boxing=%d,unboxing=%d", boxing, unboxing)
					}
				})
			}
		})
	}
}

package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

func closedBodyMethodTableFixture(pairs int, prefix string) string {
	var methods strings.Builder
	for i := 0; i < pairs; i++ {
		// Same name, different complete JVM descriptors. None is executed by
		// construction; a larger declaration table cannot alter its behavior.
		fmt.Fprintf(&methods, "private int spare%d(int v){return v;}private long spare%d(long v){return v;}", i, i)
	}
	f := repeatedClosedBodyFixture(6, prefix)
	return strings.Replace(f, " Object captured(){", methods.String()+" Object captured(){", 1)
}

func TestAdversarialConstructorOwnMethodTableSizeDoesNotDuplicateBodyWork(t *testing.T) {
	for _, pairs := range []int{16, 32, 48} {
		t.Run(fmt.Sprint(pairs), func(t *testing.T) {
			for _, prefix := range []string{"Closed", "MethodTable"} {
				t.Run(prefix, func(t *testing.T) {
					testIndependentFlatClosedCalleeFamily(t, closedBodyMethodTableFixture(pairs, prefix), prefix, "320:closed:method:capture:effects\n", []string{prefix + "Owner", prefix + "Owner$Child"})
				})
			}
		})
	}
}

func TestNativeConstructorOwnMethodTableKeepsIndependentOverloads(t *testing.T) {
	for _, pairs := range []int{16, 32, 48} {
		t.Run(fmt.Sprint(pairs), func(t *testing.T) {
			for _, prefix := range []string{"Closed", "MethodTable"} {
				t.Run(prefix, func(t *testing.T) {
					testNativeIndependentFamilyFixture(t, closedBodyMethodTableFixture(pairs, prefix), []string{prefix + "Owner"}, prefix+"Driver", "320:closed:method:capture:effects\n", nativeLexicalExactSignatures)
				})
			}
		})
	}
}

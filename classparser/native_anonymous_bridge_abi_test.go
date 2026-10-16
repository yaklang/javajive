package javaclassparser

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// The general fixture shape intentionally omits synthetic methods. Covariant
// forwarders are part of this repair's observable API, so compare them too.
func nativeCovariantBridgeABI(t *testing.T, name string, original, rebuilt []byte) {
	t.Helper()
	shape := func(raw []byte) string {
		o, e := Parse(raw)
		if e != nil {
			t.Fatal(e)
		}
		rows := []string{}
		for _, m := range o.Methods {
			if m.AccessFlags&0x40 == 0 {
				continue
			}
			n, e := o.getUtf8(m.NameIndex)
			if e != nil {
				t.Fatal(e)
			}
			d, e := o.getUtf8(m.DescriptorIndex)
			if e != nil {
				t.Fatal(e)
			}
			exceptions := []string{}
			for _, a := range m.Attributes {
				if ex, ok := a.(*ExceptionsAttribute); ok {
					for _, i := range ex.ExceptionIndexTable {
						n, e := o.getUtf8(i)
						if e != nil {
							t.Fatal(e)
						}
						exceptions = append(exceptions, n)
					}
				}
			}
			rows = append(rows, fmt.Sprintf("%s%s:%x:%s", n, d, m.AccessFlags, strings.Join(exceptions, ",")))
		}
		sort.Strings(rows)
		return strings.Join(rows, "\n")
	}
	if a, b := shape(original), shape(rebuilt); a != b {
		t.Fatalf("covariant bridge ABI %s\noriginal %s\nrebuilt %s", name, a, b)
	}
}

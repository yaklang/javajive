package javaclassparser

import (
	"strconv"
	"testing"
)

func TestNativeSerializableSourcePacketsRequireActualOwnedJavaCode(t *testing.T) {
	packet := `static Object create(){return ((Function<String,String> & Serializable)(Target::Aa));}`
	for _, row := range []struct {
		name, source string
		want         int
	}{
		{"own method", "class Scope {" + packet + "}", 1},
		{"annotation array is not class body", "@Numbers({1,2}) class Scope {" + packet + "}", 1},
		{"quoted method", "class Scope {String text=" + strconv.Quote(packet) + ";}", 0},
		{"block comment", "class Scope {/*" + packet + "*/}", 0},
		{"line comment", "class Scope {//" + packet + "\n}", 0},
		{"nested member", "class Scope {static class Member {" + packet + "}}", 0},
		{"unrelated next class", "class Scope {} class Other {" + packet + "}", 0},
		{"duplicated own packet", "class Scope {" + packet + packet + "}", 2},
		{"nested quoted braces", "class Scope {String text=\"} {\";" + packet + "}", 1},
		{"text block is not source8 code", "class Scope {String text=\"\"\"\n" + packet + "\n\"\"\";}", 0},
	} {
		t.Run(row.name, func(t *testing.T) {
			if got := serializableJavaPacketCount(row.source, packet, true); got != row.want {
				t.Fatalf("owned packet count=%d want=%d", got, row.want)
			}
		})
	}
}

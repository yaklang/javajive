package javaclassparser

import "testing"

func TestT19AnnotatedDeclarationRanges(t *testing.T) {
	for _, line := range []string{
		"\t@Mark() V put(K key,V value) {",
		"\t@a.Mark(value=\"){\", other=@b.Inner(x={1,2})) <T> T run(T t) {",
		"\tpublic @Mark() Object run() {",
	} {
		if !isMethodOrInitBlockStart(line) {
			t.Errorf("annotated declaration missed: %s", line)
		}
	}
	for _, line := range []string{"\t@Mark() Object field;", "\t@Mark()", "\tif (flag) {"} {
		if isMethodOrInitBlockStart(line) {
			t.Errorf("not a declaration: %s", line)
		}
	}
}

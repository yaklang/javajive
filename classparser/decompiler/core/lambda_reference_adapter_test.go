package core

import "testing"

func TestUnboxingLambdaAdapterRequiresExactWrapperAndEntryOrder(t *testing.T) {
	for primitive, wrapper := range map[string]string{"Z": "Boolean", "B": "Byte", "C": "Character", "S": "Short", "I": "Integer", "J": "Long", "F": "Float", "D": "Double"} {
		t.Run(primitive, func(t *testing.T) {
			actual := "(Ljava/lang/" + wrapper + ";)V"
			if !UnboxingLambdaAdapterProven("("+primitive+")V", "(Ljava/lang/Object;)V", actual, 0, RefInvokeStatic) {
				t.Fatal("exact unboxing rejected")
			}
			if UnboxingLambdaAdapterProven("("+primitive+")V", "(Ljava/lang/Object;)V", "(Ljava/lang/Number;)V", 0, RefInvokeStatic) {
				t.Fatal("guessed Number unboxing")
			}
			if UnboxingLambdaAdapterProven("("+primitive+")V", "(Ljava/lang/Object;)V", actual, 1, RefInvokeStatic) {
				t.Fatal("lost capture prefix")
			}
			if UnboxingLambdaAdapterProven("("+primitive+")V", "(Ljava/lang/Object;)V", actual, 0, RefNewInvokeSpecial) {
				t.Fatal("constructor adaptation guessed")
			}
		})
	}
	if !UnboxingLambdaAdapterProven("([CII)Ljava/lang/Integer;", "(Ljava/lang/Object;Ljava/lang/Object;Ljava/lang/Object;)Ljava/lang/Object;", "([CLjava/lang/Integer;Ljava/lang/Integer;)Ljava/lang/Integer;", 1, RefInvokeSpecial) {
		t.Fatal("ordered reference check and two unboxings rejected")
	}
	if UnboxingLambdaAdapterProven("(J)V", "(Ljava/lang/Object;)V", "(Ljava/lang/Integer;)V", 0, RefInvokeStatic) {
		t.Fatal("numeric widening outside proof")
	}
}

func TestRawLambdaReferenceAdapterRequiresExactDescriptors(t *testing.T) {
	const obj = "Ljava/lang/Object;"
	const text = "Ljava/lang/String;"
	for _, tt := range []struct {
		name, impl, erased, actual string
		captures                   int
		kind                       uint8
		want                       bool
	}{
		{"single", "(" + text + ")V", "(" + obj + ")V", "(" + text + ")V", 0, RefInvokeStatic, true},
		{"unchanged wide SAM parameter", "(" + text + "J)V", "(" + obj + "J)V", "(" + text + "J)V", 0, RefInvokeStatic, true},
		{"capture", "(" + obj + text + ")V", "(" + obj + ")V", "(" + text + ")V", 1, RefInvokeStatic, true},
		{"receiver capture", "(" + text + ")V", "(" + obj + ")V", "(" + text + ")V", 1, RefInvokeVirtual, true},
		{"reference array", "([I)V", "(" + obj + ")V", "([I)V", 0, RefInvokeStatic, true},
		{"wide capture", "(J" + text + ")V", "(" + obj + ")V", "(" + text + ")V", 1, RefInvokeStatic, true},
		{"covariant return", "(" + text + ")" + text, "(" + obj + ")" + obj, "(" + text + ")" + text, 0, RefInvokeStatic, true},
		{"nothing specialized", "(" + obj + ")V", "(" + obj + ")V", "(" + obj + ")V", 0, RefInvokeStatic, false},
		{"wrong capture count", "(" + text + ")V", "(" + obj + ")V", "(" + text + ")V", 1, RefInvokeStatic, false},
		{"missing receiver", "(" + text + ")V", "(" + obj + ")V", "(" + text + ")V", 0, RefInvokeVirtual, false},
		{"unknown kind", "(" + text + ")V", "(" + obj + ")V", "(" + text + ")V", 0, 0, false},
		{"impl parameter mismatch", "(" + obj + ")V", "(" + obj + ")V", "(" + text + ")V", 0, RefInvokeStatic, false},
		{"return adaptation", "(" + text + ")" + obj, "(" + obj + ")" + obj, "(" + text + ")" + text, 0, RefInvokeStatic, false},
		{"primitive argument", "(I)V", "(" + obj + ")V", "(I)V", 0, RefInvokeStatic, false},
		{"boxing", "(I)V", "(" + obj + ")V", "(Ljava/lang/Integer;)V", 0, RefInvokeStatic, false},
		{"strong bound", "(Ljava/lang/Integer;)V", "(Ljava/lang/Number;)V", "(Ljava/lang/Integer;)V", 0, RefInvokeStatic, false},
		{"invalid unrelated erasure", "(" + text + ")V", "(Ljava/lang/Number;)V", "(" + text + ")V", 0, RefInvokeStatic, false},
		{"invalid return erasure", "(" + text + ")" + text, "(" + obj + ")Ljava/lang/Number;", "(" + text + ")" + text, 0, RefInvokeStatic, false},
		{"wrong arity", "(" + text + ")V", "(" + obj + obj + ")V", "(" + text + ")V", 0, RefInvokeStatic, false},
		{"trailing junk", "(" + text + ")Vjunk", "(" + obj + ")V", "(" + text + ")V", 0, RefInvokeStatic, false},
		{"void parameter", "(V)V", "(" + obj + ")V", "(V)V", 0, RefInvokeStatic, false},
		{"zero arguments", "()" + text, "()" + obj, "()" + text, 0, RefInvokeStatic, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReferenceLambdaAdapterProven(tt.impl, tt.erased, tt.actual, tt.captures, tt.kind); got != tt.want {
				t.Fatalf("adaptation accepted=%v want=%v", got, tt.want)
			}
		})
	}
}

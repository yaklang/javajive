package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"reflect"
	"testing"
)

func TestAdversarialConstructorEffectStackPermutationsPreserveReceiverAndAllocationIdentity(t *testing.T) {
	a := constructorEffectValue{kind: 'L', receiver: true}
	b := constructorEffectValue{kind: 'L', allocation: 17}
	c := constructorEffectValue{kind: 'I'}
	d := constructorEffectValue{kind: 'J'}
	for _, tc := range []struct {
		opcode   int
		in, want []constructorEffectValue
	}{
		{core.OP_DUP, []constructorEffectValue{a}, []constructorEffectValue{a, a}},
		{core.OP_DUP_X1, []constructorEffectValue{a, b}, []constructorEffectValue{b, a, b}},
		{core.OP_DUP_X2, []constructorEffectValue{a, c, b}, []constructorEffectValue{b, a, c, b}},
		{core.OP_DUP_X2, []constructorEffectValue{d, b}, []constructorEffectValue{b, d, b}},
		{core.OP_DUP2, []constructorEffectValue{a, b}, []constructorEffectValue{a, b, a, b}},
		{core.OP_DUP2, []constructorEffectValue{d}, []constructorEffectValue{d, d}},
		{core.OP_DUP2_X1, []constructorEffectValue{a, c, b}, []constructorEffectValue{c, b, a, c, b}},
		{core.OP_DUP2_X1, []constructorEffectValue{a, d}, []constructorEffectValue{d, a, d}},
		{core.OP_DUP2_X2, []constructorEffectValue{a, c, b, c}, []constructorEffectValue{b, c, a, c, b, c}},
		{core.OP_DUP2_X2, []constructorEffectValue{a, b, d}, []constructorEffectValue{d, a, b, d}},
		{core.OP_DUP2_X2, []constructorEffectValue{d, a, b}, []constructorEffectValue{a, b, d, a, b}},
		{core.OP_DUP2_X2, []constructorEffectValue{d, d}, []constructorEffectValue{d, d, d}},
		{core.OP_SWAP, []constructorEffectValue{a, b}, []constructorEffectValue{b, a}},
	} {
		before := append([]constructorEffectValue(nil), tc.in...)
		got, ok := constructorEffectStackPermutation(tc.in, tc.opcode)
		if !ok || !reflect.DeepEqual(got, tc.want) || !reflect.DeepEqual(before, tc.in) {
			t.Fatalf("opcode %d got %+v/%v expected %+v", tc.opcode, got, ok, tc.want)
		}
	}
	for _, tc := range []struct {
		op int
		in []constructorEffectValue
	}{
		{core.OP_DUP, nil}, {core.OP_DUP, []constructorEffectValue{d}},
		{core.OP_DUP_X1, []constructorEffectValue{d, b}}, {core.OP_DUP_X1, []constructorEffectValue{a, d}},
		{core.OP_DUP_X2, []constructorEffectValue{a, b}}, {core.OP_DUP2, []constructorEffectValue{b}},
		{core.OP_DUP2_X1, []constructorEffectValue{d, b}}, {core.OP_DUP2_X2, []constructorEffectValue{a, d}},
		{core.OP_SWAP, []constructorEffectValue{d, b}}, {core.OP_SWAP, []constructorEffectValue{a}},
		{core.OP_DUP, []constructorEffectValue{{}}},
	} {
		if _, ok := constructorEffectStackPermutation(tc.in, tc.op); ok {
			t.Fatalf("accepted split/missing packet %+v", tc)
		}
	}
}

package frametransfer

import (
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

// This models the limited initialization/exception rule, not hierarchy access
// or the entire verifier. Each original allocation token has its own PC. The
// oracle substitutes exact symbolic tokens, independently of initialize and
// sameUninitialized; an exceptional edge uses the pre-instruction locals.
func TestBoundedConstructorAliasAndExceptionModel(t *testing.T) {
	arguments := []struct {
		desc   string
		values []Type
	}{
		{"()V", nil}, {"(I)V", []Type{IntConst(-2147483648)}},
		{"(F)V", []Type{FloatBits(0x80000000)}},
		{"(J)V", []Type{LongConst(-9223372036854775808)}},
		{"(D)V", []Type{DoubleBits(0x7ff8000000000042)}},
		{"(Ljava/lang/Object;)V", []Type{T(Null)}},
		{"(IJ)V", []Type{IntConst(2147483647), LongConst(42)}},
		{"(JLjava/lang/Object;)V", []Type{LongConst(42), RefOf("model/Value")}},
		{"(JD)V", []Type{LongConst(-1), DoubleBits(0x8000000000000000)}},
	}
	targets := []struct {
		token Type
		owner string
	}{
		{T(UninitThis), "model/Owner"}, {T(UninitThis), "model/Base"},
		{Type{Kind: UninitNew, NewPC: 0, Class: "model/Allocation"}, "model/Allocation"},
		{Type{Kind: UninitNew, NewPC: 65535, Class: "model/Allocation"}, "model/Allocation"},
	}
	digest := sha256.New()
	var encoded [16]byte
	count := 0
	for targetIndex, target := range targets {
		alphabet := []Type{target.token,
			{Kind: UninitNew, NewPC: 42, Class: "model/Allocation"},
			{Kind: UninitNew, NewPC: 555, Class: "model/Other"},
			RefOf("model/Owner"), T(Null), LongConst(42),
			DoubleBits(0x8000000000000000), IntConst(-1)}
		sequences := [][]Type{nil}
		for _, a := range alphabet {
			sequences = append(sequences, []Type{a})
			for _, b := range alphabet {
				sequences = append(sequences, []Type{a, b})
			}
		}
		if len(sequences) != 73 {
			t.Fatal("incomplete logical alias grammar")
		}
		initialized := RefOf(target.token.Class)
		if target.token.Kind == UninitThis {
			initialized = RefOf("model/Owner")
		}
		for localIndex, locals := range sequences {
			for stackIndex, stack := range sequences {
				for argIndex, args := range arguments {
					localSlots := append(boundedStackSlots(locals), T(Top), T(Top))
					f, err := NewFrameWithLimits(len(localSlots), 16)
					if err != nil {
						t.Fatal(err)
					}
					f.Locals = localSlots
					f.Stack = boundedStackSlots(append(append(append([]Type(nil), stack...), target.token), args.values...))
					f.ThisUninitialized, f.ThisClass, f.DirectSuperClass = true, "model/Owner", "model/Base"
					before := f.Clone()
					want := before.Clone()
					want.Stack = boundedStackSlots(stack)
					for i, value := range want.Locals {
						if value == target.token {
							want.Locals[i] = initialized
						}
					}
					for i, value := range want.Stack {
						if value == target.token {
							want.Stack[i] = initialized
						}
					}
					if target.token.Kind == UninitThis {
						want.ThisUninitialized = false
					}
					wantException := before.Clone()
					wantException.Stack = []Type{RefOf("java/lang/Throwable")}
					for i, value := range wantException.Locals {
						if value == target.token {
							wantException.Locals[i] = T(Top)
						}
					}
					out, exception, err := Transfer(f, Instr{Op: core.OP_INVOKESPECIAL, Name: "<init>", Class: target.owner, Desc: args.desc})
					if err != nil || !boundedFrameExact(out, want) || exception == nil || !boundedFrameExact(*exception, wantException) || !boundedFrameExact(f, before) {
						t.Fatalf("constructor model target=%d locals=%d stack=%d args=%d: out=%+v want=%+v ex=%+v wantEx=%+v err=%v", targetIndex, localIndex, stackIndex, argIndex, out, want, exception, wantException, err)
					}
					for i, v := range []int{targetIndex, localIndex, stackIndex, argIndex} {
						binary.LittleEndian.PutUint32(encoded[i*4:], uint32(v))
					}
					digest.Write(encoded[:])
					count++
				}
			}
		}
	}
	if count != 191844 {
		t.Fatalf("incomplete constructor product: %d", count)
	}
	t.Logf("bounded constructor normal/exception aliases: %d cases, sha256=%x", count, digest.Sum(nil))
}

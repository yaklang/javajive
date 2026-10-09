package javaclassparser

import (
	"context"
	"regexp"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialConstructorReadOnlyVoidMatchesIndependentFiniteLanguage(t *testing.T) {
	// Enumerate a finite grammar independently as ASCII words. The regexp
	// oracle has no class parser, opcode decoder or production recognizer.
	language := regexp.MustCompile(`^n*rn*$`)
	opcodes := []byte{byte(core.OP_NOP), byte(core.OP_RETURN), byte(core.OP_ALOAD_0)}
	labels := []byte{'n', 'r', 'x'}
	cases := 0
	for width := 0; width <= 7; width++ {
		count := 1
		for i := 0; i < width; i++ {
			count *= len(opcodes)
		}
		for word := 0; word < count; word++ {
			code, text := make([]byte, width), make([]byte, width)
			value := word
			for i := range code {
				code[i], text[i] = opcodes[value%3], labels[value%3]
				value /= 3
			}
			remaining := 512
			if accepted := constructorReadOnlyVoidBody(code, &remaining, nil); accepted != language.Match(text) {
				t.Fatalf("readonly void language differs for %q: accepted=%v", text, accepted)
			}
			cases++
		}
	}
	if cases != 3280 {
		t.Fatal("incomplete finite grammar", cases)
	}
	// Every other raw opcode is forbidden even if a valid return follows it.
	for opcode := 0; opcode < 256; opcode++ {
		remaining := 512
		got := constructorReadOnlyVoidBody([]byte{byte(opcode), byte(core.OP_RETURN)}, &remaining, nil)
		if got != (opcode == core.OP_NOP) {
			t.Fatalf("forbidden prefix %#x acquired readonly certificate", opcode)
		}
	}
}

func TestAdversarialConstructorReadOnlyVoidKeepsNopAndResourceBoundaries(t *testing.T) {
	for position := 0; position < 16; position++ {
		code := make([]byte, 16)
		code[position] = byte(core.OP_RETURN)
		remaining := 1
		work := workbudget.New(nil, workbudget.Limits{})
		if !constructorReadOnlyVoidBody(code, &remaining, work) || remaining != 0 || work.Used(workbudget.CounterGraphScans) != 16 {
			t.Fatal("NOP transparency or per-byte native work changed", position, remaining)
		}
	}
	for _, variant := range []string{"byte bound", "instruction cap", "cancel", "work"} {
		t.Run(variant, func(t *testing.T) {
			code, remaining := []byte{byte(core.OP_RETURN)}, 1
			work := workbudget.New(nil, workbudget.Limits{})
			switch variant {
			case "byte bound":
				code = append(code, make([]byte, 16)...)
			case "instruction cap":
				remaining = 0
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				code = []byte{byte(core.OP_NOP), byte(core.OP_RETURN)}
			}
			if constructorReadOnlyVoidBody(code, &remaining, work) {
				t.Fatal("void grammar bypassed a resource refusal")
			}
		})
	}
}

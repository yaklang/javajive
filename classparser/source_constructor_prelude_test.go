package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func TestAdversarialSourceConstructorPreludeRejectsLiveOrProtectedOriginalEffects(t *testing.T) {
	files := constructorNoargPreludeOriginal(t, "none", "reference", true)
	for _, variant := range []string{"control", "result read", "parameter overwritten", "wide category", "effectful argument", "protected prefix", "call removed"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(files["UnusedPrefixOwner.class"]))
			method, known := sourceAnonymousConstructorMethod(obj, "(Ljava/lang/Object;I)V", nil)
			if !known {
				t.Fatal("original constructor")
			}
			code := constructorLoweringCode(method)
			// Independently authored prefix: NOP, ILOAD_2, INVOKESTATIC,
			// ASTORE_3, then ALOAD_0/INVOKESPECIAL and the unchanged body.
			if code.Code[5] != core.OP_ASTORE_3 || code.Code[6] != core.OP_ALOAD_0 {
				t.Fatal("original prefix encoding changed")
			}
			switch variant {
			case "result read":
				code.Code = append(append(bytes.Clone(code.Code[:10]), core.OP_ALOAD_3, core.OP_POP), code.Code[10:]...)
				code.AttrLen += 2
			case "parameter overwritten":
				code.Code[5] = core.OP_ASTORE_1
			case "wide category":
				code.Code[5] = core.OP_LSTORE_3
			case "effectful argument":
				index := obj.ConstantPoolManager.AppendConstantInfo(&ConstantFieldrefInfo{ConstantMemberrefInfo: *obj.ConstantPoolManager.newMemberrefInfo("UnusedPrefixEffects", "mode", "I")})
				code.Code = append([]byte{0, core.OP_GETSTATIC, byte(index >> 8), byte(index)}, code.Code[2:]...)
				code.AttrLen += 2
			case "protected prefix":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 6, HandlerPc: 10}}
				code.AttrLen += 8
			case "call removed":
				code.Code[2] = core.OP_NOP
			}
			if _, err := Parse(obj.Bytes()); err != nil {
				t.Fatal("refusal must reach typed instruction proof", err)
			}
			packet := constructorLoweringTypedPacketFor(obj, method, true, true, nil, nil)
			if (packet != nil) != (variant == "control") {
				t.Fatal("unproved original effect packet accepted", packet != nil)
			}
			if packet != nil && (len(packet.plan.prelude) != 1 || len(packet.plan.captures) != 0 || packet.plan.superDescriptor != "()V") {
				t.Fatal("wrong effect/capture domain")
			}
		})
	}
}

func TestAdversarialSourceConstructorPreludeCompletionRejectsChangedInvocations(t *testing.T) {
	files := constructorNoargPreludeOriginal(t, "none", "reference", true)
	original, fresh, artifact := constructorNoargPreludeRoundTrip(t, files, "none", ModernJavac)
	base := artifact.source.jar.sourceConstructorLowering
	publication := base.active["UnusedPrefixOwner"]
	if publication == nil {
		t.Fatal("actual returned-source authority")
	}
	for _, variant := range []string{"control", "unreturned source", "pending only", "changed callee", "changed argument", "result becomes live", "already finished"} {
		t.Run(variant, func(t *testing.T) {
			a := &sourceConstructorLoweringAuthority{jar: base.jar, active: map[string]*sourceConstructorLoweringPublication{"UnusedPrefixOwner": publication}}
			obj := mustParseLowering(t, bytes.Clone(fresh))
			method, known := sourceAnonymousConstructorMethod(obj, "(Ljava/lang/Object;I)V", nil)
			if !known {
				t.Fatal("fresh constructor")
			}
			packet := constructorLoweringReadPacket(obj, method, false, false, false, nil, nil, 0, 1)
			if packet == nil || len(packet.preludeOrigins) != 1 {
				t.Fatal("fresh typed prelude")
			}
			code := constructorLoweringCode(method)
			call := packet.preludeOrigins[0].pc
			switch variant {
			case "unreturned source":
				a.active = nil
			case "pending only":
				a.pending, a.active = a.active, nil
			case "changed callee":
				index := obj.ConstantPoolManager.AddNewMethodInfo("UnusedPrefixEffects", "differentProducer", "(I)[Ljava/lang/Object;")
				code.Code[call+1], code.Code[call+2] = byte(index>>8), byte(index)
			case "changed argument":
				code.Code[call-1] = core.OP_ICONST_0
			case "result becomes live":
				// Retaining the result changes the allowed source transformation.
				// Convert any fresh discard to a valid unused slot, then read it.
				at := call + 3
				if code.Code[at] != core.OP_POP && code.Code[at] != core.OP_ASTORE_3 {
					t.Fatal("expected fresh result discard", code.Code[at])
				}
				code.Code[at] = core.OP_ASTORE_3
				if code.MaxLocals < 4 {
					code.MaxLocals = 4
				}
				end := packet.secondEnd
				code.Code = append(append(bytes.Clone(code.Code[:end]), core.OP_ALOAD_3, core.OP_POP), code.Code[end:]...)
				code.AttrLen += 2
			}
			input := obj.Bytes()
			if _, err := Parse(input); err != nil {
				t.Fatal("negative packet must parse", err)
			}
			if variant == "already finished" {
				var err error
				input, err = a.lower(original, input, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			before := sha256.Sum256(input)
			result, err := a.lower(original, input, nil)
			if variant == "control" || variant == "already finished" {
				if err != nil || len(result) == 0 {
					t.Fatal("owned completion", err)
				}
				if variant == "already finished" && !bytes.Equal(result, input) {
					t.Fatal("non-idempotent completion")
				}
			} else if err == nil || result != nil {
				t.Fatal("changed effect packet borrowed authority", err)
			}
			if sha256.Sum256(input) != before {
				t.Fatal("fresh input mutated")
			}
		})
	}
}

func TestAdversarialSourceConstructorPreludeCompletionKeepsInvocationMultiplicityAndOrder(t *testing.T) {
	files := constructorNoargPreludeOriginal(t, "none", "ordered pair", true)
	original, fresh, artifact := constructorNoargPreludeRoundTrip(t, files, "none", ModernJavac)
	for _, variant := range []string{"control", "reversed", "duplicated"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(fresh))
			method, known := sourceAnonymousConstructorMethod(obj, "(Ljava/lang/Object;I)V", nil)
			if !known {
				t.Fatal("fresh constructor")
			}
			packet := constructorLoweringReadPacket(obj, method, false, false, false, nil, nil, 0, 2)
			if packet == nil || len(packet.preludeOrigins) != 2 {
				t.Fatal("two fresh ordered calls")
			}
			code := constructorLoweringCode(method)
			a, b := packet.preludeOrigins[0].pc, packet.preludeOrigins[1].pc
			first := bytes.Clone(code.Code[a : a+3])
			if variant == "reversed" {
				copy(code.Code[a:a+3], code.Code[b:b+3])
				copy(code.Code[b:b+3], first)
			}
			if variant == "duplicated" {
				copy(code.Code[b:b+3], first)
			}
			input := obj.Bytes()
			before := sha256.Sum256(input)
			result, err := artifact.source.jar.sourceConstructorLowering.lower(original, input, nil)
			if variant == "control" {
				if err != nil || len(result) == 0 {
					t.Fatal("ordered calls refused", err)
				}
			} else if err == nil || result != nil {
				t.Fatal("changed invocation order/multiplicity accepted", err)
			}
			if sha256.Sum256(input) != before {
				t.Fatal("fresh input mutated")
			}
		})
	}
}

package javaclassparser

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

// Original and fresh bytecode are compiled independently. The fresh source
// spells the forwarding declaration but cannot spell ACC_SYNTHETIC. Restoring
// that bit may not authorize a different receiver, argument packet or target.
func TestAdversarialConstructorBridgeFlagsRequireOriginalAndFreshTypedPacket(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	const originalSource = `class FlagOwner {static class Child {private Child(Object p,long q)throws java.io.IOException{}}static Child make(Object p,long q)throws java.io.IOException{return new Child(p,q);}}`
	const freshSource = `class FlagOwner {static class Child {private Child(Object p,long q)throws java.io.IOException{} Child(Object p,long q,FlagOwner$1 marker)throws java.io.IOException{this(p,q);}}static Child make(Object p,long q)throws java.io.IOException{return new Child(p,q);}}class FlagOwner$1 {}`
	originals := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, originalSource, "FlagOwner", "none", false)
	for _, release := range []int{8, 11, 16, 21} {
		fresh := compileLambdaParameterMetadataFixtureWithSourcesAtRelease(t, modern, ModernJavac, freshSource, "FlagOwner", "none", nil, release, false)["FlagOwner$Child.class"]
		for _, variant := range []string{"control", "fresh receiver", "fresh argument", "fresh marker used", "fresh target visibility", "fresh bridge visibility", "fresh extra effect", "fresh throws", "fresh extra method", "fresh missing method", "fresh preview", "fresh future", "original receiver", "original modern", "original marker spelling", "both marker spelling", "work", "output", "canceled"} {
			t.Run(fmt.Sprintf("release%d/%s", release, variant), func(t *testing.T) {
				o, c := mustParseLowering(t, bytes.Clone(originals["FlagOwner$Child.class"])), mustParseLowering(t, bytes.Clone(fresh))
				bridge := func(obj *ClassObject) *MemberInfo {
					for _, m := range obj.Methods {
						d, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
						if d == "(Ljava/lang/Object;JLFlagOwner$1;)V" {
							return m
						}
					}
					t.Fatal("authored bridge")
					return nil
				}
				ob, cb := bridge(o), bridge(c)
				code := constructorLoweringCode(cb)
				var work *workbudget.Budget
				switch variant {
				case "fresh receiver":
					code.Code[0] = 0x2b // aload_1 cannot replace uninitialized-this.
				case "fresh argument":
					code.Code[1] = 0x2a // this is not the original reference argument.
				case "fresh marker used":
					code.Code = append(append([]byte{}, code.Code[:1]...), append([]byte{0x19, 4}, code.Code[2:]...)...)
				case "fresh target visibility":
					for _, m := range c.Methods {
						if m != cb {
							m.AccessFlags = 0
						}
					}
				case "fresh bridge visibility":
					cb.AccessFlags = 1
				case "fresh extra effect":
					code.Code = append([]byte{0x01, 0x57}, code.Code...)
				case "fresh throws":
					for _, a := range cb.Attributes {
						if e, ok := a.(*ExceptionsAttribute); ok {
							e.ExceptionIndexTable = []uint16{c.ThisClass}
						}
					}
				case "fresh extra method":
					copy := *cb
					copy.NameIndex = uint16(c.ConstantPoolManager.AddUtf8Info("extra"))
					c.Methods = append(c.Methods, &copy)
				case "fresh missing method":
					c.Methods = []*MemberInfo{cb}
				case "fresh preview":
					c.MinorVersion = 65535
				case "fresh future":
					c.MajorVersion = 66
				case "original receiver":
					constructorLoweringCode(ob).Code[0] = 0x2b
				case "original modern":
					o.MajorVersion = 55
				case "original marker spelling", "both marker spelling":
					ob.DescriptorIndex = uint16(o.ConstantPoolManager.AddUtf8Info("(Ljava/lang/Object;JLIndependentUnusedPort;)V"))
					if variant == "both marker spelling" {
						cb.DescriptorIndex = uint16(c.ConstantPoolManager.AddUtf8Info("(Ljava/lang/Object;JLIndependentUnusedPort;)V"))
					}
				case "work":
					work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
				case "output":
					work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				authority, input := o.Bytes(), c.Bytes()
				// Even rejected candidates must be structurally parseable; otherwise a
				// parser failure would hide whether the forwarding proof rejected them.
				mustParseLowering(t, input)
				before, old := bytes.Clone(input), bytes.Clone(authority)
				result, err := restoreReconstructionSyntheticFlags(authority, input, work)
				positive := variant == "control" || variant == "both marker spelling"
				if positive {
					if err != nil || bytes.Equal(result, input) || sourceMethodMetadataClosed(o, result, nil) != nil {
						t.Fatal("typed synthetic declaration not restored", err)
					}
					parsed := mustParseLowering(t, result)
					for _, m := range parsed.Methods {
						d, _ := sourceBridgeUTF8(parsed, m.DescriptorIndex)
						want, _ := sourceBridgeUTF8(c, cb.DescriptorIndex)
						if d == want && !bytes.Equal(constructorLoweringCode(m).Code, code.Code) {
							t.Fatal("fresh Code changed")
						}
					}
					twice, e := restoreReconstructionSyntheticFlags(authority, result, nil)
					if e != nil || !bytes.Equal(twice, result) {
						t.Fatal("non-idempotent completion", e)
					}
				} else if variant == "work" || variant == "output" || variant == "canceled" {
					if err == nil || result != nil {
						t.Fatal("partial resource-failure output", err)
					}
				} else if err != nil || !bytes.Equal(result, input) {
					t.Fatal("unproved forwarding declaration modified", err)
				}
				if !bytes.Equal(input, before) || !bytes.Equal(authority, old) {
					t.Fatal("input mutated")
				}
			})
		}
	}
}

package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
)

// This fixture adds only authored GOTO/GOTO_W edges around the native
// compiler's unchanged initializer. It supplies its own verifier/debug PCs;
// neither the production control-path proof nor its editor builds the input.
func enumEntryPathOriginal(t *testing.T, raw []byte, wide bool, hops int) []byte {
	t.Helper()
	obj := mustParseLowering(t, bytes.Clone(raw))
	for _, m := range obj.Methods {
		name, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if name != "<clinit>" {
			continue
		}
		code := constructorLoweringCode(m)
		if code == nil || len(code.ExceptionTable) != 0 || len(code.Code) == 0 || code.Code[len(code.Code)-1] != 0xb1 || hops < 1 {
			t.Fatal("authored straight initializer")
		}
		width := 3
		if wide {
			width = 5
		}
		old := bytes.Clone(code.Code)
		tail := width + len(old)
		result := make([]byte, tail+hops*width)
		branch := func(at, target int) {
			if wide {
				result[at] = 0xc8
				binary.BigEndian.PutUint32(result[at+1:], uint32(int32(target-at)))
			} else {
				if target-at < -32768 || target-at > 32767 {
					t.Fatal("fixture branch distance")
				}
				result[at] = 0xa7
				binary.BigEndian.PutUint16(result[at+1:], uint16(int16(target-at)))
			}
		}
		branch(0, tail)
		copy(result[width:], old)
		for i := 0; i < hops; i++ {
			target := tail + (i+1)*width
			if i == hops-1 {
				target = width
			}
			branch(tail+i*width, target)
		}
		code.Code = result
		for _, attribute := range code.Attributes {
			switch a := attribute.(type) {
			case *LineNumberTableAttribute:
				for _, row := range a.LineNumberTable {
					row.StartPc += uint16(width)
				}
			default:
				t.Fatalf("unexpected original initializer attribute %T", attribute)
			}
		}
		if obj.MajorVersion >= 50 {
			frames := []byte{0, byte(hops + 1)}
			previous := -1
			for _, pc := range append([]int{width}, func() []int {
				v := []int{}
				for i := 0; i < hops; i++ {
					v = append(v, tail+i*width)
				}
				return v
			}()...) {
				delta := pc - previous - 1
				if delta < 64 {
					frames = append(frames, byte(delta))
				} else {
					frames = append(frames, 251, byte(delta>>8), byte(delta))
				}
				previous = pc
			}
			obj.ConstantPoolManager.AddUtf8Info("StackMapTable")
			code.Attributes = append(code.Attributes, &UnparsedAttribute{Name: "StackMapTable", Length: uint32(len(frames)), Info: frames})
		}
		code.AttrLen = uint32(12 + len(code.Code))
		for _, a := range code.Attributes {
			switch x := a.(type) {
			case *LineNumberTableAttribute:
				code.AttrLen += 6 + x.AttrLen
			case *UnparsedAttribute:
				code.AttrLen += 6 + x.Length
			default:
				t.Fatal("authored table")
			}
		}
		return obj.Bytes()
	}
	t.Fatal("missing original initializer")
	return nil
}

func TestAdversarialEnumEntryControlPathKeepsOriginalArrayProtocol(t *testing.T) {
	for _, owner := range []string{"PathChoice", "RenamedPathChoice"} {
		for _, release := range []int{5, 8} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				for _, wide := range []bool{false, true} {
					for _, hops := range []int{1, 3} {
						t.Run(fmt.Sprintf("%s/release%d/%s/wide%v/hops%d", owner, release, debug, wide, hops), func(t *testing.T) {
							enumFreshHelperRoundTrip(t, owner, debug, release, func(files map[string][]byte) {
								files[owner+".class"] = enumEntryPathOriginal(t, files[owner+".class"], wide, hops)
							})
						})
					}
				}
			}
		}
	}
}

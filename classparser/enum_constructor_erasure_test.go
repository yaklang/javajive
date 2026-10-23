package javaclassparser

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialEnumConstructorErasureRequiresForwardingNotMarkerSpelling(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Fatal("native original compiler required")
	}
	files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, `public enum EnumPort {
 LEFT(1L,-0.0){public long apply(long x){return x+step;}},RIGHT(2L,0.5){public long apply(long x){return x-step;}};
 final long step;final double bias;EnumPort(long step,double bias){this.step=step;this.bias=bias;}public abstract long apply(long x);
}`, "EnumPort", "source,lines,vars", false)
	for _, variant := range []string{"original", "opaque marker spelling", "ordinary constructor", "wrong receiver", "wrong argument", "effectful prefix", "missing delegation", "nonprivate target", "duplicate bridge", "different throws", "kill switch", "modern nest version", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			object := mustParseLowering(t, bytes.Clone(files["EnumPort.class"]))
			var bridge *MemberInfo
			for _, m := range object.Methods {
				name, _ := sourceBridgeUTF8(object, m.NameIndex)
				if name == "<init>" && m.AccessFlags == 0x1000 {
					if bridge != nil {
						t.Fatal("unique original generated constructor")
					}
					bridge = m
				}
			}
			if bridge == nil {
				t.Fatal("actual javac8 constructor bridge")
			}
			code := constructorLoweringCode(bridge)
			var work *workbudget.Budget
			switch variant {
			case "opaque marker spelling":
				desc, _ := sourceBridgeUTF8(object, bridge.DescriptorIndex)
				params := methodParamFieldDescriptors(desc)
				marker := strings.TrimSuffix(strings.TrimPrefix(params[len(params)-1], "L"), ";")
				for _, constant := range object.ConstantPool {
					if text, ok := constant.(*ConstantUtf8Info); ok {
						text.Value = strings.ReplaceAll(text.Value, marker, "UnusedConstructorPort")
					}
				}
			case "ordinary constructor":
				bridge.AccessFlags = 0
			case "wrong receiver":
				code.Code[0] = core.OP_ALOAD_1
			case "wrong argument":
				code.Code[1] = core.OP_ALOAD_0
			case "effectful prefix":
				code.Code = append([]byte{core.OP_ACONST_NULL, core.OP_POP}, code.Code...)
				code.AttrLen += 2
			case "missing delegation":
				code.Code[len(code.Code)-4] = core.OP_INVOKEVIRTUAL
			case "nonprivate target":
				for _, m := range object.Methods {
					if name, _ := sourceBridgeUTF8(object, m.NameIndex); name == "<init>" && m != bridge {
						m.AccessFlags = 0
					}
				}
			case "duplicate bridge":
				object.Methods = append(object.Methods, bridge)
			case "different throws":
				bridge.Attributes = append(bridge.Attributes, &ExceptionsAttribute{ExceptionIndexTable: []uint16{uint16(object.ConstantPoolManager.AddNewClassInfo("java/io/IOException"))}})
			case "kill switch":
				t.Setenv("JDEC_NO_ENUM_MARKER_CTOR", "1")
			case "modern nest version":
				object.MajorVersion = 55
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			d := NewClassObjectDumper(object)
			d.Work = work
			descriptor, _ := sourceBridgeUTF8(object, bridge.DescriptorIndex)
			got := d.syntheticEnumConstructorBridges()[descriptor] != nil
			if got != (variant == "original" || variant == "opaque marker spelling") {
				t.Fatal("constructor erasure must require the unused typed forwarding port", variant, got)
			}
		})
	}
}

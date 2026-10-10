package methodir

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"math"
	"strings"
	"testing"
)

func TestLDCInstructionUsesOriginalLiteralPool(t *testing.T) {
	literals := map[int]values.JavaValue{
		1:   values.NewJavaLiteral(int32(-1234567), types.NewJavaPrimer(types.JavaInteger)),
		257: values.NewJavaLiteral(math.Float32frombits(0x80000000), types.NewJavaPrimer(types.JavaFloat)),
		258: values.NewJavaLiteral(int64(0x1020304050607080), types.NewJavaPrimer(types.JavaLong)),
		259: values.NewJavaLiteral(math.Float64frombits(0x8000000000000000), types.NewJavaPrimer(types.JavaDouble)),
	}
	d := core.NewDecompiler(nil, func(int) values.JavaValue { panic("literal dispatched to member getter") })
	d.ConstantPoolLiteralGetter = func(index int) values.JavaValue { return literals[index] }
	for _, tc := range []struct {
		op, index int
		want      Const
	}{
		{core.OP_LDC, 1, Const{Kind: ConstInt, Int: -1234567}},
		{core.OP_LDC_W, 257, Const{Kind: ConstFloat, FloatBits: 0x80000000}},
		{core.OP_LDC2_W, 258, Const{Kind: ConstLong, Long: 0x1020304050607080}},
		{core.OP_LDC2_W, 259, Const{Kind: ConstDouble, DoubleBits: 0x8000000000000000}},
	} {
		data := []byte{byte(tc.index >> 8), byte(tc.index)}
		if tc.op == core.OP_LDC {
			data = data[1:]
		}
		op := &core.OpCode{Instr: core.InstrInfos[tc.op], Data: data}
		got, err := copyInstr(op, d)
		if err != nil || got.Const != tc.want || int(got.CPIndex) != tc.index {
			t.Errorf("CP opcode 0x%x index%d: got%+v err%v want%+v", tc.op, tc.index, got.Const, err, tc.want)
		}
	}
}

func TestLiteralPoolKindUsesDeclaredComputationalCategory(t *testing.T) {
	for _, tc := range []struct {
		data any
		typ  string
		want Const
	}{
		{int(5), types.JavaLong, Const{Kind: ConstLong, Long: 5}},
		{int64(6), types.JavaLong, Const{Kind: ConstLong, Long: 6}},
		{float32(1.25), types.JavaFloat, Const{Kind: ConstFloat, FloatBits: math.Float32bits(1.25)}},
		{float64(3.25), types.JavaDouble, Const{Kind: ConstDouble, DoubleBits: math.Float64bits(3.25)}},
		{"word", types.JavaString, Const{Kind: ConstString, String: "word"}},
		{int64(1 << 40), types.JavaInteger, Const{}},
		{float64(2), types.JavaFloat, Const{}},
		{"not-int", types.JavaInteger, Const{}},
	} {
		got := literalConst(values.NewJavaLiteral(tc.data, types.NewJavaPrimer(tc.typ)))
		if got != tc.want {
			t.Errorf("declared %s payload %T: got%+v want%+v", tc.typ, tc.data, got, tc.want)
		}
	}
}

func TestLiteralPoolRejectsUnprovedDynamicAndMemberValues(t *testing.T) {
	for _, value := range []values.JavaValue{
		values.NewCustomValue(nil, func() types.JavaType { return types.NewJavaClass("java.lang.String") }),
		values.NewJavaClassMember("java.lang.String", "length", "()I", nil),
		(*values.JavaLiteral)(nil), (*values.JavaClassValue)(nil),
	} {
		d := core.NewDecompiler(nil, func(int) values.JavaValue { return value })
		d.ConstantPoolLiteralGetter = func(int) values.JavaValue { return value }
		if got := decodeLiteralCP(d, 1); got.Kind != ConstNone {
			t.Errorf("unproved loadable value %T acquired kind%d", value, got.Kind)
		}
	}
	d := core.NewDecompiler(nil, func(int) values.JavaValue {
		return values.NewJavaLiteral(int32(7), types.NewJavaPrimer(types.JavaInteger))
	})
	d.ConstantPoolLiteralGetter = func(int) values.JavaValue { panic("original literal CP rejects this entry") }
	if got := decodeLiteralCP(d, 1); got.Kind != ConstNone {
		t.Fatal("failed original literal witness fell back to member getter")
	}
	d.ConstantPoolLiteralGetter = nil
	if got := decodeLiteralCP(d, 1); got.Kind != ConstInt || got.Int != 7 {
		t.Fatal("standalone unified pool lost exact literal witness")
	}
	d.ConstantPoolLiteralGetter = func(int) values.JavaValue { return values.NewJavaClassValue(types.NewJavaClass("java.lang.String")) }
	if got := decodeLiteralCP(d, 1); got.Kind != ConstClass || got.Class != "java/lang/String" {
		t.Fatalf("class literal %v", got)
	}
}

func TestLiteralPoolPreservesFloatingPointPayloadBits(t *testing.T) {
	for _, bits := range []uint32{0, 0x80000000, 0x7f800000, 0xff800000, 0x7fc01234, 0x00000001} {
		got := literalConst(values.NewJavaLiteral(math.Float32frombits(bits), types.NewJavaPrimer(types.JavaFloat)))
		if got.Kind != ConstFloat || got.FloatBits != bits {
			t.Fatalf("float payload %08x ->%+v", bits, got)
		}
	}
	for _, bits := range []uint64{0, 0x8000000000000000, 0x7ff0000000000000, 0xfff0000000000000, 0x7ff8000000001234, 1} {
		got := literalConst(values.NewJavaLiteral(math.Float64frombits(bits), types.NewJavaPrimer(types.JavaDouble)))
		if got.Kind != ConstDouble || got.DoubleBits != bits {
			t.Fatalf("double payload %016x ->%+v", bits, got)
		}
	}
}

func TestClassPoolIdentityNeverUsesSourceDisplayNames(t *testing.T) {
	for _, name := range []string{"java/lang/String", "java/lang/StringBuilder", "alpha/Same", "beta/Same", "outer/Host$Nested", "[I", "[[J", "[[Ljava/lang/String;", "[[Lalpha/Same;"} {
		typ := types.NewJavaClass(name)
		if got := typeInternal(typ); got != name {
			t.Errorf("original class identity %q became display spelling %q", name, got)
		}
	}
	for _, name := range []string{"int", "long", "double", "void"} {
		if got := typeInternal(types.NewJavaPrimer(name)); got != "" {
			t.Errorf("nonclass primitive acquired CP Class identity %q", got)
		}
	}
	base := types.NewJavaClass("java.lang.String")
	for i := 0; i < 255; i++ {
		base = types.NewJavaArrayType(base)
	}
	if got := typeInternal(base); got != strings.Repeat("[", 255)+"Ljava/lang/String;" {
		t.Fatal("valid255-dimension descriptor refused")
	}
	base = types.NewJavaArrayType(base)
	if got := typeInternal(base); got != "" {
		t.Fatal("out-of-range JVM dimension accepted")
	}
}

package lower

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar/model"
)

// wideFixture builds a class that exercises long, float and double register
// values: category-2 arguments and returns, whole-word and floating constants,
// conversions, comparisons, wide fields and wide array elements.
func wideFixture() (*model.File, *model.Class) {
	const owner = "Ldex/Wide;"
	const (
		typeLong    = 0
		typeDouble  = 1
		typeFloat   = 2
		typeInt     = 3
		typeLongArr = 4
	)
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: "J"},
			{Descriptor: "D"},
			{Descriptor: "F"},
			{Descriptor: "I"},
			{Descriptor: "[J"},
		},
		Protos: []model.Proto{
			{ReturnType: "J", Parameters: []string{"J", "J"}, ParameterTypes: []uint32{typeLong, typeLong}},
			{ReturnType: "D", Parameters: []string{"D", "D"}, ParameterTypes: []uint32{typeDouble, typeDouble}},
			{ReturnType: "F", Parameters: []string{"F", "F"}, ParameterTypes: []uint32{typeFloat, typeFloat}},
			{ReturnType: "J", Parameters: []string{"I"}, ParameterTypes: []uint32{typeInt}},
			{ReturnType: "D"},
			{ReturnType: "F"},
			{ReturnType: "D", Parameters: []string{"J"}, ParameterTypes: []uint32{typeLong}},
			{ReturnType: "J", Parameters: []string{"D"}, ParameterTypes: []uint32{typeDouble}},
			{ReturnType: "F", Parameters: []string{"F"}, ParameterTypes: []uint32{typeFloat}},
			{ReturnType: "J", Parameters: []string{"J"}, ParameterTypes: []uint32{typeLong}},
			{ReturnType: "I", Parameters: []string{"J", "J"}, ParameterTypes: []uint32{typeLong, typeLong}},
			{ReturnType: "J", Parameters: []string{"[J", "I"}, ParameterTypes: []uint32{typeLongArr, typeInt}},
			{ReturnType: "V", Parameters: []string{"[J", "I", "J"}, ParameterTypes: []uint32{typeLongArr, typeInt, typeLong}},
			{ReturnType: "[J", Parameters: []string{"I"}, ParameterTypes: []uint32{typeInt}},
			{ReturnType: "V", Parameters: []string{"J"}, ParameterTypes: []uint32{typeLong}},
			{ReturnType: "J"},
			{ReturnType: "D", Parameters: []string{"Z"}, ParameterTypes: []uint32{typeInt}},
			{ReturnType: "J", Parameters: []string{"Z", "J"}, ParameterTypes: []uint32{typeInt, typeLong}},
			{ReturnType: "J"},
			{ReturnType: "D"},
		},
		Fields: []model.FieldID{
			{Class: owner, Type: "J", Name: "MAX"},
		},
	}
	names := []string{
		"addLong", "addDouble", "mulFloat", "widen", "constDouble", "constFloat",
		"toDouble", "toLong", "negFloat", "notLong", "cmpLong", "arrayGet",
		"arraySet", "newLongs", "setTotal", "getTotal", "crossBlock", "pickLong",
		"constSmall", "moveConstDouble",
	}
	methods := make([]model.MethodID, len(names))
	for index, name := range names {
		methods[index] = model.MethodID{Class: owner, Name: name, ProtoIndex: uint16(index)}
	}
	file.Methods = methods
	class := &model.Class{
		Descriptor:   owner,
		Superclass:   "Ljava/lang/Object;",
		AccessFlags:  dexPublic,
		StaticFields: []model.EncodedField{{AccessFlags: dexPublic | dexStatic, Field: file.Fields[0]}},
		StaticValues: []model.EncodedValue{{Type: model.ValueLong, Signed: 1000}},
	}
	direct := func(index int, registers, ins, outs uint16, words ...uint16) model.EncodedMethod {
		return model.EncodedMethod{
			AccessFlags: dexPublic | dexStatic,
			Method:      methods[index],
			Code:        &model.Code{RegistersSize: registers, InsSize: ins, OutsSize: outs, Instructions: words},
		}
	}
	class.DirectMethods = []model.EncodedMethod{
		// add-long v0, v2, v4; return-wide v0
		direct(0, 6, 4, 0, 0x009b, 0x0402, 0x0010),
		// add-double v0, v2, v4; return-wide v0
		direct(1, 6, 4, 0, 0x00ab, 0x0402, 0x0010),
		// mul-float v0, v1, v2; return v0
		direct(2, 3, 2, 0, 0x00a8, 0x0201, 0x000f),
		// int-to-long v0, v2; return-wide v0
		direct(3, 3, 1, 0, 0x2081, 0x0010),
		// const-wide v0, 0x3ff0000000000000 (1.0); return-wide v0
		direct(4, 2, 0, 0, 0x0018, 0x0000, 0x0000, 0x0000, 0x3ff0, 0x0010),
		// const/high16 v0, #0x3f80 (1.0f); return v0
		direct(5, 1, 0, 0, 0x0015, 0x3f80, 0x000f),
		// long-to-double v0, v2; return-wide v0
		direct(6, 4, 2, 0, 0x2086, 0x0010),
		// double-to-long v0, v2; return-wide v0
		direct(7, 4, 2, 0, 0x208b, 0x0010),
		// neg-float v0, v1; return v0
		direct(8, 2, 1, 0, 0x107f, 0x000f),
		// not-long v0, v2; return-wide v0
		direct(9, 4, 2, 0, 0x207e, 0x0010),
		// cmp-long v0, v1, v3; return v0
		direct(10, 5, 4, 0, 0x0031, 0x0301, 0x000f),
		// aget-wide v0, v3, v4; return-wide v0
		direct(11, 5, 2, 0, 0x0045, 0x0403, 0x0010),
		// aput-wide v2, v0, v1; return-void
		direct(12, 4, 4, 0, 0x024c, 0x0100, 0x000e),
		// new-array v0, v1, [J; return-object v0
		direct(13, 2, 1, 0, 0x1023, typeLongArr, 0x0011),
		// sput-wide v0, Wide.MAX; return-void
		direct(14, 2, 2, 0, 0x0068, 0x0000, 0x000e),
		// sget-wide v0, Wide.MAX; return-wide v0
		direct(15, 2, 0, 0, 0x0061, 0x0000, 0x0010),
		// const-wide v0, 1.0; if-eqz v4, +3; return-wide v0; const-wide v2, 2.0; return-wide v2
		direct(16, 5, 1, 0,
			0x0018, 0x0000, 0x0000, 0x0000, 0x3ff0,
			0x0438, 0x0003,
			0x0010,
			0x0218, 0x0000, 0x0000, 0x0000, 0x4000,
			0x0210),
		// const-wide/16 v0, #4; if-eqz v2, +2; move-wide v0, v3; return-wide v0
		direct(17, 5, 3, 0,
			0x0016, 0x0004,
			0x0238, 0x0003,
			0x3004,
			0x0010),
		// const-wide/16 v0, #12345; return-wide v0
		direct(18, 2, 0, 0, 0x0016, 0x3039, 0x0010),
		// const-wide v0, 1.0; move-wide v2, v0; return-wide v2
		direct(19, 4, 0, 0, 0x0018, 0x0000, 0x0000, 0x0000, 0x3ff0, 0x0204, 0x0210),
	}
	return file, class
}

func TestLowerWideValuesVerifyAndExecute(t *testing.T) {
	file, class := wideFixture()
	raw, err := Class(file, class)
	if err != nil {
		t.Fatalf("Class: %v", err)
	}
	if _, err := javaclassparser.Parse(raw); err != nil {
		t.Fatalf("parse generated class: %v", err)
	}
	javac, javacErr := exec.LookPath("javac")
	java, javaErr := exec.LookPath("java")
	if javacErr != nil || javaErr != nil {
		t.Skip("javac/java unavailable")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "dex", "Wide.class")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	driver := filepath.Join(dir, "Driver.java")
	source := `public class Driver {
    public static void main(String[] args) {
        if (dex.Wide.addLong(2L, 3L) != 5L) throw new AssertionError("addLong");
        if (dex.Wide.addDouble(1.5, 2.25) != 3.75) throw new AssertionError("addDouble");
        if (dex.Wide.mulFloat(1.5f, 4f) != 6f) throw new AssertionError("mulFloat");
        if (dex.Wide.widen(7) != 7L) throw new AssertionError("widen");
        if (dex.Wide.constDouble() != 1.0) throw new AssertionError("constDouble");
        if (dex.Wide.constFloat() != 1.0f) throw new AssertionError("constFloat");
        if (dex.Wide.constSmall() != 12345L) throw new AssertionError("constSmall");
        if (dex.Wide.moveConstDouble() != 1.0) throw new AssertionError("moveConstDouble");
        if (dex.Wide.toDouble(3L) != 3.0) throw new AssertionError("toDouble");
        if (dex.Wide.toLong(3.9) != 3L) throw new AssertionError("toLong");
        if (dex.Wide.negFloat(2.5f) != -2.5f) throw new AssertionError("negFloat");
        if (dex.Wide.notLong(0L) != -1L) throw new AssertionError("notLong");
        if (dex.Wide.cmpLong(4L, 4L) != 0) throw new AssertionError("cmpLong");
        if (dex.Wide.cmpLong(4L, 5L) >= 0) throw new AssertionError("cmpLong order");
        if (dex.Wide.arrayGet(new long[]{1L, 2L, 3L}, 1) != 2L) throw new AssertionError("arrayGet");
        long[] arr = new long[2];
        dex.Wide.arraySet(arr, 0, 42L);
        if (arr[0] != 42L) throw new AssertionError("arraySet");
        if (dex.Wide.newLongs(3).length != 3) throw new AssertionError("newLongs");
        if (dex.Wide.MAX != 1000L) throw new AssertionError("ConstantValue");
        dex.Wide.setTotal(99L);
        if (dex.Wide.getTotal() != 99L) throw new AssertionError("static field");
        if (dex.Wide.MAX != 99L) throw new AssertionError("static field write");
        if (dex.Wide.crossBlock(true) != 1.0) throw new AssertionError("crossBlock true");
        if (dex.Wide.crossBlock(false) != 2.0) throw new AssertionError("crossBlock false");
        if (dex.Wide.pickLong(true, 8L) != 8L) throw new AssertionError("pickLong true");
        if (dex.Wide.pickLong(false, 8L) != 4L) throw new AssertionError("pickLong false");
    }
}`
	if err := os.WriteFile(driver, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(javac, "-cp", dir, "-d", dir, driver).CombinedOutput(); err != nil {
		t.Fatalf("javac driver: %v\n%s", err, output)
	}
	if output, err := exec.Command(java, "-Xverify:all", "-cp", dir, "Driver").CombinedOutput(); err != nil {
		t.Fatalf("wide driver execution: %v\n%s", err, output)
	}
}

package lower

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar/model"
)

// interfaceStaticFixture builds an interface with a static field and static
// methods, which requires a Java 8 class file.
func interfaceStaticFixture() (*model.File, *model.Class) {
	const owner = "Ldex/Statics;"
	field := model.FieldID{Class: owner, Name: "VALUE", Type: "I"}
	file := &model.File{
		Entry:  "classes.dex",
		Types:  []model.Type{{Descriptor: "I"}},
		Protos: []model.Proto{{ReturnType: "I"}, {ReturnType: "V", Parameters: []string{"I"}, ParameterTypes: []uint32{0}}},
		Methods: []model.MethodID{
			{Class: owner, Name: "answer", ProtoIndex: 0},
			{Class: owner, Name: "set", ProtoIndex: 1},
			{Class: owner, Name: "get", ProtoIndex: 0},
		},
		Fields: []model.FieldID{field},
	}
	class := &model.Class{
		Descriptor:   owner,
		Superclass:   "Ljava/lang/Object;",
		AccessFlags:  dexPublic | 0x0200 | 0x0400,
		StaticFields: []model.EncodedField{{FieldIndex: 0, AccessFlags: dexPublic | dexStatic | dexFinal, Field: field}},
		DirectMethods: []model.EncodedMethod{
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[0],
				Code: &model.Code{RegistersSize: 1, Instructions: []uint16{
					0x0013, 0x002a, // const/16 v0, #42
					0x000f, // return v0
				}},
			},
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[1],
				Code: &model.Code{RegistersSize: 1, InsSize: 1, Instructions: []uint16{
					0x0067, 0x0000, // sput v0, VALUE
					0x000e, // return-void
				}},
			},
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[2],
				Code: &model.Code{RegistersSize: 1, Instructions: []uint16{
					0x0060, 0x0000, // sget v0, VALUE
					0x000f, // return v0
				}},
			},
		},
	}
	return file, class
}

func TestLowerRejectsIllegalInterfaceMemberShapes(t *testing.T) {
	const owner = "Ldex/IllegalInterface;"
	tests := []struct {
		name    string
		prepare func(*model.File, *model.Class)
		want    string
	}{
		{
			name: "field visibility",
			prepare: func(_ *model.File, class *model.Class) {
				class.StaticFields = []model.EncodedField{{
					AccessFlags: dexPrivate | dexStatic | dexFinal,
					Field:       model.FieldID{Class: owner, Name: "VALUE", Type: "I"},
				}}
			},
			want: "interface field must be public static final",
		},
		{
			name: "instance constructor",
			prepare: func(file *model.File, class *model.Class) {
				class.DirectMethods = []model.EncodedMethod{{
					AccessFlags: dexPublic | dexConstructor,
					Method:      model.MethodID{Class: owner, Name: "<init>", ProtoIndex: 0},
					Code:        &model.Code{RegistersSize: 1, InsSize: 1, Instructions: []uint16{0x000e}},
				}}
				file.Protos = []model.Proto{{ReturnType: "V"}}
			},
			want: "interface must not declare an instance constructor",
		},
		{
			name: "abstract static method",
			prepare: func(file *model.File, class *model.Class) {
				class.DirectMethods = []model.EncodedMethod{{
					AccessFlags: dexPublic | dexStatic | dexAbstract,
					Method:      model.MethodID{Class: owner, Name: "run", ProtoIndex: 0},
				}}
				file.Protos = []model.Proto{{ReturnType: "V"}}
			},
			want: "abstract method has incompatible access flags",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file := &model.File{Entry: "classes.dex"}
			class := &model.Class{Descriptor: owner, Superclass: "Ljava/lang/Object;", AccessFlags: dexPublic | dexInterface | dexAbstract}
			test.prepare(file, class)
			if _, err := ClassWithPolicy(file, class, ClassOptions{}, Recover); err != nil {
				t.Fatalf("Recover should fall back to a minimal shell: %v", err)
			}
			if _, err := Class(file, class); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Class error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLowerInterfaceStaticMethodsVerifyAndExecute(t *testing.T) {
	file, class := interfaceStaticFixture()
	raw, err := Class(file, class)
	if err != nil {
		t.Fatalf("Class: %v", err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatalf("parse generated interface: %v", err)
	}
	if parsed.MajorVersion != 52 {
		t.Fatalf("class version = %d, want 52", parsed.MajorVersion)
	}
	javac, javacErr := exec.LookPath("javac")
	java, javaErr := exec.LookPath("java")
	if javacErr != nil || javaErr != nil {
		t.Skip("javac/java unavailable")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "dex", "Statics.class")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	driver := filepath.Join(dir, "Driver.java")
	source := `public class Driver {
    public static void main(String[] args) {
        if (dex.Statics.answer() != 42) throw new AssertionError("answer");
        dex.Statics.set(7);
        if (dex.Statics.get() != 7) throw new AssertionError("field");
    }
}`
	if err := os.WriteFile(driver, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(javac, "-cp", dir, "-d", dir, driver).CombinedOutput(); err != nil {
		t.Fatalf("javac driver: %v\n%s", err, output)
	}
	if output, err := exec.Command(java, "-Xverify:all", "-cp", dir, "Driver").CombinedOutput(); err != nil {
		t.Fatalf("interface driver execution: %v\n%s", err, output)
	}
}

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

func inspectInvokeSpecialTarget(t *testing.T, raw []byte, methodName string) (major uint16, owner string, isInterfaceRef bool) {
	t.Helper()
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatalf("Parse class: %v", err)
	}
	for _, m := range parsed.Methods {
		utf8 := parsed.ConstantPoolManager.GetUtf8(int(m.NameIndex))
		if utf8 == nil || utf8.Value != methodName {
			continue
		}
		for _, attr := range m.Attributes {
			ca, ok := attr.(*javaclassparser.CodeAttribute)
			if !ok {
				continue
			}
			for i := 0; i+2 < len(ca.Code); i++ {
				if ca.Code[i] == 0xb7 { // INVOKESPECIAL
					cpIdx := int(ca.Code[i+1])<<8 | int(ca.Code[i+2])
					switch ref := parsed.ConstantPool[cpIdx-1].(type) {
					case *javaclassparser.ConstantInterfaceMethodrefInfo:
						return parsed.MajorVersion, parsed.ConstantPoolManager.GetClassName(int(ref.ClassIndex)), true
					case *javaclassparser.ConstantMethodrefInfo:
						return parsed.MajorVersion, parsed.ConstantPoolManager.GetClassName(int(ref.ClassIndex)), false
					}
				}
			}
		}
	}
	t.Fatalf("method %q did not emit INVOKESPECIAL", methodName)
	return 0, "", false
}

func TestLowerInvokeSuperInterfacesAndBridgeVerify(t *testing.T) {
	const (
		baseI    = "Ldex/isuper/BaseI;"
		midI     = "Ldex/isuper/MidI;"
		leafI    = "Ldex/isuper/LeafI;"
		directC  = "Ldex/isuper/DirectImpl;"
		baseMap  = "Ldex/isuper/BaseMap;"
		childMap = "Ldex/isuper/ChildMap;"
		markerI  = "Ldex/isuper/MarkerMap;"
	)

	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: "I"},
			{Descriptor: "Ljava/lang/Object;"},
		},
		Protos: []model.Proto{
			{ReturnType: "I"},                                                                     // 0: ()I
			{ReturnType: "V"},                                                                     // 1: ()V
			{ReturnType: "Ljava/lang/Object;", Parameters: []string{"Ljava/lang/Object;", "Ljava/lang/Object;"}, ParameterTypes: []uint32{1, 1}}, // 2: (Object,Object)Object
		},
		Methods: []model.MethodID{
			{Class: baseI, Name: "foo", ProtoIndex: 0},                               // 0
			{Class: midI, Name: "foo", ProtoIndex: 0},                                // 1
			{Class: leafI, Name: "foo", ProtoIndex: 0},                               // 2
			{Class: leafI, Name: "access$foo$jd", ProtoIndex: 0},                     // 3 (static bridge taking LeafI)
			{Class: directC, Name: "<init>", ProtoIndex: 1},                          // 4
			{Class: directC, Name: "foo", ProtoIndex: 0},                             // 5
			{Class: baseMap, Name: "<init>", ProtoIndex: 1},                          // 6
			{Class: childMap, Name: "<init>", ProtoIndex: 1},                         // 7
			{Class: "Ljava/util/Map;", Name: "getOrDefault", ProtoIndex: 2},          // 8
			{Class: childMap, Name: "getOrDefault", ProtoIndex: 2},                   // 9
			{Class: "Ljava/util/HashMap;", Name: "<init>", ProtoIndex: 1},            // 10
			{Class: baseMap, Name: "getOrDefault", ProtoIndex: 2},                    // 11
			{Class: "Ljava/lang/Object;", Name: "<init>", ProtoIndex: 1},             // 12
		},
	}

	// Adjust access$foo$jd to take LeafI so static invoke-super {v0}, LeafI.foo() is type-valid.
	file.Types = append(file.Types, model.Type{Descriptor: leafI})
	file.Protos = append(file.Protos, model.Proto{ReturnType: "I", Parameters: []string{leafI}, ParameterTypes: []uint32{2}})
	file.Methods[3] = model.MethodID{Class: leafI, Name: "access$foo$jd", ProtoIndex: 3}

	classes := []model.Class{
		{
			// 0: BaseI { default int foo() { return 10; } }
			Descriptor:  baseI,
			Superclass:  "Ljava/lang/Object;",
			AccessFlags: dexPublic | dexInterface | dexAbstract,
			VirtualMethods: []model.EncodedMethod{{
				AccessFlags: dexPublic,
				Method:      file.Methods[0],
				Code:        &model.Code{RegistersSize: 2, InsSize: 1, Instructions: []uint16{0xa012, 0x000f}}, // const/4 v0, #-6? Wait: 0x12 sets 4-bit signed; 7 is max positive
			}},
		},
		{
			// 1: MidI extends BaseI { @Override default int foo() { return BaseI.super.foo(); } }
			Descriptor:  midI,
			Superclass:  "Ljava/lang/Object;",
			Interfaces:  []string{baseI},
			AccessFlags: dexPublic | dexInterface | dexAbstract,
			VirtualMethods: []model.EncodedMethod{{
				AccessFlags: dexPublic,
				Method:      file.Methods[1],
				Code: &model.Code{RegistersSize: 1, InsSize: 1, OutsSize: 1, Instructions: []uint16{
					0x106f, 0x0000, 0x0000, // invoke-super {v0}, BaseI.foo()
					0x000a,                 // move-result v0
					0x000f,                 // return v0
				}},
			}},
		},
		{
			// 2: LeafI extends MidI:
			//    foo() calls indirect superinterface BaseI via invoke-super -> must retarget to MidI
			//    static access$foo$jd(LeafI) calls self-interface LeafI via invoke-super (Kotlin -Xjvm-default=all pattern)
			Descriptor:  leafI,
			Superclass:  "Ljava/lang/Object;",
			Interfaces:  []string{midI},
			AccessFlags: dexPublic | dexInterface | dexAbstract,
			DirectMethods: []model.EncodedMethod{{
				AccessFlags: dexPublic | dexStatic | dexSynthetic,
				Method:      file.Methods[3],
				Code: &model.Code{RegistersSize: 1, InsSize: 1, OutsSize: 1, Instructions: []uint16{
					0x106f, 0x0002, 0x0000, // invoke-super {v0}, LeafI.foo()
					0x000a,
					0x000f,
				}},
			}},
			VirtualMethods: []model.EncodedMethod{{
				AccessFlags: dexPublic,
				Method:      file.Methods[2],
				Code: &model.Code{RegistersSize: 1, InsSize: 1, OutsSize: 1, Instructions: []uint16{
					0x106f, 0x0000, 0x0000, // invoke-super {v0}, BaseI.foo()
					0x000a,
					0x000f,
				}},
			}},
		},
		{
			// 3: DirectImpl implements BaseI { public int foo() { return BaseI.super.foo(); } }
			Descriptor:  directC,
			Superclass:  "Ljava/lang/Object;",
			Interfaces:  []string{baseI},
			AccessFlags: dexPublic,
			DirectMethods: []model.EncodedMethod{{
				AccessFlags: dexPublic | dexConstructor,
				Method:      file.Methods[4],
				Code:        &model.Code{RegistersSize: 1, InsSize: 1, OutsSize: 1, Instructions: []uint16{0x1070, 0x000c, 0x0000, 0x000e}},
			}},
			VirtualMethods: []model.EncodedMethod{{
				AccessFlags: dexPublic,
				Method:      file.Methods[5],
				Code: &model.Code{RegistersSize: 1, InsSize: 1, OutsSize: 1, Instructions: []uint16{
					0x106f, 0x0000, 0x0000, // invoke-super {v0}, BaseI.foo()
					0x000a,
					0x000f,
				}},
			}},
		},
		{
			// 4: BaseMap extends java.util.HashMap (which implements java.util.Map)
			Descriptor:  baseMap,
			Superclass:  "Ljava/util/HashMap;",
			AccessFlags: dexPublic,
			DirectMethods: []model.EncodedMethod{{
				AccessFlags: dexPublic | dexConstructor,
				Method:      file.Methods[6],
				Code:        &model.Code{RegistersSize: 1, InsSize: 1, OutsSize: 1, Instructions: []uint16{0x1070, 0x000a, 0x0000, 0x000e}},
			}},
		},
		{
			// 5: MarkerMap {} (empty marker interface)
			Descriptor:  markerI,
			Superclass:  "Ljava/lang/Object;",
			Interfaces:  []string{"Ljava/util/Map;"},
			AccessFlags: dexPublic | dexInterface | dexAbstract,
		},
		{
			// 6: ChildMap extends BaseMap implements MarkerMap:
			//    invoke-super {v0, v1, v2}, Ljava/util/Map;.getOrDefault -> must retarget to BaseMap Methodref!
			Descriptor:  childMap,
			Superclass:  baseMap,
			Interfaces:  []string{markerI},
			AccessFlags: dexPublic,
			DirectMethods: []model.EncodedMethod{{
				AccessFlags: dexPublic | dexConstructor,
				Method:      file.Methods[7],
				Code:        &model.Code{RegistersSize: 1, InsSize: 1, OutsSize: 1, Instructions: []uint16{0x1070, 0x0006, 0x0000, 0x000e}},
			}},
			VirtualMethods: []model.EncodedMethod{{
				AccessFlags: dexPublic,
				Method:      file.Methods[9],
				Code: &model.Code{RegistersSize: 3, InsSize: 3, OutsSize: 3, Instructions: []uint16{
					0x306f, 0x0008, 0x0210, // invoke-super {v0, v1, v2}, Map.getOrDefault
					0x000c,                 // move-result-object v0
					0x0011,                 // return-object v0
				}},
			}},
		},
	}
	// Fix BaseI.foo return constant to 7 (valid signed 4-bit literal for const/4: v0=7 -> 0x7012).
	classes[0].VirtualMethods[0].Code.Instructions[0] = 0x7012
	classes[0].VirtualMethods[0].Code.RegistersSize = 1
	// Also make BaseMap declare implements java/util/Map explicitly so index knows BaseMap implements Map.
	classes[4].Interfaces = []string{"Ljava/util/Map;"}

	file.Classes = classes
	hierarchy := map[string]*model.Class{}
	for i := range file.Classes {
		hierarchy[file.Classes[i].Descriptor] = &file.Classes[i]
	}

	opts := ClassOptions{Hierarchy: hierarchy}
	lowered := make(map[string][]byte, len(file.Classes))
	for i := range file.Classes {
		raw, err := ClassWithOptions(file, &file.Classes[i], opts)
		if err != nil {
			t.Fatalf("ClassWithOptions(%s): %v", file.Classes[i].Descriptor, err)
		}
		lowered[file.Classes[i].Descriptor] = raw
	}

	if ver, owner, isIf := inspectInvokeSpecialTarget(t, lowered[midI], "foo"); ver < 52 || !isIf || owner != "dex/isuper/BaseI" {
		t.Fatalf("MidI.foo = (ver=%d, owner=%q, isIf=%v), want (>=52, dex/isuper/BaseI, true)", ver, owner, isIf)
	}
	if ver, owner, isIf := inspectInvokeSpecialTarget(t, lowered[leafI], "foo"); ver < 52 || !isIf || owner != "dex/isuper/MidI" {
		t.Fatalf("LeafI.foo = (ver=%d, owner=%q, isIf=%v), want (>=52, dex/isuper/MidI, true)", ver, owner, isIf)
	}
	if ver, owner, isIf := inspectInvokeSpecialTarget(t, lowered[leafI], "access$foo$jd"); ver < 52 || !isIf || owner != "dex/isuper/LeafI" {
		t.Fatalf("LeafI.access$foo$jd = (ver=%d, owner=%q, isIf=%v), want (>=52, dex/isuper/LeafI, true)", ver, owner, isIf)
	}
	if ver, owner, isIf := inspectInvokeSpecialTarget(t, lowered[directC], "foo"); ver < 52 || !isIf || owner != "dex/isuper/BaseI" {
		t.Fatalf("DirectImpl.foo = (ver=%d, owner=%q, isIf=%v), want (>=52, dex/isuper/BaseI, true)", ver, owner, isIf)
	}
	if _, owner, isIf := inspectInvokeSpecialTarget(t, lowered[childMap], "getOrDefault"); isIf || owner != "dex/isuper/BaseMap" {
		t.Fatalf("ChildMap.getOrDefault = (owner=%q, isIf=%v), want (dex/isuper/BaseMap, false)", owner, isIf)
	}

	// Unrelated target must still fail in Strict mode.
	badFile := &model.File{
		Entry:  "classes.dex",
		Protos: []model.Proto{{ReturnType: "I"}, {ReturnType: "V"}},
		Methods: []model.MethodID{
			{Class: baseI, Name: "foo", ProtoIndex: 0},
			{Class: "Ldex/isuper/Unrelated;", Name: "call", ProtoIndex: 0},
		},
	}
	badClass := &model.Class{
		Descriptor:  "Ldex/isuper/Unrelated;",
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		VirtualMethods: []model.EncodedMethod{{
			AccessFlags: dexPublic,
			Method:      badFile.Methods[1],
			Code:        &model.Code{RegistersSize: 1, InsSize: 1, OutsSize: 1, Instructions: []uint16{0x106f, 0x0000, 0x0000, 0x000a, 0x000f}},
		}},
	}
	badFile.Classes = []model.Class{*badClass}
	badHierarchy := map[string]*model.Class{baseI: &file.Classes[0], badClass.Descriptor: badClass}
	if _, err := ClassWithOptions(badFile, badClass, ClassOptions{Hierarchy: badHierarchy}); err == nil || !strings.Contains(err.Error(), "not a superclass or superinterface") {
		t.Fatalf("expected unrelated invoke-super to fail in Strict mode, got %v", err)
	}

	// Verify generated classes with real JVM bytecode verifier when javac/java exist.
	javac, javacErr := exec.LookPath("javac")
	java, javaErr := exec.LookPath("java")
	if javacErr != nil || javaErr != nil {
		return
	}
	dir := t.TempDir()
	for desc, raw := range lowered {
		rel := strings.TrimSuffix(strings.TrimPrefix(desc, "L"), ";") + ".class"
		outPath := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(outPath, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	driver := filepath.Join(dir, "Driver.java")
	source := `public class Driver {
    public static void main(String[] args) throws Exception {
        dex.isuper.DirectImpl d = new dex.isuper.DirectImpl();
        if (d.foo() != 7) throw new AssertionError("DirectImpl.foo=" + d.foo());
        dex.isuper.LeafI leaf = new dex.isuper.LeafI() {};
        if (leaf.foo() != 7) throw new AssertionError("LeafI.foo=" + leaf.foo());
        Object bridgeRes = dex.isuper.LeafI.class.getDeclaredMethod("access$foo$jd", dex.isuper.LeafI.class).invoke(null, leaf);
        if (!Integer.valueOf(7).equals(bridgeRes)) throw new AssertionError("access$foo$jd=" + bridgeRes);
        dex.isuper.ChildMap cm = new dex.isuper.ChildMap();
        if (!"fallback".equals(cm.getOrDefault("k", "fallback"))) throw new AssertionError("ChildMap.getOrDefault");
    }
}`
	if err := os.WriteFile(driver, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(javac, "-cp", dir, "-d", dir, driver).CombinedOutput(); err != nil {
		t.Fatalf("javac driver: %v\n%s", err, output)
	}
	if output, err := exec.Command(java, "-Xverify:all", "-cp", dir, "Driver").CombinedOutput(); err != nil {
		t.Fatalf("java -Xverify:all failed: %v\n%s", err, output)
	}
}

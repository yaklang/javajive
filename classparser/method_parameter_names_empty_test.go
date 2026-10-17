package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

const emptyMethodParameterFixture = `class EmptyParameterOwner{public static int ping(){return 7;}public int pong(){return 9;}}class EmptyParameterDriver{public static void main(String[]args)throws Exception{for(String name:new String[]{"ping","pong"}){java.lang.reflect.Method m=EmptyParameterOwner.class.getDeclaredMethod(name);if(m.getParameterCount()!=0||m.getParameters().length!=0||m.getParameterTypes().length!=0||m.getGenericParameterTypes().length!=0||m.getAnnotatedParameterTypes().length!=0||m.getParameterAnnotations().length!=0||m.getModifiers()!=(name.equals("ping")?9:1))throw new AssertionError("empty parameter reflection");}if(EmptyParameterOwner.class.getDeclaredMethods().length!=2||EmptyParameterOwner.ping()!=7||new EmptyParameterOwner().pong()!=9)throw new AssertionError("original no-argument behavior");System.out.println("empty:parameters:0:7:9");}}`

func compileEmptyMethodParameterFixture(t *testing.T, debug string) (map[string][]byte, string) {
	t.Helper()
	javac, _ := t04Tools(t)
	dir := methodParameterNameDirectory(t, "empty-original")
	path := filepath.Join(dir, "EmptyParameterOwner.java")
	if err := os.WriteFile(path, []byte(emptyMethodParameterFixture), 0600); err != nil {
		t.Fatal(err)
	}
	if log, err := exec.Command(javac, "-proc:none", "--release", "8", "-parameters", "-g:"+debug, "-d", dir, path).CombinedOutput(); err != nil {
		t.Fatalf("original compile: %v\n%s", err, log)
	}
	files := map[string][]byte{}
	for _, name := range []string{"EmptyParameterOwner.class", "EmptyParameterDriver.class"} {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = raw
	}
	return files, dir
}

// This new authored classfile protocol adds one legal count=0 attribute.
// Retained Code, flags, descriptors and all other method attributes stay put.
func authoredEmptyMethodParameterTable(t *testing.T, raw []byte, name string) (*ClassObject, *MemberInfo, *UnparsedAttribute) {
	t.Helper()
	object, err := Parse(append([]byte(nil), raw...))
	if err != nil {
		t.Fatal(err)
	}
	NewConstantPoolWithConstant(&object.ConstantPool).AddUtf8Info("MethodParameters")
	for _, method := range object.Methods {
		n, _ := sourceBridgeUTF8(object, method.NameIndex)
		if n != name {
			continue
		}
		d, _ := sourceBridgeUTF8(object, method.DescriptorIndex)
		if d != "()I" {
			t.Fatal("authored no-parameter descriptor")
		}
		before := append([]byte(nil), methodCodeBytes(method)...)
		flags := method.AccessFlags
		table := &UnparsedAttribute{Name: "MethodParameters", Length: 1, Info: []byte{0}}
		method.Attributes = append(method.Attributes, table)
		if !bytes.Equal(before, methodCodeBytes(method)) || flags != method.AccessFlags {
			t.Fatal("authoring changed original Code or flags")
		}
		return object, method, table
	}
	t.Fatal("authored original zero-parameter method")
	return nil, nil, nil
}

func runEmptyMethodParameterDriver(t *testing.T, directory, stage string) string {
	t.Helper()
	_, java := t04Tools(t)
	output, err := exec.Command(java, "-Xverify:all", "-cp", directory, "EmptyParameterDriver").CombinedOutput()
	if writeErr := os.WriteFile(filepath.Join(directory, "empty-jvm-oracle.txt"), output, 0600); writeErr != nil {
		t.Fatal(writeErr)
	}
	t.Logf("%s JVM: %q", stage, output)
	if err != nil || string(output) != "empty:parameters:0:7:9\n" {
		t.Fatalf("%s JVM: %v\n%s", stage, err, output)
	}
	return string(output)
}

func TestAdversarialEmptyMethodParametersKeepNoArgumentReflectionAndBehavior(t *testing.T) {
	for _, world := range []string{"absent", "empty-static", "empty-instance"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(world+"/"+debug+"/"+policy, func(t *testing.T) {
					if policy == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if policy == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					files, original := compileEmptyMethodParameterFixture(t, debug)
					before := files["EmptyParameterOwner.class"]
					if world != "absent" {
						name := "ping"
						if world == "empty-instance" {
							name = "pong"
						}
						object, _, _ := authoredEmptyMethodParameterTable(t, before, name)
						files["EmptyParameterOwner.class"] = object.Bytes()
						if !reflect.DeepEqual(methodParameterNamesAllMethodDeclarations(t, before), methodParameterNamesAllMethodDeclarations(t, files["EmptyParameterOwner.class"])) {
							t.Fatal("authored empty table changed declarations")
						}
					}
					if err := os.WriteFile(filepath.Join(original, "EmptyParameterOwner.class"), files["EmptyParameterOwner.class"], 0600); err != nil {
						t.Fatal(err)
					}
					want := runEmptyMethodParameterDriver(t, original, "original")
					result, err := DecompileWithOptions(files["EmptyParameterOwner.class"], DecompileOptions{Mode: Precision, TargetSourceVersion: 8, SourceCompiler: ModernJavac})
					if err != nil || result.Status != "complete" || len(result.StubMethods) != 0 || strings.Contains(result.Source, DecompileStubMarker) {
						t.Fatalf("no-argument source: %v status=%s stubs=%v\n%s", err, result.Status, result.StubMethods, result.Source)
					}
					rebuilt := methodParameterNameDirectory(t, "empty-rebuilt")
					path := filepath.Join(rebuilt, "EmptyParameterOwner.java")
					if err := os.WriteFile(path, []byte(result.Source), 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(rebuilt, "EmptyParameterDriver.class"), files["EmptyParameterDriver.class"], 0600); err != nil {
						t.Fatal(err)
					}
					javac, _ := t04Tools(t)
					if log, err := exec.Command(javac, "-proc:none", "--release", "8", "-parameters", "-g:"+debug, "-cp", rebuilt, "-d", rebuilt, path).CombinedOutput(); err != nil {
						t.Fatalf("rebuilt compile: %v\n%s", err, log)
					}
					if got := runEmptyMethodParameterDriver(t, rebuilt, "rebuilt"); got != want {
						t.Fatal("original reflection/behavior changed")
					}
					next, err := os.ReadFile(filepath.Join(rebuilt, "EmptyParameterOwner.class"))
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(methodParameterNamesAllMethodDeclarations(t, files["EmptyParameterOwner.class"]), methodParameterNamesAllMethodDeclarations(t, next)) {
						t.Fatal("complete no-argument declarations changed")
					}
					originalTables := methodParameterNamesIndependent(t, files["EmptyParameterOwner.class"])
					wantedTables := 0
					if world != "absent" {
						wantedTables = 1
					}
					if len(originalTables) != wantedTables || len(methodParameterNamesIndependent(t, next)) != 0 {
						t.Fatal("explicit empty/absent raw attribute witness changed")
					}
					if world != "absent" {
						t.Log("Raw empty MethodParameters is present in original and omitted by javac. No Parameter object exists; reflection equivalence is checked, not raw attribute-byte equality.")
					}
				})
			}
		}
	}
}

func TestAdversarialEmptyMethodParametersStillRequireCompleteOriginalTable(t *testing.T) {
	files, dir := compileEmptyMethodParameterFixture(t, "none")
	runEmptyMethodParameterDriver(t, dir, "original absent")
	for _, variant := range []string{"present empty", "absent", "count one short", "count one payload", "extra payload", "missing count", "wrong length", "duplicate attribute", "nil attribute", "duplicate original pointer", "duplicate original copy", "detached method", "wrong method CP kind", "wrong descriptor CP kind", "wrong descriptor request", "illegal major", "budget"} {
		t.Run(variant, func(t *testing.T) {
			object, method, table := authoredEmptyMethodParameterTable(t, files["EmptyParameterOwner.class"], "ping")
			d := NewClassObjectDumper(object)
			descriptor := "()I"
			switch variant {
			case "absent":
				method.Attributes = method.Attributes[:len(method.Attributes)-1]
			case "count one short":
				table.Info = []byte{1}
			case "count one payload":
				table.Info = []byte{1, 0, 0, 0, 0}
				table.Length = 5
			case "extra payload":
				table.Info = []byte{0, 0}
				table.Length = 2
			case "missing count":
				table.Info = nil
				table.Length = 0
			case "wrong length":
				table.Length = 2
			case "duplicate attribute":
				method.Attributes = append(method.Attributes, table)
			case "nil attribute":
				method.Attributes = append(method.Attributes, (*UnparsedAttribute)(nil))
			case "duplicate original pointer":
				object.Methods = append(object.Methods, method)
			case "duplicate original copy":
				copy := *method
				object.Methods = append(object.Methods, &copy)
			case "detached method":
				copy := *method
				method = &copy
			case "wrong method CP kind":
				object.ConstantPool[method.NameIndex-1] = &ConstantIntegerInfo{}
			case "wrong descriptor CP kind":
				object.ConstantPool[method.DescriptorIndex-1] = &ConstantIntegerInfo{}
			case "wrong descriptor request":
				descriptor = "(I)I"
			case "illegal major":
				object.MajorVersion = 0
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			names, known := d.originalOrdinaryMethodParameterNames(method, descriptor)
			valid := variant == "present empty" || variant == "absent"
			if known != valid || known && (len(names) != 0 || (names == nil) != (variant == "absent")) {
				t.Fatalf("typed empty names=%#v known=%v", names, known)
			}
		})
	}
}

func TestAdversarialEmptyMethodParametersSuccessCacheRequiresFreshTable(t *testing.T) {
	files, dir := compileEmptyMethodParameterFixture(t, "none")
	runEmptyMethodParameterDriver(t, dir, "original absent")
	for _, variant := range []string{"original", "removed table", "count mutation", "duplicate attribute", "duplicate method", "Code mutation"} {
		t.Run(variant, func(t *testing.T) {
			object, method, table := authoredEmptyMethodParameterTable(t, files["EmptyParameterOwner.class"], "ping")
			d := methodParameterBoundaryDumper(t, object.Bytes())
			for _, m := range d.obj.Methods {
				name, _ := sourceBridgeUTF8(d.obj, m.NameIndex)
				if name == "ping" {
					method = m
					for _, a := range m.Attributes {
						if p, ok := a.(*UnparsedAttribute); ok && p.Name == "MethodParameters" {
							table = p
						}
					}
				}
			}
			packet, err := d.DumpMethod("ping", "()I")
			if err != nil || packet == nil || packet.parameterNameStamp == nil {
				t.Fatalf("empty-table success did not seal normal cache: %v", err)
			}
			switch variant {
			case "removed table":
				method.Attributes = method.Attributes[:len(method.Attributes)-1]
			case "count mutation":
				table.Info[0] = 1
			case "duplicate attribute":
				method.Attributes = append(method.Attributes, table)
			case "duplicate method":
				d.obj.Methods = append(d.obj.Methods, method)
			case "Code mutation":
				for _, a := range method.Attributes {
					if code, ok := a.(*CodeAttribute); ok {
						code.Code[1] = 8
					}
				}
			}
			next, err := d.DumpMethod("ping", "()I")
			if variant == "original" {
				if err != nil || next != packet {
					t.Fatal("fresh empty table cache missed")
				}
				return
			}
			if err == nil || next != nil || d.dumpedMethodsSet["name:ping,desc:()I"] != packet {
				t.Fatal("stale empty-table cache bypassed fresh guard or replaced sealed packet")
			}
		})
	}
}

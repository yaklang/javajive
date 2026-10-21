package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

const sourceParameterMetadataFixture = `class ParameterMetadataOwner<T> {
 private final Object token;
 ParameterMetadataOwner(final Object token){this.token=token;}
 public <R> R choose(final long left,R value,double right){return value;}
 public static long mix(long first,final int second,double third){return first+second+(long)third;}
 public String literal(){return "left";}
 public Object array(String... words){return words;}
 public void empty(){}
}`

func sourceParameterTestMethod(t *testing.T, object *ClassObject, name string) *MemberInfo {
	t.Helper()
	var found *MemberInfo
	for _, method := range object.Methods {
		n, _ := sourceBridgeUTF8(object, method.NameIndex)
		if n == name {
			if found != nil {
				t.Fatal("ambiguous fixture method", name)
			}
			found = method
		}
	}
	if found == nil {
		t.Fatal("fixture method missing", name)
	}
	return found
}

func sourceParameterCompiledUnchanged(t *testing.T, compiled, output []byte) {
	t.Helper()
	a, ea := Parse(compiled)
	b, eb := Parse(output)
	if ea != nil || eb != nil || len(a.Methods) != len(b.Methods) || len(b.ConstantPool) < len(a.ConstantPool) || !reflect.DeepEqual(a.ConstantPool, b.ConstantPool[:len(a.ConstantPool)]) || !reflect.DeepEqual(a.Fields, b.Fields) || !reflect.DeepEqual(a.Attributes, b.Attributes) {
		t.Fatal("compiled constant pool, fields or class metadata changed", ea, eb)
	}
	for i, method := range a.Methods {
		other := b.Methods[i]
		keep := func(attributes []AttributeInfo) []AttributeInfo {
			var result []AttributeInfo
			for _, attribute := range attributes {
				if table, ok := attribute.(*UnparsedAttribute); ok && table.Name == "MethodParameters" {
					continue
				}
				result = append(result, attribute)
			}
			return result
		}
		if method.NameIndex != other.NameIndex || method.DescriptorIndex != other.DescriptorIndex || method.AccessFlags != other.AccessFlags || !reflect.DeepEqual(keep(method.Attributes), keep(other.Attributes)) {
			t.Fatal("compiled declaration, Code, annotations, signature or debug bytes changed")
		}
	}
	artifactLambdaMetadataCodeUnchanged(t, compiled, output)
}

func TestNativeSourceParameterMetadataRestoresEveryOriginalRowAndAbsence(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Skip("JAVA8_JAVAC required")
	}
	for _, debug := range []string{"none", "source,lines,vars"} {
		originalNamed := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, sourceParameterMetadataFixture, "ParameterMetadataOwner", debug, true)["ParameterMetadataOwner.class"]
		originalAbsent := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, sourceParameterMetadataFixture, "ParameterMetadataOwner", debug, false)["ParameterMetadataOwner.class"]
		compiledNamed := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, sourceParameterMetadataFixture, "ParameterMetadataOwner", debug, true)["ParameterMetadataOwner.class"]
		compiledAbsent := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, sourceParameterMetadataFixture, "ParameterMetadataOwner", debug, false)["ParameterMetadataOwner.class"]
		for _, pair := range []struct {
			name               string
			original, compiled []byte
		}{{"add original rows", originalNamed, compiledAbsent}, {"restore absence", originalAbsent, compiledNamed}, {"retain named rows", originalNamed, compiledNamed}, {"retain absence", originalAbsent, compiledAbsent}} {
			t.Run(debug+"/"+pair.name, func(t *testing.T) {
				a, b := bytes.Clone(pair.original), bytes.Clone(pair.compiled)
				output, err := RestoreMethodParameterMetadata(pair.original, pair.compiled, nil)
				if err != nil || lambdaParameterMetadataTables(t, output) != lambdaParameterMetadataTables(t, pair.original) {
					t.Fatal("original descriptor-position parameter rows changed", err)
				}
				sourceParameterCompiledUnchanged(t, pair.compiled, output)
				nativeLexicalExactSignatures(t, "ParameterMetadataOwner", pair.original, output)
				second, err := RestoreMethodParameterMetadata(pair.original, output, nil)
				if err != nil || !bytes.Equal(second, output) || !bytes.Equal(a, pair.original) || !bytes.Equal(b, pair.compiled) {
					t.Fatal("metadata edit mutated input or is not idempotent", err)
				}
			})
		}
	}
}

func TestNativeSourceParameterMetadataKeepsLosslessNamesAndReflection(t *testing.T) {
	modern, java := t04Tools(t)
	driver := `class ParameterMetadataDriver {
 public static void main(String[]args)throws Exception {
  java.lang.reflect.Method method=ParameterMetadataOwner.class.getDeclaredMethod("choose",long.class,Object.class,double.class);
  java.lang.reflect.Parameter[] parameters=method.getParameters();
  char[][] expected={new char[]{'x',0xD800},new char[]{'x','\\','u','D','8','0','0'},new char[]{0x03C0,0xD83D,0xDE00}};
  int[] flags={0x10,0x1000,0x8000};
  if(parameters.length!=3)throw new AssertionError("descriptor rows, not wide local words");
  for(int i=0;i<3;i++)if(!parameters[i].isNamePresent()||!java.util.Arrays.equals(expected[i],parameters[i].getName().toCharArray())||parameters[i].getModifiers()!=flags[i]||parameters[i].isSynthetic()!=(i==1)||parameters[i].isImplicit()!=(i==2))throw new AssertionError("lossless name or flag at "+i);
  Object token=new Object();ParameterMetadataOwner owner=new ParameterMetadataOwner(token);
  if(owner.choose(Long.MIN_VALUE,token,Double.longBitsToDouble(0x7ff8000000000017L))!=token||!owner.literal().equals("left"))throw new AssertionError("code or shared application constant changed");
  System.out.println("parameters:3:wide:lossless:flags:alias");
 }
}`
	files := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, sourceParameterMetadataFixture+driver, "ParameterMetadataOwner", "none", false)
	compiled := files["ParameterMetadataOwner.class"]
	object, err := Parse(compiled)
	if err != nil {
		t.Fatal(err)
	}
	method := sourceParameterTestMethod(t, object, "choose")
	names := [][]uint16{{'x', 0xD800}, {'x', '\\', 'u', 'D', '8', '0', '0'}, {0x03C0, 0xD83D, 0xDE00}}
	flags := []uint16{0x10, 0x1000, 0x8000}
	info := make([]byte, 13)
	info[0] = 3
	for i, units := range names {
		name := &ConstantUtf8Info{}
		name.SetUnits(units)
		object.ConstantPool = append(object.ConstantPool, name)
		binary.BigEndian.PutUint16(info[1+4*i:], uint16(len(object.ConstantPool)))
		binary.BigEndian.PutUint16(info[3+4*i:], flags[i])
	}
	object.ConstantPoolManager.AddUtf8Info("MethodParameters")
	method.Attributes = append(method.Attributes, &UnparsedAttribute{Name: "MethodParameters", Length: uint32(len(info)), Info: info})
	original := object.Bytes()
	output, err := RestoreMethodParameterMetadata(original, compiled, nil)
	if err != nil {
		t.Fatal(err)
	}
	sourceParameterCompiledUnchanged(t, compiled, output)
	after, err := Parse(output)
	if err != nil {
		t.Fatal(err)
	}
	afterMethod := sourceParameterTestMethod(t, after, "choose")
	var table *UnparsedAttribute
	for _, attribute := range afterMethod.Attributes {
		if value, ok := attribute.(*UnparsedAttribute); ok && value.Name == "MethodParameters" {
			table = value
		}
	}
	if table == nil || len(table.Info) != 13 {
		t.Fatal("restored logical rows missing")
	}
	for i, expected := range names {
		index := binary.BigEndian.Uint16(table.Info[1+4*i:])
		value := after.ConstantPool[index-1].(*ConstantUtf8Info)
		if !reflect.DeepEqual(value.CodeUnits(), expected) || binary.BigEndian.Uint16(table.Info[3+4*i:]) != flags[i] {
			t.Fatal("parameter UTF16 payload or flags changed", i)
		}
	}
	if unitsToDisplay(names[0]) != unitsToDisplay(names[1]) {
		t.Fatal("lossy display collision control is not active")
	}
	for label, subject := range map[string][]byte{"authored original": original, "restored compiled": output} {
		t.Run(label, func(t *testing.T) {
			root := sourceArtifactWriteFiles(t, map[string][]byte{"ParameterMetadataOwner.class": subject, "ParameterMetadataDriver.class": files["ParameterMetadataDriver.class"]})
			if got := t04RunJava(t, java, root, "ParameterMetadataDriver"); got != "parameters:3:wide:lossless:flags:alias\n" {
				t.Fatal("independent reflection/behavior oracle", got)
			}
		})
	}
}

func TestNativeSourceParameterMetadataRejectsInvalidIdentityAndTables(t *testing.T) {
	modern, _ := t04Tools(t)
	original := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, sourceParameterMetadataFixture, "ParameterMetadataOwner", "source,lines,vars", true)["ParameterMetadataOwner.class"]
	compiled := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, sourceParameterMetadataFixture, "ParameterMetadataOwner", "source,lines,vars", false)["ParameterMetadataOwner.class"]
	fixtureOriginal, fixtureCompiled := bytes.Clone(original), bytes.Clone(compiled)
	defer func() {
		if !bytes.Equal(original, fixtureOriginal) || !bytes.Equal(compiled, fixtureCompiled) {
			t.Fatal("one negative variant poisoned another variant's fixture")
		}
	}()
	for _, variant := range []string{"control", "truncated original", "truncated compiled", "wrong class flags", "wrong method flags", "missing method", "duplicate method", "changed method name", "changed descriptor", "wrong original table count", "unknown original flags", "invalid original name index", "non UTF8 original name", "invalid unqualified name", "duplicate original table", "malformed compiled table", "budget", "canceled", "late output budget", "memory budget"} {
		t.Run(variant, func(t *testing.T) {
			before, _ := Parse(bytes.Clone(original))
			after, _ := Parse(bytes.Clone(compiled))
			method := sourceParameterTestMethod(t, before, "choose")
			var table *UnparsedAttribute
			for _, attribute := range method.Attributes {
				if value, ok := attribute.(*UnparsedAttribute); ok && value.Name == "MethodParameters" {
					table = value
				}
			}
			if table == nil {
				t.Fatal("original table required")
			}
			inputOriginal, inputCompiled := original, compiled
			changedOriginal, changedCompiled := false, false
			var work *workbudget.Budget
			switch variant {
			case "truncated original":
				inputOriginal = original[:len(original)-1]
			case "truncated compiled":
				inputCompiled = compiled[:len(compiled)-1]
			case "wrong class flags":
				after.AccessFlags ^= 0x10
				changedCompiled = true
			case "wrong method flags":
				sourceParameterTestMethod(t, after, "choose").AccessFlags ^= 8
				changedCompiled = true
			case "missing method":
				after.Methods = after.Methods[:len(after.Methods)-1]
				changedCompiled = true
			case "duplicate method":
				after.Methods[1] = after.Methods[0]
				changedCompiled = true
			case "changed method name":
				after.ConstantPool = append(after.ConstantPool, NewUtf8FromString("other"))
				sourceParameterTestMethod(t, after, "choose").NameIndex = uint16(len(after.ConstantPool))
				changedCompiled = true
			case "changed descriptor":
				after.ConstantPool = append(after.ConstantPool, NewUtf8FromString("(JLjava/lang/Object;F)Ljava/lang/Object;"))
				sourceParameterTestMethod(t, after, "choose").DescriptorIndex = uint16(len(after.ConstantPool))
				changedCompiled = true
			case "wrong original table count":
				table.Info[0]--
				changedOriginal = true
			case "unknown original flags":
				binary.BigEndian.PutUint16(table.Info[3:], 1)
				changedOriginal = true
			case "invalid original name index":
				binary.BigEndian.PutUint16(table.Info[1:], 65535)
				changedOriginal = true
			case "non UTF8 original name":
				binary.BigEndian.PutUint16(table.Info[1:], before.ThisClass)
				changedOriginal = true
			case "invalid unqualified name":
				before.ConstantPool = append(before.ConstantPool, NewUtf8FromString("not/a/name"))
				binary.BigEndian.PutUint16(table.Info[1:], uint16(len(before.ConstantPool)))
				changedOriginal = true
			case "duplicate original table":
				method.Attributes = append(method.Attributes, table)
				changedOriginal = true
			case "malformed compiled table":
				after.ConstantPoolManager.AddUtf8Info("MethodParameters")
				sourceParameterTestMethod(t, after, "choose").Attributes = append(sourceParameterTestMethod(t, after, "choose").Attributes, &UnparsedAttribute{Name: "MethodParameters", Length: 1, Info: []byte{0}})
				changedCompiled = true
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			case "late output budget":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: int64(len(compiled)) + 1})
			case "memory budget":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			if changedOriginal {
				inputOriginal = before.Bytes()
			}
			if changedCompiled {
				inputCompiled = after.Bytes()
			}
			a, b := bytes.Clone(inputOriginal), bytes.Clone(inputCompiled)
			output, err := RestoreMethodParameterMetadata(inputOriginal, inputCompiled, work)
			if variant == "control" {
				if err != nil || lambdaParameterMetadataTables(t, output) != lambdaParameterMetadataTables(t, inputOriginal) {
					t.Fatal("control failed", err)
				}
				sourceParameterCompiledUnchanged(t, inputCompiled, output)
			} else if err == nil || output != nil {
				t.Fatal("invalid method metadata accepted")
			}
			if work != nil {
				var failure *workbudget.Error
				if !errors.As(err, &failure) {
					t.Fatal("resource failure hidden", err)
				}
				if variant == "late output budget" && failure.Counter != workbudget.CounterOutputBytes {
					t.Fatal("late output control failed before the output boundary", err)
				}
			}
			if !bytes.Equal(a, inputOriginal) || !bytes.Equal(b, inputCompiled) {
				t.Fatal("input mutated")
			}
		})
	}
}

func TestNativeSourceParameterMetadataRejectsLossyIdentityCollisions(t *testing.T) {
	modern, _ := t04Tools(t)
	raw := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, sourceParameterMetadataFixture, "ParameterMetadataOwner", "none", false)["ParameterMetadataOwner.class"]
	for _, kind := range []string{"class", "method"} {
		t.Run(kind, func(t *testing.T) {
			a, _ := Parse(bytes.Clone(raw))
			b, _ := Parse(bytes.Clone(raw))
			var ai, bi uint16
			if kind == "class" {
				ai = a.ConstantPool[a.ThisClass-1].(*ConstantClassInfo).NameIndex
				bi = b.ConstantPool[b.ThisClass-1].(*ConstantClassInfo).NameIndex
			} else {
				ai = sourceParameterTestMethod(t, a, "choose").NameIndex
				bi = sourceParameterTestMethod(t, b, "choose").NameIndex
			}
			aName, bName := a.ConstantPool[ai-1].(*ConstantUtf8Info), b.ConstantPool[bi-1].(*ConstantUtf8Info)
			aName.SetUnits([]uint16{'x', 0xD800})
			bName.SetString(`x\uD800`)
			if aName.Value != bName.Value || reflect.DeepEqual(aName.CodeUnits(), bName.CodeUnits()) {
				t.Fatal("lossy identity collision control inactive")
			}
			original, compiled := a.Bytes(), b.Bytes()
			if _, err := Parse(original); err != nil {
				t.Fatal("authored original identity must remain parseable", err)
			}
			if _, err := Parse(compiled); err != nil {
				t.Fatal("compiled identity must remain parseable", err)
			}
			if output, err := RestoreMethodParameterMetadata(original, compiled, nil); err == nil || output != nil {
				t.Fatal("display spelling substituted for a different JVM identity")
			}
		})
	}
}

func TestNativeSourceParameterMetadataKeepsEmptyTableDistinctFromAbsence(t *testing.T) {
	modern, _ := t04Tools(t)
	absent := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, sourceParameterMetadataFixture, "ParameterMetadataOwner", "none", false)["ParameterMetadataOwner.class"]
	object, err := Parse(bytes.Clone(absent))
	if err != nil {
		t.Fatal(err)
	}
	object.ConstantPoolManager.AddUtf8Info("MethodParameters")
	method := sourceParameterTestMethod(t, object, "empty")
	method.Attributes = append(method.Attributes, &UnparsedAttribute{Name: "MethodParameters", Length: 1, Info: []byte{0}})
	present := object.Bytes()
	for _, pair := range []struct {
		name               string
		original, compiled []byte
	}{{"add empty", present, absent}, {"remove empty", absent, present}} {
		t.Run(pair.name, func(t *testing.T) {
			output, err := RestoreMethodParameterMetadata(pair.original, pair.compiled, nil)
			if err != nil || lambdaParameterMetadataTables(t, output) != lambdaParameterMetadataTables(t, pair.original) {
				t.Fatal("empty and absent tables collapsed", err)
			}
			sourceParameterCompiledUnchanged(t, pair.compiled, output)
		})
	}
}

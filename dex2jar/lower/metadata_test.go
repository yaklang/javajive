package lower

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar/model"
)

// metadataFixture builds a class that exercises SourceFile, ConstantValue,
// debug tables, user annotations, generic signatures and declared throws.
func metadataFixture() (*model.File, *model.Class) {
	const owner = "Ldex/Metadata;"
	protos := []model.Proto{
		{ReturnType: "V"},
		{ReturnType: "I", Parameters: []string{"I"}, ParameterTypes: []uint32{0}},
		{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{1}},
	}
	methods := []model.MethodID{
		{Class: owner, Name: "twice", ProtoIndex: 1},
		{Class: owner, Name: "main", ProtoIndex: 2},
	}
	answer := model.FieldID{Class: owner, Name: "ANSWER", Type: "I"}
	text := model.FieldID{Class: owner, Name: "TEXT", Type: "Ljava/lang/String;"}
	file := &model.File{
		Entry:   "classes.dex",
		Strings: []string{"hi", "value"},
		Types:   []model.Type{{Descriptor: "I"}, {Descriptor: "[Ljava/lang/String;"}, {Descriptor: "Ldex/Metadata;"}},
		Protos:  protos,
		Fields:  []model.FieldID{answer, text},
		Methods: methods,
	}
	class := &model.Class{
		Descriptor:   owner,
		Superclass:   "Ljava/lang/Object;",
		AccessFlags:  dexPublic,
		SourceFile:   "Metadata.java",
		Signature:    "<T:Ljava/lang/Object;>Ljava/lang/Object;",
		StaticFields: []model.EncodedField{{FieldIndex: 0, AccessFlags: dexPublic | dexStatic | dexFinal, Field: answer}, {FieldIndex: 1, AccessFlags: dexPublic | dexStatic | dexFinal, Field: text}},
		StaticValues: []model.EncodedValue{{Type: model.ValueInt, Signed: 42}, {Type: model.ValueString, Unsigned: 0}},
		Annotations: []model.Annotation{{
			Visibility: model.AnnotationRuntime,
			Type:       "Ldex/Marker;",
			Elements:   []model.AnnotationElement{{Name: "value", Value: model.EncodedValue{Type: model.ValueString, Unsigned: 1}}},
		}},
		DirectMethods: []model.EncodedMethod{
			{
				MethodIndex: 0,
				AccessFlags: dexPublic | dexStatic,
				Method:      methods[0],
				Code: &model.Code{
					RegistersSize: 2, InsSize: 1,
					// const/4 v0, #2; mul-int v0, v1, v0; return v0
					Instructions: []uint16{0x2012, 0x0092, 0x0001, 0x000f},
					Debug: &model.DebugInfo{
						LineStart:      7,
						ParameterNames: []string{"value"},
						Positions: []model.DebugPosition{
							{Address: 0, Line: 7},
							{Address: 1, Line: 8},
						},
						Locals: []model.DebugLocal{{Register: 1, Name: "value", Type: "I", Start: 0, End: 4}},
					},
				},
			},
			{
				MethodIndex: 1,
				AccessFlags: dexPublic | dexStatic,
				Method:      methods[1],
				Code: &model.Code{
					RegistersSize: 1, InsSize: 1,
					Instructions: []uint16{0x000e},
				},
			},
		},
		MethodAnnotations: []model.MethodAnnotation{{
			MethodIndex: 0,
			Annotations: []model.Annotation{{
				Visibility: model.AnnotationRuntime,
				Type:       "Ldex/Checked;",
			}, {
				Visibility: model.AnnotationSystem,
				Type:       "Ldalvik/annotation/Throws;",
				Elements: []model.AnnotationElement{{
					Name: "value", Value: model.EncodedValue{Type: model.ValueArray, Array: []model.EncodedValue{{Type: model.ValueTypeRef, Unsigned: 3}}},
				}},
			}},
		}},
	}
	return file, class
}

func TestLowerEmitsMetadataAttributes(t *testing.T) {
	file, class := metadataFixture()
	file.Types = append(file.Types, model.Type{Descriptor: "Ljava/lang/Exception;"})
	raw, err := Class(file, class)
	if err != nil {
		t.Fatalf("Class: %v", err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatalf("parse generated class: %v", err)
	}

	var source *javaclassparser.SourceFileAttribute
	var signature *javaclassparser.SignatureAttribute
	for _, attribute := range parsed.Attributes {
		switch value := attribute.(type) {
		case *javaclassparser.SourceFileAttribute:
			source = value
		case *javaclassparser.SignatureAttribute:
			signature = value
		case *javaclassparser.RuntimeVisibleAnnotationsAttribute:
			if len(value.Annotations) != 1 || value.Annotations[0].TypeName != "Ldex/Marker;" {
				t.Fatalf("class annotations = %#v", value.Annotations)
			}
		}
	}
	if source == nil || signature == nil {
		t.Fatalf("class attributes missing SourceFile/Signature: %#v", parsed.Attributes)
	}
	if got := parsed.ConstantPoolManager.GetUtf8(int(source.SourceFileIndex)); got == nil || got.Value != "Metadata.java" {
		t.Fatalf("SourceFile = %#v", got)
	}

	for _, field := range parsed.Fields {
		if len(field.Attributes) != 1 {
			t.Fatalf("field attributes = %#v", field.Attributes)
		}
		constant, ok := field.Attributes[0].(*javaclassparser.ConstantValueAttribute)
		if !ok {
			t.Fatalf("field attribute = %#v", field.Attributes[0])
		}
		if constant.ConstantValueIndex == 0 {
			t.Fatal("ConstantValue index is zero")
		}
	}

	var twice *javaclassparser.MemberInfo
	for _, method := range parsed.Methods {
		if parsed.ConstantPoolManager.GetUtf8(int(method.NameIndex)).Value == "twice" {
			twice = method
		}
	}
	if twice == nil {
		t.Fatal("twice method missing")
	}
	var code *javaclassparser.CodeAttribute
	var exceptions bool
	var methodSignature bool
	for _, attribute := range twice.Attributes {
		switch value := attribute.(type) {
		case *javaclassparser.CodeAttribute:
			code = value
		case *javaclassparser.ExceptionsAttribute:
			exceptions = true
		case *javaclassparser.SignatureAttribute:
			methodSignature = true
		}
	}
	if code == nil || !exceptions {
		t.Fatalf("method attributes = %#v", twice.Attributes)
	}
	_ = methodSignature

	var lines *javaclassparser.LineNumberTableAttribute
	localTable := false
	for _, attribute := range code.Attributes {
		switch value := attribute.(type) {
		case *javaclassparser.LineNumberTableAttribute:
			lines = value
		case *javaclassparser.UnparsedAttribute:
			if value.Name == "LocalVariableTable" {
				localTable = true
			}
		}
	}
	if lines == nil || len(lines.LineNumberTable) != 2 {
		t.Fatalf("LineNumberTable = %#v", lines)
	}
	// JVM pcs are shifted by the parameter-prologue bytes; the contract is that
	// both original lines survive in order at strictly increasing positions.
	if lines.LineNumberTable[0].LineNumber != 7 || lines.LineNumberTable[1].LineNumber != 8 {
		t.Fatalf("LineNumberTable lines = %d, %d", lines.LineNumberTable[0].LineNumber, lines.LineNumberTable[1].LineNumber)
	}
	if lines.LineNumberTable[0].StartPc >= lines.LineNumberTable[1].StartPc {
		t.Fatalf("LineNumberTable pcs are not increasing: %d, %d", lines.LineNumberTable[0].StartPc, lines.LineNumberTable[1].StartPc)
	}
	if !localTable {
		t.Fatal("LocalVariableTable was not emitted")
	}

	verifyWithJVM(t, raw, "dex.Metadata")
}

// arrayOpsFixture builds byte[] literal construction plus narrow element access.
func arrayOpsFixture() (*model.File, *model.Class) {
	const owner = "Ldex/ArrayOps;"
	protos := []model.Proto{
		{ReturnType: "[B", Parameters: []string{"I"}, ParameterTypes: []uint32{0}},
		{ReturnType: "I", Parameters: []string{"[B", "I"}, ParameterTypes: []uint32{1, 0}},
		{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{2}},
	}
	methods := []model.MethodID{
		{Class: owner, Name: "make", ProtoIndex: 0},
		{Class: owner, Name: "first", ProtoIndex: 1},
		{Class: owner, Name: "main", ProtoIndex: 2},
	}
	file := &model.File{
		Entry:   "classes.dex",
		Types:   []model.Type{{Descriptor: "I"}, {Descriptor: "[B"}, {Descriptor: "[Ljava/lang/String;"}},
		Protos:  protos,
		Methods: methods,
	}
	class := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      methods[0],
				Code: &model.Code{RegistersSize: 2, InsSize: 1, OutsSize: 1, Instructions: []uint16{
					0x1024, 0x0001, 0x0001, // filled-new-array {v1}, [B
					0x000c, // move-result-object v0
					0x0011, // return-object v0
				}},
			},
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      methods[1],
				Code: &model.Code{RegistersSize: 3, InsSize: 2, Instructions: []uint16{
					0x0048, 0x0201, // aget-byte v0, v1, v2
					0x000f, // return v0
				}},
			},
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      methods[2],
				Code:        &model.Code{RegistersSize: 1, InsSize: 1, Instructions: []uint16{0x000e}},
			},
		},
	}
	return file, class
}

func TestLowerFilledNewArrayAndNarrowArrayElements(t *testing.T) {
	file, class := arrayOpsFixture()
	raw, err := Class(file, class)
	if err != nil {
		t.Fatalf("Class: %v", err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatalf("parse generated class: %v", err)
	}
	makeCode := methodCode(t, parsed, "make")
	if !bytes.Contains(makeCode, []byte{0xbc, 0x08}) {
		t.Fatalf("byte[] construction does not use NEWARRAY 8: % x", makeCode)
	}
	if !bytes.Contains(makeCode, []byte{0x54}) {
		t.Fatalf("filled array does not store byte elements: % x", makeCode)
	}
	firstCode := methodCode(t, parsed, "first")
	if !bytes.Contains(firstCode, []byte{0x33}) {
		t.Fatalf("aget-byte does not lower to BALOAD: % x", firstCode)
	}
	verifyWithJVM(t, raw, "dex.ArrayOps")
}

func methodCode(t *testing.T, parsed *javaclassparser.ClassObject, name string) []byte {
	t.Helper()
	for _, method := range parsed.Methods {
		if parsed.ConstantPoolManager.GetUtf8(int(method.NameIndex)).Value != name {
			continue
		}
		for _, attribute := range method.Attributes {
			if code, ok := attribute.(*javaclassparser.CodeAttribute); ok {
				return code.Code
			}
		}
	}
	t.Fatalf("method %q has no Code attribute", name)
	return nil
}

func verifyWithJVM(t *testing.T, raw []byte, className string) {
	t.Helper()
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java is unavailable")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, strings.ReplaceAll(className, ".", "/")+".class")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(java, "-Xverify:all", "-cp", dir, className).CombinedOutput(); err != nil {
		t.Fatalf("java verification failed: %v\n%s", err, output)
	}
}

// classMetadataFixture covers class-level system/user annotations, inner and
// enclosing metadata, parameter annotations, and an array-owner invocation.
func classMetadataFixture() (*model.File, *model.Class) {
	const owner = "Ldex/Outer$Inner;"
	protos := []model.Proto{
		{ReturnType: "Ljava/lang/Object;", Parameters: []string{"[I"}, ParameterTypes: []uint32{1}},
		{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{2}},
		{ReturnType: "Ljava/lang/Object;"},
	}
	methods := []model.MethodID{
		{Class: owner, Name: "cloneIt", ProtoIndex: 0},
		{Class: owner, Name: "main", ProtoIndex: 1},
		{Class: "[I", Name: "clone", ProtoIndex: 2},
	}
	file := &model.File{
		Entry:   "classes.dex",
		Strings: []string{"<T:Ljava/lang/Object;>Ljava/lang/Object;", "dex/Marker;", "value"},
		Types:   []model.Type{{Descriptor: "I"}, {Descriptor: "[I"}, {Descriptor: "[Ljava/lang/String;"}},
		Protos:  protos,
		Methods: methods,
	}
	class := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		Annotations: []model.Annotation{
			{
				Visibility: model.AnnotationSystem,
				Type:       "Ldalvik/annotation/Signature;",
				Elements: []model.AnnotationElement{{
					Name: "value", Value: model.EncodedValue{Type: model.ValueArray, Array: []model.EncodedValue{
						{Type: model.ValueString, Unsigned: 0},
					}},
				}},
			},
			{Visibility: model.AnnotationBuild, Type: "Ldex/CompileOnly;"},
		},
		InnerClasses: []model.InnerClass{{Inner: owner, Outer: "Ldex/Outer;", Name: "Inner", Flags: uint16(dexPublic)}},
		Enclosing:    &model.EnclosingMethod{Class: "Ldex/Outer;", Method: "make", Proto: "()Ldex/Outer$Inner;"},
		DirectMethods: []model.EncodedMethod{
			{
				MethodIndex: 0,
				AccessFlags: dexPublic | dexStatic,
				Method:      methods[0],
				Code: &model.Code{RegistersSize: 1, InsSize: 1, OutsSize: 1, Instructions: []uint16{
					0x106e, 0x0002, 0x0000, // invoke-virtual {v0}, [I.clone()
					0x000c, // move-result-object v0
					0x0011, // return-object v0
				}},
			},
			{
				MethodIndex: 1,
				AccessFlags: dexPublic | dexStatic,
				Method:      methods[1],
				Code:        &model.Code{RegistersSize: 1, InsSize: 1, Instructions: []uint16{0x000e}},
			},
		},
		MethodAnnotations: []model.MethodAnnotation{{
			MethodIndex: 0,
			Annotations: []model.Annotation{{
				Visibility: model.AnnotationBuild,
				Type:       "Ldex/CompileOnly;",
				Elements: []model.AnnotationElement{{
					Name: "value", Value: model.EncodedValue{Type: model.ValueInt, Signed: 7},
				}},
			}},
		}},
		ParameterAnnotations: []model.ParameterAnnotation{{
			MethodIndex: 0,
			Parameters: [][]model.Annotation{{{
				Visibility: model.AnnotationRuntime,
				Type:       "Ldex/Marker;",
			}}},
		}},
	}
	return file, class
}

func TestLowerEmitsClassMetadataAndArrayOwnerInvoke(t *testing.T) {
	file, class := classMetadataFixture()
	raw, err := Class(file, class)
	if err != nil {
		t.Fatalf("Class: %v", err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatalf("parse generated class: %v", err)
	}
	var signature bool
	var inner, enclosing, invisible, visible bool
	for _, attribute := range parsed.Attributes {
		switch value := attribute.(type) {
		case *javaclassparser.SignatureAttribute:
			signature = true
		case *javaclassparser.InnerClassesAttribute:
			inner = true
		case *javaclassparser.UnparsedAttribute:
			if value.Name == "EnclosingMethod" {
				enclosing = true
			}
		case *javaclassparser.RuntimeVisibleAnnotationsAttribute:
			if value.IsInvisible {
				invisible = true
			} else {
				visible = true
			}
		}
	}
	if !signature || !inner || !enclosing || !invisible || visible {
		t.Fatalf("class metadata = signature=%v inner=%v enclosing=%v invisible=%v visible=%v",
			signature, inner, enclosing, invisible, visible)
	}
	var clone *javaclassparser.MemberInfo
	for _, method := range parsed.Methods {
		if parsed.ConstantPoolManager.GetUtf8(int(method.NameIndex)).Value == "cloneIt" {
			clone = method
		}
	}
	if clone == nil {
		t.Fatal("cloneIt method missing")
	}
	verifyWithJVM(t, raw, "dex.Outer$Inner")
	_ = clone
}

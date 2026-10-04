package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const reviewedByteBuddyJar = "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar"

// Validate original descriptors and cache def-use relationships, not incidental
// generated slot numbers or whether a legacy text repair still changes output.
func assertReviewedCachedJarField(t *testing.T, entry, field, descriptor, method, sourceType string, checkCalls bool) {
	t.Helper()
	raw := originalJarClassForReview(t, reviewedByteBuddyJar, entry)
	object, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	foundField := false
	for _, f := range object.Fields {
		n, _ := object.getUtf8(f.NameIndex)
		d, _ := object.getUtf8(f.DescriptorIndex)
		if n == field {
			if d != descriptor {
				t.Fatalf("original field %s descriptor %s, want %s", field, d, descriptor)
			}
			foundField = true
		}
	}
	if !foundField {
		t.Fatalf("original cache field %s missing", field)
	}
	var code *CodeAttribute
	for _, m := range object.Methods {
		n, _ := object.getUtf8(m.NameIndex)
		d, _ := object.getUtf8(m.DescriptorIndex)
		if n == method && d == "()"+descriptor {
			for _, attr := range m.Attributes {
				if c, ok := attr.(*CodeAttribute); ok {
					code = c
				}
			}
		}
	}
	if code == nil {
		t.Fatalf("original return descriptor %s()%s missing", method, descriptor)
	}
	if checkCalls {
		// load declares CNFE in the original interface. Inserting a new reflective
		// call to make that catch compile adds behavior absent from the bytecode.
		assertReviewedLoadDeclaresCNFE(t)
		d := core.NewDecompiler(code.Code, nil)
		if err := d.ParseOpcode(); err != nil {
			t.Fatal(err)
		}
		cp := NewConstantPoolWithConstant(&object.ConstantPool)
		for _, op := range d.Opcodes() {
			if op.Instr.OpCode != core.OP_INVOKESTATIC {
				continue
			}
			index := int(op.Data[0])<<8 | int(op.Data[1])
			if member, ok := cp.IndexInfo(index).(*ConstantMethodrefInfo); ok {
				pair, ok := cp.IndexInfo(int(member.NameAndTypeIndex)).(*ConstantNameAndTypeInfo)
				if ok && cp.GetClassName(int(member.ClassIndex)) == "java/lang/Class" && cp.GetUtf8(int(pair.NameIndex)).Value == "forName" {
					t.Fatal("fixture gained Class.forName: review cache call contract")
				}
			}
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_HARDJAR_SHAPE_OFF", setting)
		fs, err := NewJarFSFromLocal(filepath.Join(home, ".m2/repository", reviewedByteBuddyJar))
		if err != nil {
			t.Fatal(err)
		}
		generated, readErr := fs.ReadFile(entry)
		fs.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		body := reviewedSourceMethod(t, string(generated), regexp.QuoteMeta(sourceType)+`\s+`+method+`\(\)`)
		write := requireReviewedPattern(t, body, `this\.`+field+`\s*=\s*(\w+)\s*;`)
		local := regexp.QuoteMeta(write[1])
		typ := regexp.QuoteMeta(sourceType)
		requireReviewedPattern(t, body, typ+`\s+`+local+`\s*=`)
		requireReviewedPattern(t, body, `return\s+`+local+`\s*;`)
		requireReviewedPattern(t, body, `if\s*\(\s*\(?`+local+`\)?\s*==\s*\(?null\)?\s*\)`)
		requireReviewedPattern(t, body, local+`\s*=\s*[^;\n]*this\.`+field+`[^;\n]*;`)
		requireReviewedPattern(t, body, `this\.`+field+`\)?\s*!=\s*\(?null`)
		if checkCalls && strings.Contains(body, "Class.forName(") {
			t.Fatalf("%s=%q: original checked load catch gained an extra Class.forName effect:\n%s", "JDEC_HARDJAR_SHAPE_OFF", setting, body)
		}
	}
}

func assertReviewedLoadDeclaresCNFE(t *testing.T) {
	t.Helper()
	raw := originalJarClassForReview(t, reviewedByteBuddyJar, "net/bytebuddy/description/type/TypeDescription$SuperTypeLoading$ClassLoadingDelegate.class")
	object, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	cp := NewConstantPoolWithConstant(&object.ConstantPool)
	for _, method := range object.Methods {
		name, _ := object.getUtf8(method.NameIndex)
		desc, _ := object.getUtf8(method.DescriptorIndex)
		if name != "load" || desc != "(Ljava/lang/String;Ljava/lang/ClassLoader;)Ljava/lang/Class;" {
			continue
		}
		for _, attr := range method.Attributes {
			if exceptions, ok := attr.(*ExceptionsAttribute); ok {
				for _, index := range exceptions.ExceptionIndexTable {
					if cp.GetClassName(int(index)) == "java/lang/ClassNotFoundException" {
						return
					}
				}
			}
		}
	}
	t.Fatal("original ClassLoadingDelegate.load no longer declares CNFE")
}

package javaclassparser

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

type reviewedViewMember struct {
	name, descriptor, signature string
	field                       bool
}
type reviewedViewInvoke struct {
	owner, name, descriptor string
	opcode                  int
}

func reviewedInvocationView(t *testing.T, jar, entry string, members []reviewedViewMember, invokes []reviewedViewInvoke, check func(string)) {
	t.Helper()
	raw := originalJarClassForReview(t, jar, entry)
	object, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	cp := NewConstantPoolWithConstant(&object.ConstantPool)
	for _, wanted := range members {
		found := false
		pool := object.Methods
		if wanted.field {
			pool = object.Fields
		}
		for _, member := range pool {
			name := cp.GetUtf8(int(member.NameIndex)).Value
			desc := cp.GetUtf8(int(member.DescriptorIndex)).Value
			if name != wanted.name || desc != wanted.descriptor {
				continue
			}
			sig := ""
			for _, attr := range member.Attributes {
				if signature, ok := attr.(*SignatureAttribute); ok {
					sig = cp.GetUtf8(int(signature.SignatureIndex)).Value
				}
			}
			if sig != wanted.signature {
				t.Fatalf("original %s%s Signature %q != %q", name, desc, sig, wanted.signature)
			}
			found = true
		}
		if !found {
			t.Fatalf("original declaration missing %+v", wanted)
		}
	}
	foundCalls := make([]bool, len(invokes))
	for _, method := range object.Methods {
		for _, attribute := range method.Attributes {
			code, ok := attribute.(*CodeAttribute)
			if !ok {
				continue
			}
			decoder := core.NewDecompiler(code.Code, nil)
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			for _, op := range decoder.Opcodes() {
				if len(op.Data) < 2 {
					continue
				}
				switch op.Instr.OpCode {
				case core.OP_INVOKEVIRTUAL, core.OP_INVOKESTATIC, core.OP_INVOKEINTERFACE, core.OP_INVOKESPECIAL, core.OP_GETFIELD, core.OP_GETSTATIC, core.OP_PUTFIELD, core.OP_PUTSTATIC:
				default:
					continue
				}
				index := int(op.Data[0])<<8 | int(op.Data[1])
				var member ConstantMemberrefInfo
				switch ref := cp.IndexInfo(index).(type) {
				case *ConstantMethodrefInfo:
					member = ref.ConstantMemberrefInfo
				case *ConstantInterfaceMethodrefInfo:
					member = ref.ConstantMemberrefInfo
				case *ConstantFieldrefInfo:
					member = ref.ConstantMemberrefInfo
				default:
					continue
				}
				pair, ok := cp.IndexInfo(int(member.NameAndTypeIndex)).(*ConstantNameAndTypeInfo)
				if !ok {
					continue
				}
				owner := cp.GetClassName(int(member.ClassIndex))
				name := cp.GetUtf8(int(pair.NameIndex)).Value
				desc := cp.GetUtf8(int(pair.DescriptorIndex)).Value
				for i, wanted := range invokes {
					if owner == wanted.owner && name == wanted.name && desc == wanted.descriptor && op.Instr.OpCode == wanted.opcode {
						foundCalls[i] = true
					}
				}
			}
		}
	}
	for i, wanted := range invokes {
		if !foundCalls[i] {
			t.Fatalf("original opcode/receiver/argument contract missing %+v", wanted)
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_HARDJAR_SHAPE_OFF", setting)
		fs, err := NewJarFSFromLocal(filepath.Join(home, ".m2/repository", jar))
		if err != nil {
			t.Fatal(err)
		}
		source, err := fs.ReadFile(entry)
		fs.Close()
		if err != nil {
			t.Fatal(err)
		}
		check(string(source))
	}
}

func requireReviewedViewLocal(t *testing.T, source, typ, initializer string) string {
	t.Helper()
	binding := requireReviewedPattern(t, source, regexp.QuoteMeta(typ)+`\s+(\w+)\s*=\s*`+initializer)
	return regexp.QuoteMeta(binding[1])
}

func assertReviewedInterfaceListInitializer(t *testing.T, source, field string) {
	t.Helper()
	inline := regexp.MustCompile(regexp.QuoteMeta(field) + `\s*=\s*[^;\n]*Collections\.unmodifiableList\([^;\n]*Arrays\.asList\(`)
	if inline.MatchString(source) {
		return
	}
	// A closed original <clinit> segment can live in a static helper: bind the
	// actual field initializer to its helper return instead of prescribing syntax.
	initializer := requireReviewedPattern(t, source, regexp.QuoteMeta(field)+`\s*=\s*([\w$]+)\(\)\s*;`)
	declaration := regexp.MustCompile(`static\s+List(?:<[^;\n]+>)?\s+` + regexp.QuoteMeta(initializer[1]) + `\(\)`).FindStringIndex(source)
	if declaration == nil {
		t.Fatal("field initializer helper missing")
	}
	open := javaIndexBraceFrom(source, declaration[1])
	if open < 0 {
		t.Fatal("helper body missing")
	}
	end := javaMatchBrace(source, open)
	if end < 0 {
		t.Fatal("helper body is unclosed")
	}
	body := source[open : end+1]
	requireReviewedPattern(t, body, `return\s+[^;\n]*Collections\.unmodifiableList\([^;\n]*Arrays\.asList\(`)
	if regexp.MustCompile(`return\s+null\s*;|unproven interface initializer|yak-decompiler:`).MatchString(body) {
		t.Fatal("original list initializer became a default or incomplete stub")
	}
}

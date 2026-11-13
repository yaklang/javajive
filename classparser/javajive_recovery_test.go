package javaclassparser

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/yaklang/javajive/classparser/classfilebuilder"
)

func recoveryAttribute(payload []byte) *UnparsedAttribute {
	return &UnparsedAttribute{Name: javaJiveRecoveryAttributeName, Length: uint32(len(payload)), Info: payload}
}

func recoveryRecordPayload(records ...javaJiveRecoveryRecord) []byte {
	payload, err := json.Marshal(records)
	if err != nil {
		panic(err)
	}
	return payload
}

func parsedRecoveryFixture(t *testing.T, payloads ...[]byte) *ClassObject {
	t.Helper()
	builder := classfilebuilder.NewClass("fixture/RecoveryFixture", "java/lang/Object", 0x0021)
	for _, payload := range payloads {
		builder.AddClassAttribute(javaJiveRecoveryAttributeName, payload)
	}
	raw, err := builder.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return obj
}

func TestJavaJiveRecoveryParsedAttributeCommentsLeadDump(t *testing.T) {
	payload := recoveryRecordPayload(
		javaJiveRecoveryRecord{Version: 1, Code: "z.code", Action: "stub_method", Member: "z", Descriptor: "()V", Reason: "last"},
		javaJiveRecoveryRecord{Version: 1, Code: "a.code", Action: "stub_class", Entry: "classes.dex", Class: "Lfixture/RecoveryFixture;", Reason: "first"},
	)
	obj := parsedRecoveryFixture(t, payload)
	if len(obj.Attributes) != 1 {
		t.Fatalf("attributes = %d, want 1", len(obj.Attributes))
	}
	attribute, ok := obj.Attributes[0].(*UnparsedAttribute)
	if !ok || attribute.Name != javaJiveRecoveryAttributeName || string(attribute.Info) != string(payload) {
		t.Fatalf("recovery attribute was not preserved as UnparsedAttribute: %#v", obj.Attributes[0])
	}

	source, err := NewClassObjectDumper(obj).DumpClass()
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := "// javajive recovery: code=\"a.code\" action=\"stub_class\" entry=\"classes.dex\" class=\"Lfixture/RecoveryFixture;\" reason=\"first\"\n" +
		"// javajive recovery: code=\"z.code\" action=\"stub_method\" member=\"z\" descriptor=\"()V\" reason=\"last\"\n"
	if !strings.HasPrefix(source, wantPrefix) {
		t.Fatalf("recovery comments are not sorted at the very top:\n%s", source)
	}
	if !strings.Contains(source[len(wantPrefix):], "package fixture;") {
		t.Fatalf("ordinary source missing after recovery comments:\n%s", source)
	}
}

func TestJavaJiveRecoveryOrdinaryClassIsUnchanged(t *testing.T) {
	obj := parsedRecoveryFixture(t)
	source, err := NewClassObjectDumper(obj).DumpClass()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(source, "javajive recovery:") || !strings.HasPrefix(source, "package fixture;") {
		t.Fatalf("ordinary class gained a recovery preamble:\n%s", source)
	}
}

func TestJavaJiveRecoverySanitizesAndBoundsFields(t *testing.T) {
	longReason := strings.Repeat("界", javaJiveRecoveryMaxFieldRunes+20)
	payload := recoveryRecordPayload(javaJiveRecoveryRecord{
		Version: 1,
		Code:    " code\n// forged ",
		Action:  "stub_\"method\\path",
		Member:  "run\r\n// injected",
		Reason:  longReason,
	})
	comments := javaJiveRecoveryComments(&ClassObject{Attributes: []AttributeInfo{recoveryAttribute(payload)}})
	if strings.Count(comments, "\n") != 1 {
		t.Fatalf("untrusted fields injected comment lines: %q", comments)
	}
	if !strings.Contains(comments, `code="code // forged"`) ||
		!strings.Contains(comments, `action="stub_\"method\\path"`) ||
		!strings.Contains(comments, `member="run  // injected"`) {
		t.Fatalf("fields were not sanitized and escaped deterministically: %q", comments)
	}
	marker := ` reason="`
	start := strings.Index(comments, marker)
	if start < 0 {
		t.Fatalf("reason missing: %q", comments)
	}
	reason := comments[start+len(marker) : len(comments)-2]
	if utf8.RuneCountInString(reason) != javaJiveRecoveryMaxFieldRunes {
		t.Fatalf("bounded reason has %d runes, want %d", utf8.RuneCountInString(reason), javaJiveRecoveryMaxFieldRunes)
	}
}

func TestJavaJiveRecoveryMalformedPayloadsAreIgnored(t *testing.T) {
	valid := javaJiveRecoveryRecord{Version: 1, Code: "code", Action: "action", Reason: "reason"}
	tooMany := make([]javaJiveRecoveryRecord, javaJiveRecoveryMaxRecords+1)
	for i := range tooMany {
		tooMany[i] = valid
	}
	tooManyPayload, err := json.Marshal(tooMany)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]*UnparsedAttribute{
		"invalid JSON":        recoveryAttribute([]byte(`[{`)),
		"wrong top level":     recoveryAttribute([]byte(`{"version":1}`)),
		"trailing JSON":       recoveryAttribute([]byte(`[{"version":1,"code":"c","action":"a","reason":"r"}] {}`)),
		"unknown field":       recoveryAttribute([]byte(`[{"version":1,"code":"c","action":"a","reason":"r","extra":true}]`)),
		"unsupported version": recoveryAttribute(recoveryRecordPayload(javaJiveRecoveryRecord{Version: 2, Code: "c", Action: "a", Reason: "r"})),
		"missing required":    recoveryAttribute(recoveryRecordPayload(javaJiveRecoveryRecord{Version: 1, Code: "c", Action: "a"})),
		"too many records":    recoveryAttribute(tooManyPayload),
		"oversize payload":    recoveryAttribute([]byte(strings.Repeat(" ", javaJiveRecoveryMaxPayload+1))),
		"length mismatch":     {Name: javaJiveRecoveryAttributeName, Length: 99, Info: recoveryRecordPayload(valid)},
	}
	for name, attribute := range tests {
		t.Run(name, func(t *testing.T) {
			if got := javaJiveRecoveryComments(&ClassObject{Attributes: []AttributeInfo{attribute}}); got != "" {
				t.Fatalf("malformed payload produced comments: %q", got)
			}
		})
	}

	malformed := recoveryAttribute([]byte(`not json`))
	validAttribute := recoveryAttribute(recoveryRecordPayload(valid))
	if got := javaJiveRecoveryComments(&ClassObject{Attributes: []AttributeInfo{malformed, validAttribute}}); !strings.HasPrefix(got, "// javajive recovery:") {
		t.Fatalf("malformed sibling suppressed valid attribute: %q", got)
	}
}

func TestJavaJiveRecoverySortsAcrossAttributesAndCapsAggregate(t *testing.T) {
	obj := &ClassObject{Attributes: []AttributeInfo{
		recoveryAttribute(recoveryRecordPayload(javaJiveRecoveryRecord{Version: 1, Code: "b", Action: "stub", Reason: "two"})),
		recoveryAttribute(recoveryRecordPayload(javaJiveRecoveryRecord{Version: 1, Code: "a", Action: "stub", Reason: "one"})),
	}}
	comments := javaJiveRecoveryComments(obj)
	if strings.Index(comments, `code="a"`) > strings.Index(comments, `code="b"`) {
		t.Fatalf("records are not sorted across attributes: %q", comments)
	}

	attributes := make([]AttributeInfo, javaJiveRecoveryMaxRecords+1)
	payload := recoveryRecordPayload(javaJiveRecoveryRecord{Version: 1, Code: "c", Action: "a", Reason: "r"})
	for i := range attributes {
		attributes[i] = recoveryAttribute(payload)
	}
	if got := javaJiveRecoveryComments(&ClassObject{Attributes: attributes}); got != "" {
		t.Fatalf("aggregate record cap was not enforced: %d bytes", len(got))
	}
}

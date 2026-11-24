package javaclassparser

import (
	"strings"
	"testing"
)

func TestJacksonSameJarNestedNamesPreserved(t *testing.T) {
	input := `package com.fasterxml.jackson.annotation;

public class JsonInclude$Value {
	public JsonInclude$Value merge(com.fasterxml.jackson.annotation.JsonFormat$Shape s, Object var2) {
		return (s != null) ? (JsonInclude$Value.empty()) : (var2);
	}
	public static JsonInclude$Value empty() { return null; }
}`

	sameJar := func(internal string) bool {
		return internal == "com/fasterxml/jackson/annotation/JsonInclude$Value" ||
			internal == "com/fasterxml/jackson/annotation/JsonFormat$Shape"
	}

	// Same-JAR mode must preserve flat `$` class/reference names so the declaration
	// header does not turn into `class JsonInclude.Value` (syntax error) and siblings resolve it.
	kept := fixJacksonRemainingReconstructsWithSameJar(input, sameJar)
	if !strings.Contains(kept, "public class JsonInclude$Value") {
		t.Fatalf("same-JAR mode unexpectedly corrupted class declaration header:\n%s", kept)
	}
	if !strings.Contains(kept, "com.fasterxml.jackson.annotation.JsonFormat$Shape") {
		t.Fatalf("same-JAR mode unexpectedly dotted JsonFormat$Shape:\n%s", kept)
	}
	if strings.Contains(kept, "JsonInclude.Value") {
		t.Fatalf("same-JAR mode unexpectedly dotted JsonInclude$Value:\n%s", kept)
	}

	// External classpath mode (nil / false) retains historical dotted rewrite.
	external := fixJacksonRemainingReconstructs(input)
	if !strings.Contains(external, "JsonInclude.Value") || !strings.Contains(external, "JsonFormat.Shape") {
		t.Fatalf("external classpath mode did not dot Jackson nested types:\n%s", external)
	}

	// ClassObjectDumper.hasSameJarClass checks both c.obj and foldSiblingResolver.
	d := &ClassObjectDumper{
		foldSiblingResolver: func(name string) ([]byte, bool) {
			return nil, name == "com/fasterxml/jackson/annotation/JsonInclude$Value"
		},
	}
	if !d.hasSameJarClass("com/fasterxml/jackson/annotation/JsonInclude$Value") {
		t.Fatal("hasSameJarClass should detect sibling resolved via foldSiblingResolver")
	}
	if d.hasSameJarClass("com/fasterxml/jackson/annotation/JsonFormat$Shape") {
		t.Fatal("hasSameJarClass reported true for missing sibling")
	}
}

package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestTryCatchPlacementKeepsCatchIfElse(t *testing.T) {
	body := strings.Join([]string{
		"if ((var2) != (0)){",
		"\ttry{",
		"\t\tthis.beforeDelivery((Method)(null));",
		"\t}catch(ResourceException var3){",
		"\t\tif (ReflectionUtils.declaresException(var1.getMethod(),var3.getClass())){",
		"\t\t\tthrow (Throwable) var3;",
		"\t\t}else{",
		"\t\t\tthrow new GenericMessageEndpointFactory$InternalResourceException(var3);",
		"\t\t}",
		"\t}",
		"}",
	}, "\n")
	got := fixTryCatchExceptionPlacement(body)
	if got != body {
		t.Fatalf("catch if/else was rewritten:\n%s", got)
	}
}

func TestTryCatchPlacementStillMovesReflectionCall(t *testing.T) {
	body := strings.Join([]string{
		"if (cond){",
		"\tctor = type.getDeclaredConstructor();",
		"}",
		"try{",
		"\ttry{",
		"\t\tready();",
		"\t}catch(Exception ex){",
		"\t\tthrow new RuntimeException(ex);",
		"\t}",
		"}catch(Throwable t){ }",
	}, "\n")
	got := fixTryCatchExceptionPlacement(body)
	if strings.Contains(got, "ctor = type.getDeclaredConstructor();") && strings.Index(got, "getDeclaredConstructor") < strings.Index(got, "try{") {
		t.Fatalf("reflection call stayed before the try:\n%s", got)
	}
	if !strings.Contains(got, "getDeclaredConstructor") {
		t.Fatalf("reflection call was deleted:\n%s", got)
	}
}

func TestGenericMessageEndpointCompatibilityParsesShape(t *testing.T) {
	data, err := os.ReadFile("testdata/GenericMessageEndpoint.class")
	if err != nil {
		t.Fatal(err)
	}
	res, err := DecompileWithOptions(data, DecompileOptions{Mode: Compatibility})
	if err != nil {
		t.Fatal(err)
	}
	src := res.Source
	if !strings.Contains(src, "catch(ResourceException var3){\n\t\t\t\tif (ReflectionUtils.declaresException(var1.getMethod(),var3.getClass())){") {
		t.Fatalf("catch lost its if:\n%s", src)
	}
	if !strings.Contains(src, "try{\n\t\t\t\t\tthis.afterDelivery();") {
		t.Fatalf("afterDelivery is not the try body:\n%s", src)
	}
}

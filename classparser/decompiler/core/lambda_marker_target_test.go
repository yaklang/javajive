package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestLambdaMarkerCreationKeepsInstantiatedPrimaryTarget(t *testing.T) {
	for _, kind := range []string{"instantiated", "raw", "different erasure", "no markers"} {
		t.Run(kind, func(t *testing.T) {
			raw := types.NewJavaClass("java.util.function.Function")
			target := types.NewParameterizedType("java.util.function.Function", []types.JavaType{types.NewJavaClass("java.util.List"), types.NewJavaClass("java.util.stream.Stream")})
			if kind == "raw" {
				target = raw
			}
			if kind == "different erasure" {
				target = types.NewParameterizedType("java.util.function.Supplier", []types.JavaType{types.NewJavaClass("java.util.List")})
			}
			poly := values.NewCustomValue(func(*class_context.ClassContext) string { return "List::stream" }, func() types.JavaType { return target })
			poly.Flag = "lambda"
			lit := func(n int) values.JavaValue { return values.NewJavaLiteral(n, types.NewJavaPrimer(types.JavaInteger)) }
			marker := values.NewJavaClassValue(types.NewJavaClass("example.Marker"))
			req := CallSiteRequest{Identity: IdentityLambdaAltMetafactory, StaticArgs: []values.JavaValue{nil, nil, nil, lit(lambdaFlagMarkers), lit(1), marker}, OriginPC: 9}
			if kind == "no markers" {
				req.StaticArgs[4] = lit(0)
			}
			got := t19PreserveMarkers(req, poly, raw)
			if kind == "no markers" {
				if got != poly {
					t.Fatal("unmarked creation changed")
				}
				return
			}
			intersection, ok := got.(*values.LambdaIntersection)
			if !ok || intersection.Value != poly || len(intersection.Markers) != 1 || intersection.Markers[0].String(&class_context.ClassContext{}) != "Marker" {
				t.Fatal("marker identity or poly creation lost")
			}
			_, parameterized := types.AsParameterizedType(intersection.Primary)
			if parameterized != (kind == "instantiated") {
				t.Fatalf("primary parameterized=%t for %s", parameterized, kind)
			}
		})
	}
}

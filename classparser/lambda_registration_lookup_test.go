package javaclassparser

import (
	"bytes"
	"strings"
	"testing"
)

func TestAdversarialLambdaLayoutSeparatesDeclarationLookupFromRegistration(t *testing.T) {
	raw := lambdaParameterTestOriginal(t)
	for _, variant := range []string{"lookup only", "unclassified family", "owned family", "failed", "private getter", "retained accessor", "constructor bridge", "root delegation", "local class", "enum body", "registration layout", "anonymous unit", "member anonymous", "marker", "root handle"} {
		t.Run(variant, func(t *testing.T) {
			d, source, methods := lambdaEmissionTestDumper(t, raw)
			var first, second *dumpedMethods
			for _, method := range methods {
				if method.methodName == "unary" {
					first = method
				} else if method.methodName == "pinned" {
					second = method
				}
			}
			if first == nil || second == nil {
				t.Fatal("original independent factory declarations")
			}
			source = strings.Replace(source, first.code, "/*FIRST*/", 1)
			source = strings.Replace(source, second.code, first.code, 1)
			source = strings.Replace(source, "/*FIRST*/", second.code, 1)
			p := &nativeMemberFamily{declarationLookupOnly: true, children: map[string]*nativeMemberClass{}}
			d.nativeMemberRoot = p
			switch variant {
			case "unclassified family":
				p.declarationLookupOnly = false
			case "owned family":
				p.owner = d.obj.GetClassName()
			case "failed":
				p.failed = true
			case "private getter":
				p.getters = map[string]*nativeMemberPrivateGetter{"original": nil}
			case "retained accessor":
				p.retainedAccessors = map[string]*nativeMemberPrivateGetter{"original": nil}
			case "constructor bridge":
				p.rootAccessBridges = map[string]*nativeConstructorAccessBridge{"original": nil}
			case "root delegation":
				p.rootBridgeDelegations = map[string]*nativeRootBridgeDelegation{"original": nil}
			case "local class":
				p.methodLocals = map[string]*nativeMethodLocalClass{"original": nil}
			case "enum body":
				p.enumConstants = map[string]*nativeEnumConstantBody{"original": nil}
			case "registration layout":
				p.registrationLayouts = map[string]*nativeMemberRegistrationScope{"original": nil}
			case "anonymous unit":
				p.anonymousUnits = map[string]*nativeAnonymousFamily{"original": nil}
			case "member anonymous":
				p.memberAnonymous = map[string]*nativeAnonymousFamily{"original": nil}
			case "marker":
				p.emptyMarkers = map[string]*ClassObject{"original": nil}
			case "root handle":
				p.rootHandleSource = &nativeMemberClass{}
			}
			before := d.obj.Bytes()
			result, known := d.native8LambdaSourceIdentityLayout(source, nil, methods)
			if known != (variant == "lookup only") || !bytes.Equal(before, d.obj.Bytes()) {
				t.Fatalf("lookup/registration separation=%v for %s", known, variant)
			}
			if known && !d.native8LambdaSourceIdentitiesClosed(result, nil, methods) || !known && result != source {
				t.Fatal("layout published an unclosed factory order or a speculative edit")
			}
		})
	}
}

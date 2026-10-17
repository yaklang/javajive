package javaclassparser

import (
	"context"
	"fmt"
	"strings"
	"testing"

	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialMethodSourceRequestRefusesNonsyntheticBodyInlining(t *testing.T) {
	fixture := methodParameterNonsyntheticLambdaFixture()
	fixture.refuseSource = true
	methodParameterNameRoundTrip(t, fixture, "none")
}

func TestAdversarialMethodSourceRequestSameCaptureCountKeepsEnclosingNamespace(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint("reverse=", reverse), func(t *testing.T) {
			fixture := methodParameterSameCaptureLambdaFixture(reverse)
			fixture.mutate = nil
			methodParameterNameRoundTrip(t, fixture, "none")
		})
	}
}

func TestAdversarialMethodSourceRequestLambdaFailurePreservesOrdinaryCompletedPacket(t *testing.T) {
	files := methodParameterBoundaryCompile(t, `class GuardOwner{static int read(int originalNamed){return originalNamed;}static int impl(int captured,int left,int right){return 100*captured+10*left+right;}static java.util.function.IntBinaryOperator first(int captured){return(left,right)->impl(captured,left,right);}static java.util.function.IntUnaryOperator second(int captured,int fixed){return right->impl(captured,fixed,right);}}class GuardDriver{public static void main(String[]args)throws Exception{if(GuardOwner.read(7)!=7||GuardOwner.impl(7,2,3)!=723||!GuardOwner.class.getDeclaredMethod("read",int.class).getParameters()[0].getName().equals("originalNamed"))throw new AssertionError();System.out.println("boundary:original");}}`, "GuardOwner")
	d := methodParameterBoundaryDumper(t, files["GuardOwner.class"])
	ordinary, err := d.DumpMethod("impl", "(III)I")
	if err != nil || ordinary == nil || !strings.Contains(ordinary.code, "static int impl(int captured, int left, int right)") {
		t.Fatalf("ordinary source packet: %v %+v", err, ordinary)
	}
	// lambdaMethods remains declaration-suppression bookkeeping. It cannot
	// change a later explicit ordinary source request into an arrow header.
	d.lambdaMethods["impl"] = []string{"(III)I"}
	request, known := d.newLambdaMethodSourceRequest(1)
	if !known {
		t.Fatal("lambda source request")
	}
	d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
	_, err = d.dumpMethodWithSourceRequest("impl", "(III)I", coreutils.NewRootVariableId(), nil, request)
	if err == nil {
		t.Fatal("lambda source request borrowed an ordinary success despite its exhausted proof budget")
	}
	if d.dumpedMethodsSet[fmt.Sprintf("name:impl,desc:(III)I,source:lambda-body,captures:1,request:%d", request.serial)] != nil {
		t.Fatal("failed lambda representation remained cached")
	}
	d.Work = nil // The fresh ordinary cache gate has its own bounded validation.
	if cached, err := d.DumpMethod("impl", "(III)I"); err != nil || cached != ordinary || cached.code != ordinary.code {
		t.Fatal("failed lambda request changed the completed ordinary representation")
	}
	d.Work = nil
	helper := methodSourceRequestSyntheticName(t, d, "(III)I")
	physicalHelper, err := d.DumpMethod(helper, "(III)I")
	if err != nil || physicalHelper == nil {
		t.Fatal("original synthetic declaration view", err)
	}
	for _, captures := range []int{1, 2, 1} {
		request, known := d.newLambdaMethodSourceRequest(captures)
		if !known {
			t.Fatal("fresh lambda source request")
		}
		lambda, err := d.dumpMethodWithSourceRequest(helper, "(III)I", coreutils.NewRootVariableId(), nil, request)
		header := "(int l0) ->"
		if captures == 1 {
			header = "(int l0, int l1) ->"
		}
		if err != nil || lambda == nil || lambda == ordinary || lambda == physicalHelper || !strings.Contains(lambda.code, header) || strings.Contains(lambda.code, "int captured") {
			t.Fatalf("typed lambda representation with %d captures: %v %+v", captures, err, lambda)
		}
		if cached, err := d.DumpMethod("impl", "(III)I"); err != nil || cached != ordinary {
			t.Fatal("ordinary -> lambda -> ordinary changed the physical declaration packet")
		}
		if cached, err := d.DumpMethod(helper, "(III)I"); err != nil || cached != physicalHelper {
			t.Fatal("synthetic declaration -> lambda -> declaration cache role changed")
		}
	}
	for _, invalid := range []methodSourceRequest{{role: 9}, {role: methodSourceDeclaration, captureCount: 1}, {role: methodSourceLambdaBody}, {role: methodSourceLambdaBody, captureCount: -1, serial: 1}, {role: methodSourceLambdaBody, captureCount: 256, serial: 1}} {
		if _, err := d.dumpMethodWithSourceRequest("impl", "(III)I", coreutils.NewRootVariableId(), nil, invalid); err == nil {
			t.Fatal("invalid typed source request accepted")
		}
	}
	if cached, err := d.DumpMethod("impl", "(III)I"); err != nil || cached != ordinary {
		t.Fatal("invalid requests changed the ordinary cache entry")
	}
}

func TestAdversarialMethodSourceRequestsAreFreshAndBounded(t *testing.T) {
	d := &ClassObjectDumper{}
	first, known := d.newLambdaMethodSourceRequest(1)
	if !known {
		t.Fatal("first callback request")
	}
	second, known := d.newLambdaMethodSourceRequest(1)
	if !known || first.serial == 0 || second.serial != first.serial+1 {
		t.Fatal("distinct callback contexts shared request identity")
	}
	d.lambdaDepth = methodSourceLambdaDepthLimit - 1
	if _, known := d.newLambdaMethodSourceRequest(1); !known {
		t.Fatal("last admitted source depth")
	}
	last := d.methodSourceRequestSerial
	d.lambdaDepth++
	if _, known := d.newLambdaMethodSourceRequest(1); known || d.methodSourceRequestSerial != last {
		t.Fatal("new callbacks can expand beyond the fixed lambda source depth")
	}
	d.lambdaDepth = 0
	d.methodSourceRequestSerial = methodSourceLambdaRequestLimit - 1
	if _, known := d.newLambdaMethodSourceRequest(1); !known || d.methodSourceRequestSerial != methodSourceLambdaRequestLimit {
		t.Fatal("last admitted callback request")
	}
	if _, known := d.newLambdaMethodSourceRequest(1); known || d.methodSourceRequestSerial != methodSourceLambdaRequestLimit {
		t.Fatal("nil Work permits unbounded callback expansion")
	}
	d.methodSourceRequestSerial = ^uint64(0)
	if _, known := d.newLambdaMethodSourceRequest(1); known || d.methodSourceRequestSerial != ^uint64(0) {
		t.Fatal("request identity wrapped and could alias a completed packet")
	}
	for _, budget := range []string{"work", "memory", "canceled"} {
		t.Run(budget, func(t *testing.T) {
			d := &ClassObjectDumper{}
			switch budget {
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				if _, known := d.newLambdaMethodSourceRequest(1); !known {
					t.Fatal("first budgeted request")
				}
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := d.methodSourceRequestSerial
			if _, known := d.newLambdaMethodSourceRequest(1); known || d.methodSourceRequestSerial != before {
				t.Fatal("failed request budget minted a reusable callback identity")
			}
		})
	}
}

func TestAdversarialMethodSourceRequestOrdinaryDeclarationAfterFactories(t *testing.T) {
	fixture := methodParameterNonsyntheticLambdaFixture()
	fixture.mutate = nil
	declaration := `static int impl(int capture,int left,int right){return 100*capture+10*left+right;}`
	if strings.Count(fixture.declaration, declaration) != 1 {
		t.Fatal("authored original declaration order")
	}
	fixture.declaration = strings.TrimSuffix(strings.Replace(fixture.declaration, declaration, "", 1), "}") + declaration + "}"
	methodParameterNameRoundTrip(t, fixture, "none")
}

func TestAdversarialMethodSourceRequestKeepsOrdinaryVarargsMetadata(t *testing.T) {
	methodParameterNameRoundTrip(t, methodParameterNameCase{
		name: "ordinary varargs target", declaration: `class NamedOwner{static int impl(int captured,int... values){return 10*captured+values.length;}static java.util.function.ToIntFunction<int[]> first(int captured){return values->impl(captured,values);}static int read(int originalNamed){return originalNamed;}}`,
		check:          `if(NamedOwner.impl(7,1,2)!=72||NamedOwner.first(7).applyAsInt(new int[]{1,2})!=72||!NamedOwner.class.getDeclaredMethod("impl",int.class,int[].class).isVarArgs())throw new AssertionError("ordinary varargs declaration identity");`,
		parameterTypes: `int.class`, names: []string{"originalNamed"}, parameters: true,
	}, "none")
}

func TestAdversarialMethodSourceRequestKeepsSameRequestInProgressRecursion(t *testing.T) {
	files := methodParameterBoundaryCompile(t, `class GuardOwner{static int read(int originalNamed){return originalNamed;}static int impl(int captured,int left,int right){return 100*captured+10*left+right;}static java.util.function.IntBinaryOperator first(int captured){return(left,right)->Integer.valueOf(100*captured+10*left+right).intValue();}}class GuardDriver{public static void main(String[]args)throws Exception{if(GuardOwner.read(7)!=7||GuardOwner.impl(7,2,3)!=723||!GuardOwner.class.getDeclaredMethod("read",int.class).getParameters()[0].getName().equals("originalNamed"))throw new AssertionError();System.out.println("boundary:original");}}`, "GuardOwner")
	d := methodParameterBoundaryDumper(t, files["GuardOwner.class"])
	helper := methodSourceRequestSyntheticName(t, d, "(III)I")
	request, known := d.newLambdaMethodSourceRequest(1)
	if !known {
		t.Fatal("actual request")
	}
	prior := d.FuncCtx.SourceValueNameShadow
	seen := false
	d.FuncCtx.SourceValueNameShadow = func(name string) bool {
		if name == "Integer" && !seen {
			seen = true
			key := fmt.Sprintf("name:%s,desc:(III)I,source:lambda-body,captures:1,request:%d", helper, request.serial)
			inProgress := d.dumpedMethodsSet[key]
			recursive, err := d.dumpMethodWithSourceRequest(helper, "(III)I", coreutils.NewRootVariableId(), nil, request)
			if err != nil || inProgress == nil || recursive != inProgress {
				panic("same typed request lost its in-progress packet")
			}
		}
		return prior != nil && prior(name)
	}
	result, err := d.dumpMethodWithSourceRequest(helper, "(III)I", coreutils.NewRootVariableId(), nil, request)
	d.FuncCtx.SourceValueNameShadow = prior
	if err != nil || !seen || result == nil || !strings.Contains(result.code, "(int l0, int l1) ->") || d.lambdaDepth != 0 {
		t.Fatalf("bounded same-request recursion: %v seen=%v depth=%d", err, seen, d.lambdaDepth)
	}
}

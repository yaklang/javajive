package javaclassparser

import (
	"fmt"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestAdversarialMethodSourceReceiptCannotBorrowOrdinaryCache(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberLambdaFixture)
	for _, variant := range []string{"original", "ordinary cache only", "ordinary registration", "ordinary replaces request packet", "missing request packet", "foreign dumper", "foreign object", "foreign member", "stale request", "duplicate original declaration"} {
		t.Run(variant, func(t *testing.T) {
			child, method, _, _, _ := nativeMemberLambdaTestChild(t, files)
			if !nativeMemberLambdaImplementation(child, method, nil) {
				t.Fatal("original implementation protocol")
			}
			name, _ := sourceBridgeUTF8(child.object, method.NameIndex)
			descriptor, _ := sourceBridgeUTF8(child.object, method.DescriptorIndex)
			d := NewClassObjectDumper(child.object)
			d.lambdaMethods[name] = []string{descriptor}
			body := &dumpedMethods{member: method, code: "return null;", bodyCode: "return null;"}
			ordinaryKey := fmt.Sprintf("name:%s,desc:%s", name, descriptor)
			d.dumpedMethodsSet[ordinaryKey] = body
			if variant == "ordinary registration" {
				request, known := d.newLambdaMethodSourceRequest(2)
				if !known || d.recordLambdaMethodSourceBody(name, descriptor, request, body) || len(d.lambdaMethodSourceBodies) != 0 {
					t.Fatal("ordinary declaration packet minted lambda consumption")
				}
			} else if variant != "ordinary cache only" {
				key := registerMethodSourceBodyFixture(t, d, name, descriptor, 2, body)
				switch variant {
				case "ordinary replaces request packet":
					d.dumpedMethodsSet[key] = &dumpedMethods{member: method, code: body.code, bodyCode: body.bodyCode}
				case "missing request packet":
					delete(d.dumpedMethodsSet, key)
				case "foreign dumper":
					body.sourceDumper = NewClassObjectDumper(child.object)
				case "foreign object":
					body.sourceObject = &ClassObject{}
				case "foreign member":
					copy := *method
					body.member = &copy
				case "stale request":
					body.sourceRequest.serial++
				case "duplicate original declaration":
					copy := *method
					child.object.Methods = append(child.object.Methods, &copy)
				}
			}
			if got := nativeMemberLambdaSourceClosed(child, d, nil); got != (variant == "original") {
				t.Fatalf("source consumed=%v for %s", got, variant)
			}
		})
	}
}

func TestAdversarialMethodSourceReceiptRetryRestoresRequestIdentity(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberLambdaFixture)
	child, method, _, _, _ := nativeMemberLambdaTestChild(t, files)
	name, _ := sourceBridgeUTF8(child.object, method.NameIndex)
	descriptor, _ := sourceBridgeUTF8(child.object, method.DescriptorIndex)
	d := NewClassObjectDumper(child.object)
	body := &dumpedMethods{member: method, code: "return null;", bodyCode: "return null;"}
	firstKey := registerMethodSourceBodyFixture(t, d, name, descriptor, 2, body)
	firstSerial := d.methodSourceRequestSerial
	rollback := d.snapshotRetryState()
	body.bodyCode = "stub"
	d.lambdaMethodSourceBodies[method].method = nil
	second := &dumpedMethods{member: method, code: "return null;", bodyCode: "return null;"}
	secondKey := registerMethodSourceBodyFixture(t, d, name, descriptor, 2, second)
	spentSerial := d.methodSourceRequestSerial
	rollback()
	restored := d.lambdaMethodSourceBody(method, nil)
	if restored == nil || restored.bodyCode != "return null;" || restored.sourceRequest.serial != firstSerial || d.methodSourceRequestSerial != spentSerial || d.dumpedMethodsSet[firstKey] != restored || d.dumpedMethodsSet[secondKey] != nil {
		t.Fatal("retry left a failed source body or mismatched request/cache receipt")
	}
	request, known := d.newLambdaMethodSourceRequest(2)
	if !known || request.serial != spentSerial+1 || d.dumpedMethodsSet[secondKey] != nil {
		t.Fatal("retry request serial can alias a retained failed packet")
	}
}

func TestAdversarialMethodSourceReceiptFailedParentRevokesCompletedChild(t *testing.T) {
	source := `class GuardOwner{static int read(int originalNamed){return originalNamed;}static java.util.function.IntUnaryOperator first(int captured){java.util.function.IntUnaryOperator result=value->captured+value;Integer.valueOf(captured);return result;}}class GuardDriver{public static void main(String[]args)throws Exception{if(GuardOwner.read(7)!=7||GuardOwner.first(7).applyAsInt(2)!=9||!GuardOwner.class.getDeclaredMethod("read",int.class).getParameters()[0].getName().equals("originalNamed"))throw new AssertionError("original lambda parent");System.out.println("boundary:original");}}`
	files := methodParameterBoundaryCompile(t, source, "GuardOwner")
	for _, failure := range []string{"panic", "parent MP mutation", "incomplete", "arrow output budget"} {
		t.Run(failure, func(t *testing.T) {
			d := methodParameterBoundaryDumper(t, files["GuardOwner.class"])
			// The shared helper primes a complete class and clears its render cache.
			// This new transaction starts without those now-detached source indices.
			d.lambdaMethodSourceBodies, d.nativeLambdaFactorySources, d.nativeLambdaLocalSources = nil, nil, nil
			helper := methodSourceRequestSyntheticName(t, d, "(II)I")
			prior := d.FuncCtx.SourceValueNameShadow
			seen := false
			sentinel := &struct{}{}
			var completed *dumpedMethods
			var method *MemberInfo
			d.FuncCtx.SourceValueNameShadow = func(name string) bool {
				if name == "Integer" && !seen && len(d.lambdaMethodSourceBodies) > 0 {
					for m, receipt := range d.lambdaMethodSourceBodies {
						method = m
						completed = receipt.body
					}
					if completed == nil || completed.sourceReconstructionFailed() {
						panic("child was not complete before parent failure")
					}
					seen = true
					child := &nativeMemberClass{object: d.obj, lambdaImplementations: map[*MemberInfo]bool{method: true}}
					if !nativeMemberLambdaSourceClosed(child, d, nil) {
						panic("completed actual child receipt did not close before failure")
					}
					switch failure {
					case "panic":
						panic(sentinel)
					case "parent MP mutation":
						for _, a := range d.CurrentMethod.Attributes {
							if table, ok := a.(*UnparsedAttribute); ok && table.Name == "MethodParameters" {
								table.Info[4] ^= 16
							}
						}
					case "incomplete":
						d.FuncCtx.SourceRendering.IncompleteValue = true
					case "arrow output budget":
						d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
						d.FuncCtx.Work = d.Work
					}
				}
				return prior != nil && prior(name)
			}
			var result *dumpedMethods
			var renderErr error
			var caught any
			func() {
				defer func() { caught = recover() }()
				result, renderErr = d.DumpMethod("first", "(I)Ljava/util/function/IntUnaryOperator;")
			}()
			if !seen {
				t.Fatal("original child did not finish before the forced parent failure", renderErr, caught)
			}
			if failure == "panic" {
				if caught != sentinel {
					t.Fatal("original panic was not propagated", caught)
				}
			} else if caught != nil || renderErr == nil && !result.sourceReconstructionFailed() {
				t.Fatal("authored failed parent unexpectedly succeeded", renderErr, caught)
			}
			d.Work = nil
			d.FuncCtx.Work = nil
			if len(d.lambdaMethodSourceBodies) != 0 || len(d.nativeLambdaFactorySources) != 0 || len(d.nativeLambdaLocalSources) != 0 {
				t.Fatal("failed parent retained source consumption authority")
			}
			if completed == nil || d.dumpedMethodsSet[completed.sourceCacheKey] != completed || !strings.Contains(completed.sourceCacheKey, "source:lambda-body") {
				t.Fatal("parent failure removed independent completed render packet")
			}
			child := &nativeMemberClass{object: d.obj, lambdaImplementations: map[*MemberInfo]bool{method: true}}
			if d.lambdaMethodSourceBody(method, nil) != nil || nativeMemberLambdaSourceClosed(child, d, nil) {
				t.Fatal("completed render cache impersonated rolled-back source consumption")
			}
			serial := d.methodSourceRequestSerial
			request, ok := d.newLambdaMethodSourceRequest(1)
			if !ok || request.serial <= serial {
				t.Fatal("failed parent recycled a retained child request identity")
			}
			_ = helper
		})
	}
}

func TestAdversarialMethodSourceReceiptRetryRestoresFactoryAndLocalIndices(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberLambdaFixture)
	child, method, _, _, _ := nativeMemberLambdaTestChild(t, files)
	name, _ := sourceBridgeUTF8(child.object, method.NameIndex)
	desc, _ := sourceBridgeUTF8(child.object, method.DescriptorIndex)
	d := NewClassObjectDumper(child.object)
	body := &dumpedMethods{member: method, code: "return null;", bodyCode: "return null;"}
	registerMethodSourceBodyFixture(t, d, name, desc, 2, body)
	site := &nativeLambdaLocalCaptureSite{}
	source := &nativeLambdaLocalCaptureSource{factoryPC: 7, hasFactoryPC: true, implementationBody: body}
	d.nativeLambdaFactorySources = map[*nativeLambdaLocalCaptureSite]*nativeLambdaLocalCaptureSource{site: source}
	d.nativeLambdaLocalSources = map[string]*nativeLambdaLocalCaptureSource{"original": source}
	rollback := d.snapshotRetryState()
	source.factoryPC = 99
	source.implementationBody = nil
	d.nativeLambdaFactorySources[&nativeLambdaLocalCaptureSite{}] = &nativeLambdaLocalCaptureSource{}
	d.nativeLambdaLocalSources["failed"] = &nativeLambdaLocalCaptureSource{}
	rollback()
	restored := d.lambdaMethodSourceBody(method, nil)
	if restored == nil || len(d.nativeLambdaFactorySources) != 1 || len(d.nativeLambdaLocalSources) != 1 {
		t.Fatal("retry retained failed consumption indices")
	}
	for _, source := range []*nativeLambdaLocalCaptureSource{d.nativeLambdaFactorySources[site], d.nativeLambdaLocalSources["original"]} {
		if source == nil || source.factoryPC != 7 || source.implementationBody != restored {
			t.Fatal("retry source index detached from restored typed body")
		}
	}
}

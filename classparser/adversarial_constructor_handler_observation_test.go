package javaclassparser

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const closedHandlerObservationFixture = `
class HandlerObserveEffects{static Object seen;static final RuntimeException failure=new RuntimeException("handler original");static void fail(){throw failure;}}
class HandlerObserveParent{HandlerObserveParent(){observe();}private void observe(){try{HandlerObserveEffects.fail();}catch(Throwable failure){HandlerObserveEffects.seen=captured();}}Object captured(){return null;}}
class HandlerObserveOwner{final Object token;HandlerObserveOwner(Object t){token=t;}final class Child extends HandlerObserveParent{Child(){super();}Object captured(){return HandlerObserveOwner.this.token;}}HandlerObserveParent make(){return new Child();}}
class HandlerObserveDriver{public static void main(String[]args){int rows=0;Object token=new Object();for(Object value:new Object[]{null,token}){HandlerObserveEffects.seen=new Object();try{HandlerObserveParent p=new HandlerObserveOwner(value).make();if(HandlerObserveEffects.seen!=value||p.captured()!=value)throw new AssertionError("handler lost original early capture");}catch(NullPointerException ex){throw new AssertionError("handler read capture after incorrectly moved initialization",ex);}rows++;}System.out.println(rows+":handler:capture:observation");}}
`

// Catch(Throwable) and catch_type0 cover the same runtime domain. Change only
// that original table word, then verify the complete original family on the
// JVM before decompiling anything. The only capture observation lies in the
// exception handler: dropping its edges would appear to prove the normal path.
func TestAdversarialConstructorClosedFinallyCannotHideHandlerOnlyCaptureObservation(t *testing.T) {
	_, java := t04Tools(t)
	for _, prefix := range []string{"HandlerObserve", "HandlerWitness"} {
		t.Run(prefix, func(t *testing.T) {
			fixture := strings.ReplaceAll(closedHandlerObservationFixture, "HandlerObserve", prefix)
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					files := nativeCompileDebugClasses(t, fixture, debug)
					parent, err := Parse(bytes.Clone(files[prefix+"Parent.class"]))
					if err != nil {
						t.Fatal(err)
					}
					changed := 0
					for _, method := range parent.Methods {
						if name, _ := parent.getUtf8(method.NameIndex); name == "observe" {
							for _, attribute := range method.Attributes {
								if code, ok := attribute.(*CodeAttribute); ok {
									for _, handler := range code.ExceptionTable {
										if handler.CatchType == 0 {
											t.Fatal("expected authored typed Throwable handler")
										}
										name, known := sourceBridgeClassName(parent, handler.CatchType)
										if !known || name != "java/lang/Throwable" {
											t.Fatal("unexpected original catch", name)
										}
										handler.CatchType = 0
										changed++
									}
								}
							}
						}
					}
					if changed != 1 {
						t.Fatal("expected exactly one handler word edit", changed)
					}
					files[prefix+"Parent.class"] = parent.Bytes()
					original := t.TempDir()
					for name, raw := range files {
						if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					if got := t04RunJava(t, java, original, prefix+"Driver"); got != "2:handler:capture:observation\n" {
						t.Fatal("valid original catch-all oracle", got)
					}
					resolve := func(name string) ([]byte, bool) { raw, known := files[name+".class"]; return bytes.Clone(raw), known }
					for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
						t.Run(policy, func(t *testing.T) {
							if policy == "no-source-rewrites" {
								t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
							}
							if policy == "no-core-cleanups" {
								t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
							}
							for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
								t.Run(string(mode), func(t *testing.T) {
									var source string
									var err error
									raw := bytes.Clone(files[prefix+"Owner$Child.class"])
									if mode == "legacy" {
										source, err = DecompileWithResolver(raw, resolve)
									} else {
										result, e := DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
										source, err = result.Source, e
									}
									if err != nil || !strings.Contains(source, DecompileStubMarker) {
										t.Fatalf("handler-only capture observation must refuse unsafe motion: %v\n%s", err, source)
									}
								})
							}
						})
					}
				})
			}
		})
	}
}

package javaclassparser

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const liveHandlerObservationFixture = `
class HandlerLiveEffects{static int stage;static String trace;static Object seen;static final RuntimeException failure=new RuntimeException("live original");static void fail(int n){trace+=n;if(stage==0&&n==0)throw failure;}static void record(Object selected,Throwable ex){if(ex!=failure)throw new AssertionError("changed failure identity");trace+="M";seen=selected==null?null:((HandlerLiveParent)selected).captured();}}
class HandlerLiveParent{HandlerLiveParent(){observe();}private void observe(){Object selected=null;try{HandlerLiveEffects.fail(0);selected=this;HandlerLiveEffects.fail(1);throw HandlerLiveEffects.failure;}catch(Throwable ex){HandlerLiveEffects.record(selected,ex);}}Object captured(){return null;}}
class HandlerLiveOwner{final Object token;HandlerLiveOwner(Object t){token=t;}final class Child extends HandlerLiveParent{Child(){super();}Object captured(){return HandlerLiveOwner.this.token;}}HandlerLiveParent make(){return new Child();}}
class HandlerLiveDriver{public static void main(String[]args){int rows=0;Object token=new Object();for(Object value:new Object[]{null,token})for(int stage:new int[]{0,1}){HandlerLiveEffects.stage=stage;HandlerLiveEffects.trace="";HandlerLiveEffects.seen=new Object();try{HandlerLiveParent p=new HandlerLiveOwner(value).make();if(HandlerLiveEffects.seen!=(stage==0?null:value)||p.captured()!=value||!HandlerLiveEffects.trace.equals(stage==0?"0M":"01M"))throw new AssertionError("lost handler local, capture, effect order or exceptional snapshot");}catch(NullPointerException ex){throw new AssertionError("early handler receiver read after moved capture",ex);}rows++;}System.out.println(rows+":handler:live:observation");}}
`

// The first handler input has a receiver-free local; the later input retains
// THIS in that live slot. Projecting it away could hide an early virtual capture
// observation, even when all locals have the same computational categories.
// Catch(Throwable) and catch_type0 cover the same runtime domain. Change only
// that original table word, then verify the complete original family on the
// JVM before decompiling anything. The only capture observation lies in the
// exception handler: dropping its edges would appear to prove the normal path.
func TestAdversarialConstructorHandlerMemoCannotForgetLiveReceiverLocals(t *testing.T) {
	_, java := t04Tools(t)
	for _, prefix := range []string{"HandlerLive", "FrameWitness"} {
		t.Run(prefix, func(t *testing.T) {
			fixture := strings.ReplaceAll(liveHandlerObservationFixture, "HandlerLive", prefix)
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
					if got := t04RunJava(t, java, original, prefix+"Driver"); got != "4:handler:live:observation\n" {
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
										t.Fatalf("live receiver local must refuse unsafe handler memo reuse: %v\n%s", err, source)
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

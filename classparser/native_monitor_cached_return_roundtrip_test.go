package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"strings"
	"testing"
)

const nativeMonitorCachedReturnFixture = `class CacheEffects{static int calls;static String trace="";static final IllegalArgumentException failure=new IllegalArgumentException("original");static Object[] compute(CacheOwner owner,Object token,int fail){if(!Thread.holdsLock(owner.monitor()))throw new AssertionError("compute outside monitor");calls++;trace+="C";if(fail!=0)throw failure;return new Object[]{token};}}
class CacheOwner{private final Object lock;private Object[] cache;CacheOwner(Object lock){this.lock=lock;}Object monitor(){return lock;}Object[] get(Object token,int fail){synchronized(lock){if(cache==null){cache=CacheEffects.compute(this,token,fail);}return cache;}}void clear(){synchronized(lock){cache=null;}}static class Reader{Object[] snapshot(CacheOwner owner){return owner.cache;}}}
class CacheDriver{public static void main(String[]args){CacheOwner.Reader reader=new CacheOwner.Reader();int rows=0;for(Object token:new Object[]{null,new Object()}){Object lock=new Object();CacheOwner owner=new CacheOwner(lock);CacheEffects.calls=0;CacheEffects.trace="";Object[]first=owner.get(token,0);if(first[0]!=token||first!=reader.snapshot(owner)||owner.get(new Object(),1)!=first||CacheEffects.calls!=1||!CacheEffects.trace.equals("C")||Thread.holdsLock(lock))throw new AssertionError("lazy cache/identity/once/release");owner.clear();try{owner.get(token,1);throw new AssertionError("missing failure");}catch(IllegalArgumentException e){if(e!=CacheEffects.failure||reader.snapshot(owner)!=null||Thread.holdsLock(lock))throw new AssertionError("failure identity/partial state/release");}if(owner.get(token,0)[0]!=token||CacheEffects.calls!=3)throw new AssertionError("retry");rows++;}CacheOwner nil=new CacheOwner(null);CacheEffects.calls=0;try{nil.get(new Object(),0);throw new AssertionError("missing null lock failure");}catch(NullPointerException e){if(CacheEffects.calls!=0||reader.snapshot(nil)!=null)throw new AssertionError("effects before acquire");}System.out.println(rows+":monitor:cached:return:identity:order:failure");}}`

func TestNativeMonitorCachedFieldReturnRoundTrip(t *testing.T) {
	testNativePrivateSetterCompiledFixture(t, "CacheOwner", "CacheDriver", "2:monitor:cached:return:identity:order:failure\n", func(t *testing.T, debug string) map[string][]byte {
		return nativeCompileDebugClasses(t, nativeMonitorCachedReturnFixture, debug)
	}, nativeMonitorCachedReadProtected)
}
func TestNativeMonitorCachedFieldRenamedRoundTrip(t *testing.T) {
	source := strings.ReplaceAll(nativeMonitorCachedReturnFixture, "CacheOwner", "OtherCachedScope")
	source = strings.ReplaceAll(source, "cache", "originalSnapshot")
	testNativePrivateSetterCompiledFixture(t, "OtherCachedScope", "CacheDriver", "2:monitor:originalSnapshotd:return:identity:order:failure\n", func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, source, debug) }, nativeMonitorCachedReadProtected)
}

func nativeMonitorEarlyCachedFixture() string {
	source := strings.Replace(nativeMonitorCachedReturnFixture, "synchronized(lock){if(cache==null){cache=CacheEffects.compute(this,token,fail);}return cache;}", "synchronized(lock){if(fail==-1&&cache==null){return null;}else{if(cache==null){cache=CacheEffects.compute(this,token,fail);}return cache;}}", 1)
	source = strings.Replace(source, "Object[]first=owner.get(token,0);", "if(owner.get(token,-1)!=null||CacheEffects.calls!=0||Thread.holdsLock(lock))throw new AssertionError(\"early return effect/monitor\");Object[]first=owner.get(token,0);", 1)
	return source
}
func TestNativeMonitorEarlyCachedFieldReturnRoundTrip(t *testing.T) {
	testNativePrivateSetterCompiledFixture(t, "CacheOwner", "CacheDriver", "2:monitor:cached:return:identity:order:failure\n", func(t *testing.T, debug string) map[string][]byte {
		return nativeCompileDebugClasses(t, nativeMonitorEarlyCachedFixture(), debug)
	}, nativeMonitorCachedReadProtected)
}
func TestNativeMonitorEarlyCachedFieldRenamedRoundTrip(t *testing.T) {
	source := strings.ReplaceAll(nativeMonitorEarlyCachedFixture(), "CacheOwner", "DifferentEarlyCacheScope")
	testNativePrivateSetterCompiledFixture(t, "DifferentEarlyCacheScope", "CacheDriver", "2:monitor:cached:return:identity:order:failure\n", func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, source, debug) }, nativeMonitorCachedReadProtected)
}

func nativeMonitorSnapshotCachedFixture() string {
	source := strings.Replace(nativeMonitorCachedReturnFixture, "private Object[] cache;", "private Object[] cache;private Object[] input=new Object[0];void deleteInput(){input=null;}", 1)
	source = strings.Replace(source, "synchronized(lock){if(cache==null){cache=CacheEffects.compute(this,token,fail);}return cache;}", "synchronized(lock){if(input==null&&cache==null){return null;}else{if(cache==null){Object[] computed=CacheEffects.compute(this,token,fail);if(cache==null){cache=computed;deleteInput();}}return cache;}}", 1)
	source = strings.Replace(source, "synchronized(lock){cache=null;}", "synchronized(lock){cache=null;input=new Object[0];}", 1)
	return source
}
func TestNativeMonitorSnapshotCachedFieldReturnRoundTrip(t *testing.T) {
	testNativePrivateSetterCompiledFixture(t, "CacheOwner", "CacheDriver", "2:monitor:cached:return:identity:order:failure\n", func(t *testing.T, debug string) map[string][]byte {
		return nativeCompileDebugClasses(t, nativeMonitorSnapshotCachedFixture(), debug)
	}, nativeMonitorCachedReadProtected)
}
func TestNativeMonitorSnapshotCachedTernaryReturnRoundTrip(t *testing.T) {
	source := strings.Replace(nativeMonitorSnapshotCachedFixture(), "return cache;}}", "return fail==-2?null:cache;}}", 1)
	source = strings.Replace(source, "owner.clear();", "if(owner.get(token,-2)!=null||CacheEffects.calls!=1||Thread.holdsLock(lock))throw new AssertionError(\"ternary return/release\");owner.clear();", 1)
	testNativePrivateSetterCompiledFixture(t, "CacheOwner", "CacheDriver", "2:monitor:cached:return:identity:order:failure\n", func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, source, debug) }, nativeMonitorCachedReadProtected)
}

// Independent bytecode oracle: the authored getter reads each snapshot under
// the monitor catch-all, before release. A rebuilt GETFIELD outside every
// catch-all can race a concurrent writer, even when a sequential JVM run agrees.
func nativeMonitorCachedReadProtected(t *testing.T, name string, original, rebuilt []byte) {
	t.Helper()
	if strings.Contains(name, "$") {
		return
	}
	for _, raw := range [][]byte{original, rebuilt} {
		obj, e := Parse(raw)
		if e != nil {
			t.Fatal(e)
		}
		for _, m := range obj.Methods {
			n, _ := sourceBridgeUTF8(obj, m.NameIndex)
			if n != "get" {
				continue
			}
			for _, a := range m.Attributes {
				code, ok := a.(*CodeAttribute)
				if !ok {
					continue
				}
				d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
				if e := d.ParseOpcode(); e != nil {
					t.Fatal(e)
				}
				found := false
				for _, op := range d.Opcodes() {
					if op.Instr.OpCode != core.OP_GETFIELD {
						continue
					}
					member := constructorMotionMember(obj, op, core.OP_GETFIELD)
					if member == nil || member.Description != "[Ljava/lang/Object;" {
						continue
					}
					found = true
					protected := false
					for _, h := range code.ExceptionTable {
						if h.CatchType == 0 && op.CurrentOffset >= h.StartPc && op.CurrentOffset < h.EndPc {
							protected = true
						}
					}
					if !protected {
						t.Fatalf("snapshot field %s read at pc%d outside original monitor catch-all", member.Member, op.CurrentOffset)
					}
				}
				if !found {
					t.Fatal("missing snapshot read")
				}
			}
		}
	}
}

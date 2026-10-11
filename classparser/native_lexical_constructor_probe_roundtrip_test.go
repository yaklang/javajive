package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A source type query occurs before the hidden constructor parameter is renamed.
// It must not poison the transaction; the eventual projection still reopens
// the exact original parameter and GETFIELD path after constructor preparation.
const nativeLexicalConstructorProbeFixture = `class ProbeEffects{static String trace="";static int fail;static final RuntimeException error=new IllegalStateException("original");static void mark(String s,int stage){trace+=s;if(fail==stage)throw error;}}
class ProbeOwner<T extends Number>{static class Node<U>{final U value;Node(U value){this.value=value;}}final T token;ProbeOwner(T token){this.token=token;}Node<T> follow(Node<T> node){ProbeEffects.mark("F",2);return node;}class Cursor<U>{Node<T> next;Cursor(){ProbeEffects.mark("B",1);}T token(){return ProbeOwner.this.token;}}class Group{class Reader extends ProbeOwner<T>.Cursor<T>{final int marker;Reader(Node<T> node,int marker){super();next=ProbeOwner.this.follow(node);this.marker=marker;}Node<T> node(){return next;}T read(){return next==null?null:next.value;}}Reader make(Node<T> node,int marker){return new Reader(node,marker);}}}
class ProbeDriver{public static void main(String[]args){int count=0;for(Number token:new Number[]{null,Integer.valueOf(17),Double.valueOf(3.5)})for(boolean empty:new boolean[]{false,true})for(int fail=0;fail<3;fail++){ProbeOwner<Number> owner=new ProbeOwner<Number>(token);ProbeOwner.Node<Number> node=empty?null:new ProbeOwner.Node<Number>(token);ProbeOwner<Number>.Group group=owner.new Group();ProbeEffects.trace="";ProbeEffects.fail=fail;try{ProbeOwner<Number>.Group.Reader reader=group.make(node,37);if(fail!=0||reader.node()!=node||reader.read()!=(empty?null:token)||reader.token()!=token||reader.marker!=37||!ProbeEffects.trace.equals("BF"))throw new AssertionError("receiver/binding/identity/order");}catch(RuntimeException e){if(fail==0||e!=ProbeEffects.error||!ProbeEffects.trace.equals(fail==1?"B":"BF"))throw new AssertionError("original exception/order",e);}count++;}System.out.println(count+":constructor:probe:identity:original-order");}}`

func TestNativeLexicalConstructorTypeQueriesKeepSourceTransaction(t *testing.T) {
	for _, owner := range []string{"ProbeOwner", "DifferentConstructorScope"} {
		t.Run(owner, func(t *testing.T) {
			fixture := strings.ReplaceAll(nativeLexicalConstructorProbeFixture, "ProbeOwner", owner)
			testNativeIndependentFamilyFixture(t, fixture, []string{owner}, "ProbeDriver", "18:constructor:probe:identity:original-order\n", nativeLexicalExactSignatures)
		})
	}
}

func TestNativeLexicalConstructorQueriesNativeCompiler(t *testing.T) {
	legacy := os.Getenv("JAVA8_JAVAC")
	if legacy == "" {
		t.Skip("JAVA8_JAVAC required for the independent original compiler")
	}
	version, err := exec.Command(legacy, "-version").CombinedOutput()
	if err != nil || !strings.Contains(string(version), "javac 1.8.") {
		t.Fatal("original javac8 identity", err, string(version))
	}
	testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
		dir := t.TempDir()
		source := filepath.Join(dir, "ProbeOwner.java")
		if err := os.WriteFile(source, []byte(nativeLexicalConstructorProbeFixture), 0600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(legacy, "-proc:none", "-source", "8", "-target", "8", "-g:"+debug, "-d", dir, source).CombinedOutput(); err != nil {
			t.Fatal("original compile", err, string(out))
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		files := map[string][]byte{}
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".class") {
				raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
				if err != nil {
					t.Fatal(err)
				}
				files[entry.Name()] = raw
			}
		}
		return files
	}, NativeJavac8, legacy, []string{"ProbeOwner"}, "ProbeDriver", "18:constructor:probe:identity:original-order\n", nil, nativeLexicalExactSignatures)
}

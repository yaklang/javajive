package javaclassparser

import (
	"os"
	"testing"
)

func testNativeAnonymousArgumentRegistrationTree(t *testing.T, native bool) {
	sources := map[string]string{
		"p/Events.java": `package p;public class Events {
 public static Node[] nodes=new Node[4]; public static int count,fail,value;
 public static final RuntimeException failure=new IllegalArgumentException("identity");
 public static void enter(Node node){int i=count++;nodes[i]=node;
  int[] tags={11,22,33,44};if(node.id()!=(value^tags[i]))throw new AssertionError("capture before parent callback");
  if(fail==count)throw failure;}
}`,
		"p/Node.java": `package p;public abstract class Node {
 public final Node left,right;
 public Node(Node left,Node right){Events.enter(this);this.left=left;this.right=right;}
 public abstract int id();
}`,
		"q/TreeOwner.java": `package q;public class TreeOwner {
 public static p.Node make(final int value){
  return new p.Node(
   new p.Node(null,null){public int id(){return value^11;}},
   new p.Node(new p.Node(null,null){public int id(){return value^22;}},null){public int id(){return value^33;}}
  ){public int id(){return value^44;}};
 }
}`,
		"q/TreeDriver.java": `package q;public class TreeDriver {
 public static void main(String[]args)throws Exception {int rows=0;
  for(int value:new int[]{Integer.MIN_VALUE,-1,0,1,127,Integer.MAX_VALUE})for(int fail=0;fail<=4;fail++){
   p.Events.nodes=new p.Node[4];p.Events.count=0;p.Events.fail=fail;p.Events.value=value;
   boolean caught=false;p.Node root=null;
   try{root=TreeOwner.make(value);}catch(RuntimeException ex){if(ex!=p.Events.failure)throw ex;caught=true;}
   if(caught!=(fail!=0)||p.Events.count!=(fail==0?4:fail))throw new AssertionError("construction/failure order");
   for(int i=0;i<p.Events.count;i++){p.Node n=p.Events.nodes[i];Class<?> c=n.getClass();
    if(!c.getName().equals("q.TreeOwner$"+(i+1))||!c.isAnonymousClass()||c.getEnclosingMethod().getDeclaringClass()!=TreeOwner.class||!c.getEnclosingMethod().getName().equals("make")||c.getSuperclass()!=p.Node.class)throw new AssertionError("class identity or scope");}
   if(fail==0&&(root!=p.Events.nodes[3]||root.left!=p.Events.nodes[0]||root.right!=p.Events.nodes[2]||root.right.left!=p.Events.nodes[1]||root.right.right!=null||root.left.left!=null||root.left.right!=null))throw new AssertionError("argument graph identity");
   rows++;
  }System.out.println(rows+":argument-tree:registration:callbacks:failure-identity");
 }
}`,
	}
	want := "30:argument-tree:registration:callbacks:failure-identity\n"
	if native {
		compiler := os.Getenv("JAVA8_JAVAC")
		if compiler == "" {
			t.Skip("JAVA8_JAVAC required for independent compiler")
		}
		testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return fieldOwnerCompileJava8Sources(t, sources, debug) }, NativeJavac8, compiler, []string{"q/TreeOwner"}, "q.TreeDriver", want, nil, nativeLexicalExactSignatures)
		return
	}
	testNativePrivateSetterCompiledFixtureWithPolicies(t, "q/TreeOwner", "q.TreeDriver", want, func(t *testing.T, debug string) map[string][]byte {
		return nativeCompileSourceReleaseClasses(t, sources, debug, "8")
	}, nil, []string{"normal", "no-source-rewrites", "no-core-cleanups"}, nativeLexicalExactSignatures)
}

func TestAdversarialAnonymousArgumentRegistrationTree(t *testing.T) {
	testNativeAnonymousArgumentRegistrationTree(t, false)
}

func TestAdversarialAnonymousArgumentRegistrationTreeNativeCompiler(t *testing.T) {
	testNativeAnonymousArgumentRegistrationTree(t, true)
}

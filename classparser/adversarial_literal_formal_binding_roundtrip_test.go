package javaclassparser

import "testing"

func TestAdversarialNestedClassAndSameSpelledFormalKeepDistinctBindings(t *testing.T) {
	const source = `
class TypeOwner {static class Token {final int value;Token(int value){this.value=value;}}
 static class Box<TypeOwner$Token> {final java.util.List<TypeOwner.Token> fixed;final TypeOwner$Token generic;Box(java.util.List<TypeOwner.Token> fixed,TypeOwner$Token generic){this.fixed=fixed;this.generic=generic;}TypeOwner.Token read(int index){return fixed.get(index);}TypeOwner$Token value(){return generic;}}
 static Token read(Box<String> box,int index){return box.read(index);}static String text(Box<String> box){return box.value();}
}
class TypeDriver {public static void main(String[]args)throws Exception{int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(boolean nil:new boolean[]{false,true}){
 TypeOwner.Token token=nil?null:new TypeOwner.Token(n);String text=nil?null:new String("exact");java.util.List<TypeOwner.Token> storage=new java.util.ArrayList<TypeOwner.Token>();storage.add(token);TypeOwner.Box<String> box=new TypeOwner.Box<String>(storage,text);
 if(TypeOwner.read(box,0)!=token||TypeOwner.text(box)!=text)throw new AssertionError("class/formal substitution changed value identity");for(int index:new int[]{-1,1}){try{TypeOwner.read(box,index);throw new AssertionError("missing original bounds failure");}catch(IndexOutOfBoundsException expected){}}
 java.lang.reflect.Type field=TypeOwner.Box.class.getDeclaredField("fixed").getGenericType();if(!(field instanceof java.lang.reflect.ParameterizedType)||((java.lang.reflect.ParameterizedType)field).getActualTypeArguments()[0]!=TypeOwner.Token.class)throw new AssertionError("literal class was rebound to formal");java.lang.reflect.Type generic=TypeOwner.Box.class.getDeclaredMethod("value").getGenericReturnType();if(!(generic instanceof java.lang.reflect.TypeVariable)||((java.lang.reflect.TypeVariable<?>)generic).getGenericDeclaration()!=TypeOwner.Box.class||!((java.lang.reflect.TypeVariable<?>)generic).getName().equals("TypeOwner$Token"))throw new AssertionError("formal was rebound to class");rows++;
 }System.out.println(rows+":literal-class:formal:declaration:identity:bounds");}}
`
	testResourceCompletionFamily(t, source, "TypeOwner", "TypeDriver", "10:literal-class:formal:declaration:identity:bounds\n")
}

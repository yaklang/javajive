package javaclassparser

const nativeAnnotationFieldBindingFixture = `@java.lang.annotation.Retention(java.lang.annotation.RetentionPolicy.RUNTIME)
@interface FieldPacket {class Value {public final String withPrefix;Value(String text){withPrefix=text;}}}
class FieldPacketUser {String choose(int a,int b,int c,int d,FieldPacket.Value value){return value==null?"fallback":value.withPrefix;}}
class FieldBindingDriver {public static void main(String[]args){FieldPacketUser user=new FieldPacketUser();for(String text:new String[]{null,"","payload"}){FieldPacket.Value value=new FieldPacket.Value(text);if(user.choose(1,2,3,4,value)!=text)throw new AssertionError("field identity");}if(!user.choose(1,2,3,4,null).equals("fallback"))throw new AssertionError("null arm");if(FieldPacket.Value.class.getDeclaringClass()!=FieldPacket.class)throw new AssertionError("owner");System.out.println("typed-field:identity:null:owner");}}`

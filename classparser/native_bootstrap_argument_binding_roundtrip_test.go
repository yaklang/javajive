package javaclassparser

import (
	"strings"
	"testing"
)

// The declared Function<Object,Object> and its erased SAM do not describe the
// original argument check. Only the instantiated MethodType in the bootstrap
// binds that check, and its parameter type need not have a CONSTANT_Class.
const bootstrapArgumentBindingFixture = `class BootstrapArgumentOwner{
 static class Anchor{}
 static java.util.function.Function<Object,Object> function(){return BootstrapArgumentFactory::identity;}
}
class BootstrapArgumentFactory{static{BootstrapArgumentEffects.initializations++;}static Object identity(Object value){BootstrapArgumentEffects.trace+="I";if(BootstrapArgumentEffects.fail)throw BootstrapArgumentEffects.failure;return value;}}
class ForeignBootstrapScope{static class Value{}}
class BootstrapArgumentEffects{static String trace;static boolean fail;static int initializations;static final RuntimeException failure=new RuntimeException("identity");}
class BootstrapArgumentDriver{public static void main(String[]args){Object token=new ForeignBootstrapScope.Value();int rows=0;for(Object value:new Object[]{null,token,new Object(),"wrong"})for(boolean fail:new boolean[]{false,true}){
 BootstrapArgumentEffects.trace="";BootstrapArgumentEffects.fail=fail;
 java.util.function.Function<Object,Object> function=BootstrapArgumentOwner.function();
 boolean compatible=value==null||value instanceof ForeignBootstrapScope.Value;
 try{Object actual=function.apply(value);if(!compatible||fail||actual!=value||!BootstrapArgumentEffects.trace.equals("I"))throw new AssertionError("argument binding and result identity");}
 catch(ClassCastException e){if(compatible||!BootstrapArgumentEffects.trace.equals(""))throw new AssertionError("argument check before original effect",e);}
 catch(RuntimeException e){if(!compatible||!fail||e!=BootstrapArgumentEffects.failure||!BootstrapArgumentEffects.trace.equals("I"))throw new AssertionError("effect and exception identity",e);}
 rows++;}if(BootstrapArgumentOwner.Anchor.class.getDeclaringClass()!=BootstrapArgumentOwner.class||ForeignBootstrapScope.Value.class.getDeclaringClass()!=ForeignBootstrapScope.class)throw new AssertionError("original lexical ownership");System.out.println(rows+":bootstrap:argument-check:identity:effects");}}
`

func TestNativeBootstrapInstantiatedArgumentTypeKeepsOriginalRuntimeCheck(t *testing.T) {
	for _, row := range []struct {
		name, descriptor, token, sourceType, overload string
		shadow                                        bool
	}{
		{"reference", "LForeignBootstrapScope$Value;", "new ForeignBootstrapScope.Value()", "ForeignBootstrapScope.Value", "", false},
		{"reference widening", "LForeignBootstrapScope$Value;", "new ForeignBootstrapScope.Value()", "ForeignBootstrapScope.Value", "static Object identity(ForeignBootstrapScope.Value value){BootstrapArgumentEffects.trace+=\"X\";return value;}", false},
		{"reference array", "[LForeignBootstrapScope$Value;", "new ForeignBootstrapScope.Value[]{new ForeignBootstrapScope.Value()}", "ForeignBootstrapScope.Value[]", "", false},
		{"nested array", "[[LForeignBootstrapScope$Value;", "new ForeignBootstrapScope.Value[1][1]", "ForeignBootstrapScope.Value[][]", "", false},
		{"primitive array", "[I", "new int[]{1,-1}", "int[]", "", false},
		{"competing overload", "LForeignBootstrapScope$Value;", "new ForeignBootstrapScope.Value()", "ForeignBootstrapScope.Value", "static Object identity(ForeignBootstrapScope.Value value){BootstrapArgumentEffects.trace+=\"X\";return value;}", false},
		{"debug name collision", "LForeignBootstrapScope$Value;", "new ForeignBootstrapScope.Value()", "ForeignBootstrapScope.Value", "", true},
		{"class owner field shadow", "LForeignBootstrapScope$Value;", "new ForeignBootstrapScope.Value()", "ForeignBootstrapScope.Value", "", false},
		{"interface static implementation", "LForeignBootstrapScope$Value;", "new ForeignBootstrapScope.Value()", "ForeignBootstrapScope.Value", "", false},
		{"interface owner field shadow", "LForeignBootstrapScope$Value;", "new ForeignBootstrapScope.Value()", "ForeignBootstrapScope.Value", "", false},
		{"class initialization order", "LForeignBootstrapScope$Value;", "new ForeignBootstrapScope.Value()", "ForeignBootstrapScope.Value", "", false},
		{"independent repeated formal", "LForeignBootstrapScope$Value;", "new ForeignBootstrapScope.Value()", "ForeignBootstrapScope.Value", "", false},
		{"void result", "LForeignBootstrapScope$Value;", "new ForeignBootstrapScope.Value()", "ForeignBootstrapScope.Value", "", false},
		{"generated parameter owner collision", "LForeignBootstrapScope$Value;", "new ForeignBootstrapScope.Value()", "ForeignBootstrapScope.Value", "", false},
		{"unchanged entry control", "Ljava/lang/Object;", "new ForeignBootstrapScope.Value()", "Object", "", false},
	} {
		t.Run(row.name, func(t *testing.T) {
			for _, owner := range []string{"BootstrapArgumentOwner", "SeparateBootstrapBinding"} {
				t.Run(owner, func(t *testing.T) {
					fixture := strings.ReplaceAll(bootstrapArgumentBindingFixture, "BootstrapArgumentOwner", owner)
					fixture = strings.ReplaceAll(fixture, "Object token=new ForeignBootstrapScope.Value()", "Object token="+row.token)
					fixture = strings.ReplaceAll(fixture, "value instanceof ForeignBootstrapScope.Value", "value instanceof "+row.sourceType)
					fixture = strings.ReplaceAll(fixture, "class BootstrapArgumentFactory{", "class BootstrapArgumentFactory{"+row.overload)
					if row.name == "reference widening" {
						fixture = strings.ReplaceAll(fixture, "class ForeignBootstrapScope{static class Value{}", "class BootstrapArgumentBase{}class ForeignBootstrapScope{static class Value extends BootstrapArgumentBase{}")
					}
					resultDescriptor := "Ljava/lang/Object;"
					if row.name == "void result" {
						resultDescriptor = "V"
						fixture = strings.ReplaceAll(fixture, "java.util.function.Function<Object,Object>", "java.util.function.Consumer<Object>")
						fixture = strings.ReplaceAll(fixture, "static Object identity(Object value)", "static void identity(Object value)")
						fixture = strings.ReplaceAll(fixture, "return value;", "return;")
						fixture = strings.ReplaceAll(fixture, "Object actual=function.apply(value);if(!compatible||fail||actual!=value||", "function.accept(value);if(!compatible||fail||")
					}
					prefix := ""
					if strings.HasPrefix(row.name, "interface") {
						fixture = strings.ReplaceAll(fixture, "class BootstrapArgumentFactory{static{BootstrapArgumentEffects.initializations++;}", "interface BootstrapArgumentFactory{")
					}
					if strings.Contains(row.name, "owner field shadow") {
						fixture = strings.ReplaceAll(fixture, "static class Anchor{}", "static int factoryShadow;static class Anchor{}")
					}
					if row.name == "interface owner field shadow" {
						prefix = "bootstrap/binding/"
						fixture = "package bootstrap.binding;" + fixture
					}
					if row.name == "independent repeated formal" {
						fixture = strings.ReplaceAll(fixture, "java.util.function.Function<Object,Object>", "java.util.function.UnaryOperator<Object>")
					}
					if row.name == "class initialization order" {
						prelude := "java.util.function.Function<Object,Object> first=" + owner + ".function();if(BootstrapArgumentEffects.initializations!=0)throw new AssertionError(\"early implementation initialization\");try{first.apply(new Object());throw new AssertionError(\"missing argument check\");}catch(ClassCastException expected){}if(BootstrapArgumentEffects.initializations!=0)throw new AssertionError(\"implementation initialized before entry check\");"
						fixture = strings.ReplaceAll(fixture, "Object token=", prelude+"Object token=")
					}
					if row.shadow {
						fixture = strings.ReplaceAll(fixture, "function()", "function(int lambdaArgument0)")
						fixture = strings.ReplaceAll(fixture, owner+".function(int lambdaArgument0)", owner+".function(9)")
					}
					mutate := func(t *testing.T, files map[string][]byte) {
						object, err := Parse(files[prefix+owner+".class"])
						if err != nil {
							t.Fatal(err)
						}
						pool := NewConstantPoolWithConstant(&object.ConstantPool)
						if strings.Contains(row.name, "owner field shadow") {
							matched := false
							for _, field := range object.Fields {
								name, _ := sourceBridgeUTF8(object, field.NameIndex)
								if name == "factoryShadow" {
									field.NameIndex = uint16(pool.AddUtf8Info("BootstrapArgumentFactory"))
									matched = true
								}
							}
							if !matched {
								t.Fatal("original owner-shadow field")
							}
						}
						var bootstrap *BootstrapMethodsAttribute
						for _, attribute := range object.Attributes {
							if table, ok := attribute.(*BootstrapMethodsAttribute); ok {
								bootstrap = table
							}
						}
						if bootstrap == nil || len(bootstrap.BootstrapMethods) != 1 || len(bootstrap.BootstrapMethods[0].BootstrapArguments) != 3 {
							t.Fatal("original metafactory packet")
						}
						if row.name == "reference widening" {
							// Author a JVM-only handle whose implementation receives the
							// original parent type; its narrower same-name overload remains.
							factory, err := Parse(files["BootstrapArgumentFactory.class"])
							if err != nil {
								t.Fatal(err)
							}
							factoryPool := NewConstantPoolWithConstant(&factory.ConstantPool)
							matched := false
							for _, method := range factory.Methods {
								name, _ := sourceBridgeUTF8(factory, method.NameIndex)
								desc, _ := sourceBridgeUTF8(factory, method.DescriptorIndex)
								if name == "identity" && desc == "(Ljava/lang/Object;)Ljava/lang/Object;" {
									method.DescriptorIndex = uint16(factoryPool.AddUtf8Info("(LBootstrapArgumentBase;)Ljava/lang/Object;"))
									matched = true
								}
							}
							if !matched {
								t.Fatal("original parent parameter implementation")
							}
							files["BootstrapArgumentFactory.class"] = factory.Bytes()
						}
						packet := bootstrap.BootstrapMethods[0]
						if row.name == "reference widening" {
							handle := object.ConstantPool[packet.BootstrapArguments[1]-1].(*ConstantMethodHandleInfo)
							ref := object.ConstantPool[handle.ReferenceIndex-1].(*ConstantMethodrefInfo)
							binding := object.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
							ref.NameAndTypeIndex = uint16(pool.AppendConstantInfo(&ConstantNameAndTypeInfo{NameIndex: binding.NameIndex, DescriptorIndex: uint16(pool.AddUtf8Info("(LBootstrapArgumentBase;)Ljava/lang/Object;"))}))
						}
						old := object.ConstantPool[packet.BootstrapArguments[2]-1].(*ConstantMethodTypeInfo)
						text, known := sourceBridgeUTF8(object, old.DescriptorIndex)
						if !known || text != "(Ljava/lang/Object;)"+resultDescriptor {
							t.Fatal("original erased argument type", text)
						}
						// Leave the declared method Signature, erased SAM and original
						// external implementation handle untouched except the parent-view bank. A new CP slot avoids
						// changing the erased SAM if javac shared its MethodType entry.
						descriptor := strings.ReplaceAll(row.descriptor, "LForeignBootstrapScope$Value;", "L"+prefix+"ForeignBootstrapScope$Value;")
						desc := uint16(pool.AddUtf8Info("(" + descriptor + ")" + resultDescriptor))
						packet.BootstrapArguments[2] = uint16(pool.AppendConstantInfo(&ConstantMethodTypeInfo{DescriptorIndex: desc}))
						files[prefix+owner+".class"] = object.Bytes()
					}
					factory := "BootstrapArgumentFactory"
					if row.name == "generated parameter owner collision" {
						factory = "lambdaArgument0"
						fixture = strings.ReplaceAll(fixture, "BootstrapArgumentFactory", factory)
					}
					owned := []string{prefix + owner, prefix + factory, prefix + "ForeignBootstrapScope"}
					if row.name == "reference widening" {
						owned = append(owned, "BootstrapArgumentBase")
					}
					testNativeIndependentMutatedFamilyFixture(t, fixture, owned, strings.ReplaceAll(prefix, "/", ".")+"BootstrapArgumentDriver", "8:bootstrap:argument-check:identity:effects\n", mutate, nativeLexicalExactSignatures)
				})
			}

		})
	}
}

const bootstrapPairArgumentBindingFixture = `interface BootstrapArgumentFunction<R,L,S>{R mix(L left,S right);}
class BootstrapPairOwner{
 static class Anchor{}
 static java.util.function.BiFunction<Object,Object,Object> function(){return BootstrapPairFactory::pair;}
}
class BootstrapPairFactory{
 static Object pair(Object left,Object right){BootstrapArgumentEffects.trace+="P";if(BootstrapArgumentEffects.fail)throw BootstrapArgumentEffects.failure;return left==null?right:left;}
 static Object pair(ForeignBootstrapScope.Value left,int[] right){BootstrapArgumentEffects.trace+="X";return right;}
}
class ForeignBootstrapScope{static class Value{}}
class BootstrapArgumentEffects{static String trace;static boolean fail;static final RuntimeException failure=new RuntimeException("pair");}
class BootstrapPairDriver{public static void main(String[]args){Object token=new ForeignBootstrapScope.Value();Object array=new int[]{1,-1};int rows=0;
 for(Object left:new Object[]{null,token,new Object(),"wrong-left"})for(Object right:new Object[]{null,array,new Object(),"wrong-right"})for(boolean fail:new boolean[]{false,true}){
 BootstrapArgumentEffects.trace="";BootstrapArgumentEffects.fail=fail;
 java.util.function.BiFunction<Object,Object,Object> function=BootstrapPairOwner.function();
 boolean compatible=(left==null||left instanceof ForeignBootstrapScope.Value)&&(right==null||right instanceof int[]);
 try{Object actual=function.apply(left,right);if(!compatible||fail||actual!=(left==null?right:left)||!BootstrapArgumentEffects.trace.equals("P"))throw new AssertionError("two argument checks and original overload");}
 catch(ClassCastException e){if(compatible||!BootstrapArgumentEffects.trace.equals(""))throw new AssertionError("both entry checks before implementation",e);}
 catch(RuntimeException e){if(!compatible||!fail||e!=BootstrapArgumentEffects.failure||!BootstrapArgumentEffects.trace.equals("P"))throw new AssertionError("pair effect and exception identity",e);}
 rows++;}if(BootstrapPairOwner.Anchor.class.getDeclaringClass()!=BootstrapPairOwner.class||ForeignBootstrapScope.Value.class.getDeclaringClass()!=ForeignBootstrapScope.class)throw new AssertionError("pair lexical ownership");System.out.println(rows+":bootstrap:pair:overload:identity:effects");}}
`

func TestNativeBootstrapMultipleArgumentChecksKeepOriginalOverloadAndFormalOrder(t *testing.T) {
	for _, custom := range []bool{false, true} {
		name := "jdk"
		if custom {
			name = "declaration"
		}
		t.Run(name, func(t *testing.T) {
			for _, owner := range []string{"BootstrapPairOwner", "SeparatePairBinding"} {
				t.Run(owner, func(t *testing.T) {
					fixture := strings.ReplaceAll(bootstrapPairArgumentBindingFixture, "BootstrapPairOwner", owner)
					if custom {
						fixture = strings.ReplaceAll(fixture, "java.util.function.BiFunction", "BootstrapArgumentFunction")
						fixture = strings.ReplaceAll(fixture, "function.apply(left,right)", "function.mix(left,right)")
					}
					mutate := func(t *testing.T, files map[string][]byte) {
						object, err := Parse(files[owner+".class"])
						if err != nil {
							t.Fatal(err)
						}
						pool := NewConstantPoolWithConstant(&object.ConstantPool)
						var table *BootstrapMethodsAttribute
						for _, attribute := range object.Attributes {
							if found, ok := attribute.(*BootstrapMethodsAttribute); ok {
								table = found
							}
						}
						if table == nil || len(table.BootstrapMethods) != 1 || len(table.BootstrapMethods[0].BootstrapArguments) != 3 {
							t.Fatal("pair metafactory packet")
						}
						packet := table.BootstrapMethods[0]
						old := object.ConstantPool[packet.BootstrapArguments[2]-1].(*ConstantMethodTypeInfo)
						text, known := sourceBridgeUTF8(object, old.DescriptorIndex)
						if !known || text != "(Ljava/lang/Object;Ljava/lang/Object;)Ljava/lang/Object;" {
							t.Fatal("original pair input type", text)
						}
						desc := uint16(pool.AddUtf8Info("(LForeignBootstrapScope$Value;[I)Ljava/lang/Object;"))
						packet.BootstrapArguments[2] = uint16(pool.AppendConstantInfo(&ConstantMethodTypeInfo{DescriptorIndex: desc}))
						files[owner+".class"] = object.Bytes()
					}
					testNativeIndependentMutatedFamilyFixture(t, fixture, []string{owner, "BootstrapPairFactory", "ForeignBootstrapScope", "BootstrapArgumentFunction"}, "BootstrapPairDriver", "32:bootstrap:pair:overload:identity:effects\n", mutate, nativeLexicalExactSignatures)
				})
			}
		})
	}
}

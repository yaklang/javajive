import java.lang.reflect.Field;
import java.lang.reflect.Member;
import java.lang.reflect.Method;

// Regression seed for the reflection-family ternary LUB (hierarchy table reflection rows,
// kill-switch JDEC_TYPELUB_OFF) + the ternary-decl cached-type refresh (kill-switch
// JDEC_TERNARY_DECL_LUB_CACHE_OFF).
//
// `cond ? this.method : this.field` merges java.lang.reflect.Method and Field. Their only
// use-correct denotable common type is the INTERFACE java.lang.reflect.Member (carries getName /
// getDeclaringClass / getModifiers), NOT the class AccessibleObject (which has none of those). Without
// the reflection rows MergeTypes falls back to the first arm (Method), and the local is declared
// `Method m = cond ? method : field` -- javac "bad type in conditional expression". Mirrors fastjson2
// FieldReader.toString / compareTo and FieldReaderObject. Return type is String (not a reflection
// type) so the only Method/Field/Member token under test is the local declaration.
public class ReflectMemberLUBSeed {
    Method method;
    Field field;

    String name() {
        Member m = (this.method != null) ? this.method : this.field;
        return (m != null) ? m.getName() : null;
    }
}

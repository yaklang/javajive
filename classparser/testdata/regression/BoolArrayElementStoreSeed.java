// Regression seed for the boolean[] element-store int->boolean coercion
// (values.CoerceBooleanAssignRHS via arrayStoreRHS; JDEC_BOOL_TO_INT_COERCE_OFF).
//
// A boolean expression stored into a boolean[] element compiles (like every boolean value on the JVM)
// to a materialized int diamond `cond ? 1 : 0` written with bastore (which serves both byte[] and
// boolean[]). Without the coercion the decompiler renders the raw int RHS into a boolean[] element,
// which Java rejects ("int cannot be converted to boolean"). Mirrors spring ASM
// ClassReader.readTypeAnnotationTarget / AttributeMethods.<init> and commons-lang3 Conversion.
public class BoolArrayElementStoreSeed {
    boolean[] flags(Class<?>[] types) {
        boolean[] out = new boolean[types.length];
        for (int i = 0; i < types.length; i++) {
            Class<?> t = types[i];
            out[i] = t == Class.class || t == Class[].class || t.isEnum();
        }
        return out;
    }
}

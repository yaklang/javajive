// Regression seed for the array-parameter null-Object argument cast
// (arrayParamRefArgCast in renderArgAt; kill-switch JDEC_ARRAY_PARAM_REF_ARG_CAST_OFF).
//
// A null-initialized local passed only to a typed array parameter is `aconst_null; astore; ...; aload;
// invoke(...[B...)`. The decompiler types the null local as Object (the default null type), so the call
// renders `sizeOf(objVar, 0)` where the overload expects `byte[]` -> javac rejects "Object cannot be
// converted to byte[]". The value already occupies the array parameter slot in bytecode (a null), so an
// explicit `(byte[])` cast at the call site is behaviour-preserving. Mirrors spring ASM
// Attribute.computeAttributesSize / putAttributes and cglib Enhancer (Object -> Object[]).
public class ArrayParamRefArgSeed {
    int size() {
        byte[] data = null;
        int a = 0;
        int b = -1;
        int c = -1;
        return sizeOf(data, a, b, c);
    }

    int sizeOf(byte[] data, int a, int b, int c) {
        return data == null ? a + b + c : data.length;
    }
}

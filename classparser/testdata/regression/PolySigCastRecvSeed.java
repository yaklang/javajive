// Seed for self-parenthesizing the polymorphic-signature return cast (JDEC_POLYSIG_CAST_PARENS_OFF).
//
// `MethodHandle.invoke`/`invokeExact` are @PolymorphicSignature: the source cast `(Boolean) mh.invoke(...)`
// gives the invoke call a non-Object descriptor return (Boolean), which the decompiler re-emits as a cast
// `(Boolean)(mh.invoke(...))`. When that cast is the RECEIVER of a further call (`.booleanValue()`), single
// outer parens misparse: `(Boolean)(x).booleanValue()` binds as `(Boolean)((x).booleanValue())` (a cast
// binds looser than a call), so javac tries booleanValue() on the Object invoke result ("cannot find symbol:
// method booleanValue()"; fastjson2 JSONReader:3130 `METHOD_HANDLE_HAS_NEGATIVE.invoke(...)`). The fix
// self-parenthesizes the cast (`((Boolean)(mh.invoke(...)))`), exactly like OP_CHECKCAST, so it is safe as a
// receiver. Single-class reproducible (MethodHandle is JDK).
import java.lang.invoke.MethodHandle;

public class PolySigCastRecvSeed {
    static MethodHandle MH;

    boolean check(byte[] b) throws Throwable {
        return ((Boolean) MH.invoke(b, 0, b.length)).booleanValue();
    }
}

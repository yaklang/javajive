import java.io.InputStream;
import java.security.AccessController;
import java.security.PrivilegedAction;
import java.security.ProtectionDomain;

// Regression seed for the AccessController.doPrivileged ambiguity: a bare lambda / method-reference
// argument is applicable to BOTH PrivilegedAction<T> and PrivilegedExceptionAction<T>, so javac rejects
// the decompiled `doPrivileged(x::y)` as "reference to doPrivileged is ambiguous". The invokedynamic
// result type records the exact functional interface (PrivilegedAction), so the fix re-emits the
// source's `(PrivilegedAction) ...` cast. Real hits: fastjson2 DynamicClassLoader / JSONFactory.
// The source needs the cast to compile at all; the compiled bytecode is identical to the unadorned
// call, so it exercises exactly the decompiler's re-derivation. Compile with `--release 8`.
public class DoPrivilegedLambdaSeed {
    static final ProtectionDomain DOMAIN;
    static final InputStream STREAM;

    static {
        DOMAIN = AccessController.doPrivileged(
                (PrivilegedAction<ProtectionDomain>) DoPrivilegedLambdaSeed.class::getProtectionDomain);
        STREAM = AccessController.doPrivileged((PrivilegedAction<InputStream>) () -> {
            return DoPrivilegedLambdaSeed.class.getResourceAsStream("x");
        });
    }
}

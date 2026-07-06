// Regression seed for the Throwable-family catch-slot supertype-arm merge
// (reachingRefSlotThrowableArmMerge; kill-switch JDEC_REF_SLOT_THROWABLE_ARM_MERGE_OFF).
//
// A try/catch with two handlers writes each caught exception into ONE JVM slot; the post-catch read is a
// single logical `Throwable cause` whose type is the arms' LUB. In DFS order the InterruptedException arm
// mints the slot's var and the getCause()->Throwable arm splits off, so `cause instanceof X` /
// `(X) cause` bind to the narrow InterruptedException var and javac rejects them
// ("InterruptedException cannot be converted to ..."). The merge widens the shared ref to Throwable
// (safe: a merged catch variable's uses are Throwable-level). Mirrors spring core codec Decoder.decode.
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ExecutionException;

public class ThrowableCatchMergeSeed {
    String run(CompletableFuture<String> f) {
        Throwable cause;
        try {
            return f.get();
        } catch (ExecutionException e) {
            cause = e.getCause();
        } catch (InterruptedException e) {
            cause = e;
        }
        throw cause instanceof IllegalStateException
                ? (IllegalStateException) cause
                : new IllegalStateException(cause.getMessage(), cause);
    }
}

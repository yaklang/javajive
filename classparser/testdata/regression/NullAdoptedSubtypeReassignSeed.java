// Regression seed for the null-adopted subtype reassignment merge
// (JDEC_NULL_ADOPTED_SUBTYPE_REASSIGN_OFF + the java.io stream hierarchy in hierarchy.go).
//
// `InputStream in = null;` then `in = pick();` adopts InputStream onto the slot; a later
// `in = new GZIPInputStream(in)` on one arm stores a SUBTYPE into the same slot. Without the merge the
// null-adopt-once guard treats the subtype store as a fresh variable, so the post-merge `read(in)` is
// left unassigned on the non-wrapping path -> javac "variable might not have been initialized". Mirrors
// jsoup HttpConnection$Response.execute.
import java.io.ByteArrayInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.util.zip.GZIPInputStream;

public class NullAdoptedSubtypeReassignSeed {
    // Returns the abstract InputStream so the ternary/adopt types the slot as InputStream (the SUPERTYPE),
    // exactly like HttpConnection$Response's getErrorStream()/getInputStream() pair.
    private static InputStream pick(byte[] x) {
        return new ByteArrayInputStream(x);
    }

    int total(boolean gzip, byte[] a, byte[] b) throws IOException {
        InputStream in = null;
        in = a != null ? pick(a) : pick(b);
        if (gzip) {
            in = new GZIPInputStream(in);
        }
        int n = 0;
        int c;
        while ((c = in.read()) != -1) {
            n += c;
        }
        return n;
    }
}

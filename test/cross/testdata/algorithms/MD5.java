package bench;

// Self-hosted MD5 (RFC 1321) implemented from scratch (no java.security): a bit-manipulation-heavy
// algorithm used to prove decompile -> javac recompile -> execute fidelity. main() prints the MD5 of
// a few fixed inputs in lowercase hex; the round-trip test asserts the recompiled program prints the
// identical bytes as the original compile.
public class MD5 {
    private static final int[] S = {
            7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22,
            5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20,
            4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23,
            6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21
    };
    private static final int[] K = new int[64];

    static {
        for (int i = 0; i < 64; i++) {
            K[i] = (int) (long) (Math.abs(Math.sin(i + 1)) * 4294967296.0);
        }
    }

    public static String hash(byte[] msg) {
        int a0 = 0x67452301, b0 = 0xefcdab89, c0 = 0x98badcfe, d0 = 0x10325476;
        int newLen = ((msg.length + 8) / 64 + 1) * 64;
        byte[] padded = new byte[newLen];
        System.arraycopy(msg, 0, padded, 0, msg.length);
        padded[msg.length] = (byte) 0x80;
        long lenBits = (long) msg.length * 8L;
        for (int i = 0; i < 8; i++) {
            padded[newLen - 8 + i] = (byte) (lenBits >>> (8 * i));
        }
        for (int off = 0; off < newLen; off += 64) {
            int[] m = new int[16];
            for (int i = 0; i < 16; i++) {
                m[i] = (padded[off + i * 4] & 0xff)
                        | ((padded[off + i * 4 + 1] & 0xff) << 8)
                        | ((padded[off + i * 4 + 2] & 0xff) << 16)
                        | ((padded[off + i * 4 + 3] & 0xff) << 24);
            }
            int a = a0, b = b0, c = c0, d = d0;
            for (int i = 0; i < 64; i++) {
                int f, g;
                if (i < 16) {
                    f = (b & c) | (~b & d);
                    g = i;
                } else if (i < 32) {
                    f = (d & b) | (~d & c);
                    g = (5 * i + 1) % 16;
                } else if (i < 48) {
                    f = b ^ c ^ d;
                    g = (3 * i + 5) % 16;
                } else {
                    f = c ^ (b | ~d);
                    g = (7 * i) % 16;
                }
                f = f + a + K[i] + m[g];
                a = d;
                d = c;
                c = b;
                b = b + Integer.rotateLeft(f, S[i]);
            }
            a0 += a;
            b0 += b;
            c0 += c;
            d0 += d;
        }
        return hexLE(a0) + hexLE(b0) + hexLE(c0) + hexLE(d0);
    }

    private static String hexLE(int v) {
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < 4; i++) {
            int b = (v >>> (8 * i)) & 0xff;
            sb.append(Character.forDigit((b >>> 4) & 0xf, 16));
            sb.append(Character.forDigit(b & 0xf, 16));
        }
        return sb.toString();
    }

    public static void main(String[] args) {
        String[] inputs = {"", "abc", "The quick brown fox jumps over the lazy dog"};
        for (String s : inputs) {
            System.out.println(hash(s.getBytes()));
        }
    }
}

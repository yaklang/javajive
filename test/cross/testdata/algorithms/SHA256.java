package bench;

// Self-hosted SHA-256 (FIPS 180-4) implemented from scratch (no java.security). Exercises large static
// constant arrays, message scheduling, rotate/shift bit operations and modular addition. main() prints
// the SHA-256 of fixed inputs in lowercase hex.
public class SHA256 {
    private static final int[] K = {
            0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
            0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
            0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
            0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
            0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
            0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
            0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
            0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2
    };

    public static String hash(byte[] msg) {
        int h0 = 0x6a09e667, h1 = 0xbb67ae85, h2 = 0x3c6ef372, h3 = 0xa54ff53a;
        int h4 = 0x510e527f, h5 = 0x9b05688c, h6 = 0x1f83d9ab, h7 = 0x5be0cd19;

        int newLen = ((msg.length + 8) / 64 + 1) * 64;
        byte[] padded = new byte[newLen];
        System.arraycopy(msg, 0, padded, 0, msg.length);
        padded[msg.length] = (byte) 0x80;
        long lenBits = (long) msg.length * 8L;
        for (int i = 0; i < 8; i++) {
            padded[newLen - 1 - i] = (byte) (lenBits >>> (8 * i));
        }

        int[] w = new int[64];
        for (int off = 0; off < newLen; off += 64) {
            for (int i = 0; i < 16; i++) {
                w[i] = ((padded[off + i * 4] & 0xff) << 24)
                        | ((padded[off + i * 4 + 1] & 0xff) << 16)
                        | ((padded[off + i * 4 + 2] & 0xff) << 8)
                        | (padded[off + i * 4 + 3] & 0xff);
            }
            for (int i = 16; i < 64; i++) {
                int s0 = Integer.rotateRight(w[i - 15], 7) ^ Integer.rotateRight(w[i - 15], 18) ^ (w[i - 15] >>> 3);
                int s1 = Integer.rotateRight(w[i - 2], 17) ^ Integer.rotateRight(w[i - 2], 19) ^ (w[i - 2] >>> 10);
                w[i] = w[i - 16] + s0 + w[i - 7] + s1;
            }
            int a = h0, b = h1, c = h2, d = h3, e = h4, f = h5, g = h6, h = h7;
            for (int i = 0; i < 64; i++) {
                int bigS1 = Integer.rotateRight(e, 6) ^ Integer.rotateRight(e, 11) ^ Integer.rotateRight(e, 25);
                int ch = (e & f) ^ (~e & g);
                int t1 = h + bigS1 + ch + K[i] + w[i];
                int bigS0 = Integer.rotateRight(a, 2) ^ Integer.rotateRight(a, 13) ^ Integer.rotateRight(a, 22);
                int maj = (a & b) ^ (a & c) ^ (b & c);
                int t2 = bigS0 + maj;
                h = g;
                g = f;
                f = e;
                e = d + t1;
                d = c;
                c = b;
                b = a;
                a = t1 + t2;
            }
            h0 += a;
            h1 += b;
            h2 += c;
            h3 += d;
            h4 += e;
            h5 += f;
            h6 += g;
            h7 += h;
        }
        return hex(h0) + hex(h1) + hex(h2) + hex(h3) + hex(h4) + hex(h5) + hex(h6) + hex(h7);
    }

    private static String hex(int v) {
        StringBuilder sb = new StringBuilder();
        for (int i = 7; i >= 0; i--) {
            sb.append(Character.forDigit((v >>> (i * 4)) & 0xf, 16));
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

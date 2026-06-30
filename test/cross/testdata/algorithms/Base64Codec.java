package bench;

// Self-hosted Base64 encode + decode (RFC 4648, no java.util.Base64). Exercises byte<->char mapping,
// bit packing across 3-byte/4-char groups, padding and a reverse lookup table. main() round-trips a few
// inputs through encode then decode and prints both so the decompiler round-trip can be compared.
public class Base64Codec {
    private static final char[] ALPHABET =
            "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/".toCharArray();
    private static final int[] INV = new int[128];

    static {
        for (int i = 0; i < INV.length; i++) {
            INV[i] = -1;
        }
        for (int i = 0; i < ALPHABET.length; i++) {
            INV[ALPHABET[i]] = i;
        }
    }

    public static String encode(byte[] data) {
        StringBuilder sb = new StringBuilder();
        int i = 0;
        while (i + 3 <= data.length) {
            int n = ((data[i] & 0xff) << 16) | ((data[i + 1] & 0xff) << 8) | (data[i + 2] & 0xff);
            sb.append(ALPHABET[(n >>> 18) & 0x3f]);
            sb.append(ALPHABET[(n >>> 12) & 0x3f]);
            sb.append(ALPHABET[(n >>> 6) & 0x3f]);
            sb.append(ALPHABET[n & 0x3f]);
            i += 3;
        }
        int rem = data.length - i;
        if (rem == 1) {
            int n = (data[i] & 0xff) << 16;
            sb.append(ALPHABET[(n >>> 18) & 0x3f]);
            sb.append(ALPHABET[(n >>> 12) & 0x3f]);
            sb.append("==");
        } else if (rem == 2) {
            int n = ((data[i] & 0xff) << 16) | ((data[i + 1] & 0xff) << 8);
            sb.append(ALPHABET[(n >>> 18) & 0x3f]);
            sb.append(ALPHABET[(n >>> 12) & 0x3f]);
            sb.append(ALPHABET[(n >>> 6) & 0x3f]);
            sb.append('=');
        }
        return sb.toString();
    }

    public static byte[] decode(String s) {
        int pad = 0;
        for (int i = s.length() - 1; i >= 0 && s.charAt(i) == '='; i--) {
            pad++;
        }
        int outLen = s.length() / 4 * 3 - pad;
        byte[] out = new byte[outLen];
        int oi = 0;
        for (int i = 0; i < s.length(); i += 4) {
            int n = (INV[s.charAt(i)] << 18) | (INV[s.charAt(i + 1)] << 12);
            if (s.charAt(i + 2) != '=') {
                n |= INV[s.charAt(i + 2)] << 6;
            }
            if (s.charAt(i + 3) != '=') {
                n |= INV[s.charAt(i + 3)];
            }
            out[oi++] = (byte) ((n >>> 16) & 0xff);
            if (oi < outLen) {
                out[oi++] = (byte) ((n >>> 8) & 0xff);
            }
            if (oi < outLen) {
                out[oi++] = (byte) (n & 0xff);
            }
        }
        return out;
    }

    public static void main(String[] args) {
        String[] inputs = {"", "f", "fo", "foo", "foob", "fooba", "foobar",
                "The quick brown fox jumps over the lazy dog"};
        for (String s : inputs) {
            String enc = encode(s.getBytes());
            String dec = new String(decode(enc));
            System.out.println(enc + " | " + dec);
        }
    }
}

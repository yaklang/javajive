package bench;

// Self-hosted CRC-32 (IEEE 802.3, polynomial 0xEDB88320) with a runtime-built lookup table. Exercises
// static table initialization, nested loops and unsigned shifts. main() prints the CRC of fixed inputs.
public class CRC32 {
    private static final int[] TABLE = new int[256];

    static {
        for (int n = 0; n < 256; n++) {
            int c = n;
            for (int k = 0; k < 8; k++) {
                c = ((c & 1) != 0) ? (0xedb88320 ^ (c >>> 1)) : (c >>> 1);
            }
            TABLE[n] = c;
        }
    }

    public static long crc(byte[] data) {
        int crc = 0xffffffff;
        for (byte b : data) {
            crc = TABLE[(crc ^ b) & 0xff] ^ (crc >>> 8);
        }
        return (crc ^ 0xffffffff) & 0xffffffffL;
    }

    public static void main(String[] args) {
        String[] in = {"", "abc", "The quick brown fox jumps over the lazy dog"};
        for (String s : in) {
            System.out.println(Long.toHexString(crc(s.getBytes())));
        }
    }
}

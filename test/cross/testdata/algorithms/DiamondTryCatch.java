package bench;

import java.nio.ByteBuffer;

// If-diamond then a shared try/catch around a loop nest. Mirrors guava
// InetAddresses.textToNumericFormatV6: the condition is `skip >= 0 ? n >= 1 : n == 0`,
// then allocate + parse loops inside NumberFormatException. Round-trip must keep the
// loops on BOTH diamond arms (not leak `= Exception` into one arm).
public class DiamondTryCatch {
    static short parse(String s) {
        int v = Integer.parseInt(s, 16);
        if (v > 65535) {
            throw new NumberFormatException();
        }
        return (short) v;
    }

    public static byte[] convert(String[] parts, int skipIndex, int partsHi, int partsLo) {
        int skipped = 8 - (partsHi + partsLo);
        if (skipIndex >= 0) {
            if (skipped < 1) {
                return null;
            }
        } else {
            if (skipped != 0) {
                return null;
            }
        }
        ByteBuffer buf = ByteBuffer.allocate(16);
        try {
            for (int i = 0; i < partsHi; i++) {
                buf.putShort(parse(parts[i]));
            }
            for (int i = 0; i < skipped; i++) {
                buf.putShort((short) 0);
            }
            for (int i = partsLo; i > 0; i--) {
                buf.putShort(parse(parts[parts.length - i]));
            }
        } catch (NumberFormatException ex) {
            return null;
        }
        return buf.array();
    }

    static int fingerprint(byte[] b) {
        if (b == null) {
            return -1;
        }
        int h = 1;
        for (int i = 0; i < b.length; i++) {
            h = 31 * h + (b[i] & 0xff);
        }
        return h;
    }

    public static void main(String[] args) {
        String[] full = {"0", "1", "2", "3", "4", "5", "6", "7"};
        System.out.println("full=" + fingerprint(convert(full, -1, 8, 0)));
        String[] compressed = {"1", "", "2"};
        System.out.println("skip=" + fingerprint(convert(compressed, 1, 1, 1)));
        System.out.println("badskip=" + fingerprint(convert(full, 0, 8, 0)));
        String[] boom = {"zzzz", "1", "2", "3", "4", "5", "6", "7"};
        System.out.println("nfe=" + fingerprint(convert(boom, -1, 8, 0)));
    }
}

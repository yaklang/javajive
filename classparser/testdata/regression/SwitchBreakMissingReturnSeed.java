// Regression seed for fixSwitchBreakMissingReturn: a value-returning method whose last
// statement is a switch. javac compiles the F/f fall-through into D/d as `goto next-case`;
// the switch rewriter reconstructs that edge as `break`, so the method ends without a
// return on that path ("missing return statement"). Mirrors commons-lang3
// NumberUtils.createNumber. Recompile: javac --release 8 -d . SwitchBreakMissingReturnSeed.java
public class SwitchBreakMissingReturnSeed {
    public static Number parse(String s) {
        if (s == null) {
            return null;
        }
        char last = s.charAt(s.length() - 1);
        if (!Character.isDigit(last) && last != '.') {
            String mant = s.substring(0, s.length() - 1);
            switch (last) {
                case 'L':
                case 'l':
                    try {
                        return Long.valueOf(mant);
                    } catch (NumberFormatException e) {
                        throw new NumberFormatException(s);
                    }
                case 'F':
                case 'f':
                    try {
                        Float f = Float.valueOf(s);
                        if (!f.isInfinite() && f.floatValue() != 0.0F) {
                            return f;
                        }
                    } catch (NumberFormatException e) {
                        // fall through to D/d
                    }
                    // $FALL-THROUGH$
                case 'D':
                case 'd':
                    try {
                        return Double.valueOf(s);
                    } catch (NumberFormatException e) {
                        throw new NumberFormatException(s);
                    }
                default:
                    throw new NumberFormatException(s);
            }
        }
        return Integer.valueOf(s);
    }
}

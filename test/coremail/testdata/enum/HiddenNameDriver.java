package fixtures;

public class HiddenNameDriver {
    public static void main(String[] args) throws Exception {
        HiddenName a = (HiddenName) HiddenName.class.getField("a").get(null);
        HiddenName b = (HiddenName) HiddenName.class.getField("b").get(null);
        HiddenName[] values = HiddenName.values();
        HiddenName[] again = HiddenName.values();
        boolean missing = false;
        try { HiddenName.valueOf("a"); }
        catch (IllegalArgumentException expected) { missing = true; }
        boolean same = values.length == 2 && values[0] == a && values[1] == b && again != values && again[0] == a;
        boolean lookup = HiddenName.valueOf("OK") == a && HiddenName.valueOf("NET_ERR") == b;
        System.out.println(a.name() + ":" + b.name() + ":" + a.ordinal() + ":" + b.ordinal() + ":" + a.payload() + ":" + b.payload() + ":" + same + ":" + missing + ":" + lookup);
    }
}

package fixtures;

public class FactoryDriver {
    public static void main(String[] args) {
        FinalFactoryChoice[] values = FinalFactoryChoice.values();
        if (values.length != 3 || values[0] != FinalFactoryChoice.BINARY ||
            values[1] != FinalFactoryChoice.BYTE || values[2] != FinalFactoryChoice.TEXT)
            throw new AssertionError("constant order/identity");
        if (values[0].ordinal() != 0 || values[1].ordinal() != 1 || values[2].ordinal() != 2)
            throw new AssertionError("ordinals");
        if (!values[0].name().equals("BINARY") || !values[0].value().equals("binary"))
            throw new AssertionError("names or payload");
        if (FinalFactoryChoice.valueOf("TEXT") != values[2])
            throw new AssertionError("valueOf identity");
        values[0] = null;
        if (FinalFactoryChoice.values()[0] != FinalFactoryChoice.BINARY)
            throw new AssertionError("values must clone");
        try { FinalFactoryChoice.valueOf("missing"); throw new AssertionError("missing failure"); }
        catch (IllegalArgumentException expected) {}
        System.out.println("identity:order:ordinal:payload:clone:failure");
    }
}

public class IfaceDefaultSuperSeed implements IfaceDefaultSuper {
    public String describe() {
        return IfaceDefaultSuper.super.describe() + "-sub";
    }
}

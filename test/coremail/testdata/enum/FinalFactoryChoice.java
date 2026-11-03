package fixtures;

public enum FinalFactoryChoice {
    BINARY("binary"), BYTE("byte"), TEXT("text");
    private final String value;
    FinalFactoryChoice(String value) { this.value = value; }
    public String value() { return value; }
}

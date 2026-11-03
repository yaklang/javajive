package fixtures;

public enum HiddenName {
    OK(7), NET_ERR(8);
    private final int payload;
    HiddenName(int payload) { this.payload = payload; }
    public int payload() { return payload; }
}

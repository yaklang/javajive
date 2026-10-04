import java.io.IOException;

public final class Main {
    private static String escapeText(String value) {
        return value.replace("&", "&amp;");
    }

    private static String escapeQuotes(String value) {
        return value.replace("\"", "\\\"").replace("\n", "\\n");
    }

    private static void render(boolean useText, Object value, Appendable output) throws IOException {
        output.append('"');
        output.append(useText
                ? escapeText((String) value)
                : escapeQuotes((String) value).replace("\n", "\\n"));
        output.append('"');
    }

    public static void main(String[] args) throws Exception {
        String input = "a\"b\nc&";
        StringBuilder escapedQuotes = new StringBuilder();
        render(false, input, escapedQuotes);
        System.out.println(escapedQuotes);
        StringBuilder escapedText = new StringBuilder();
        render(true, input, escapedText);
        System.out.println(escapedText);

        StringBuilder invalidOutput = new StringBuilder();
        try {
            render(false, new Object(), invalidOutput);
        } catch (ClassCastException expected) {
            System.out.println("classcast-after-prefix:" + invalidOutput);
        }
    }
}

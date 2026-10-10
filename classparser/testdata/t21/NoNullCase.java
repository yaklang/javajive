public class NoNullCase {
	static String f(Object x) {
		return switch (x) {
			case String s -> "s:" + s;
			case Integer i -> "i:" + i;
			default -> "other";
		};
	}

	public static void main(String[] a) {
		try {
			System.out.println("null=" + f(null));
		} catch (NullPointerException e) {
			System.out.println("null=NPE");
		}
		System.out.println(f("ab"));
		System.out.println(f(2));
		System.out.println(f(2L));
	}
}

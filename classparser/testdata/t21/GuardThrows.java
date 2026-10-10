public class GuardThrows {
	static String events = "";
	static Object last;

	static Object select(Object x) {
		events += "S";
		last = x;
		return x;
	}

	static boolean g1(String s) {
		events += "1";
		return false;
	}

	static boolean g2(String s) {
		events += "2";
		throw new IllegalStateException("boom:" + s);
	}

	static String f(Object x) {
		return switch (select(x)) {
			case String s when g1(s) -> "A";
			case String s when g2(s) -> "B";
			default -> "C";
		};
	}

	public static void main(String[] a) {
		String in = "xy";
		try {
			System.out.println(f(in));
		} catch (IllegalStateException e) {
			System.out.println("ex=" + e.getMessage());
		}
		System.out.println(events);
		System.out.println(last == in);
	}
}

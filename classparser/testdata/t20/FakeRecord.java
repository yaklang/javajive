public class FakeRecord extends java.lang.Record {
	private final int x;

	public FakeRecord(int x) {
		this.x = x;
	}

	public String toString() {
		return "fake:" + x;
	}

	public int hashCode() {
		return x;
	}

	public boolean equals(Object o) {
		return o instanceof FakeRecord f && f.x == x;
	}

	public static void main(String[] a) {
		System.out.println(FakeRecord.class.isRecord());
		System.out.println(new FakeRecord(3));
	}
}

package fixtures;

public class ConstantBodyDriver {
	public static void main(String[] args) throws Exception {
		ConstantBody le = (ConstantBody) ConstantBody.class.getField("a").get(null);
		ConstantBody lt = (ConstantBody) ConstantBody.class.getField("b").get(null);
		ConstantBody plain = (ConstantBody) ConstantBody.class.getField("c").get(null);
		boolean ok = le.test(1, 1)
			&& !lt.test(1, 1)
			&& !plain.test(1, 2)
			&& le.name().equals("LE")
			&& lt.ordinal() == 1
			&& plain.ordinal() == 2
			&& ConstantBody.values().length == 3
			&& ConstantBody.values()[0] == le
			&& ConstantBody.valueOf("LE") == le
			&& ConstantBody.valueOf("PLAIN") == plain;
		if (!ok) {
			throw new AssertionError();
		}
		System.out.print("ok");
	}
}

public record SpecialValues(float f, double d, int[] arr, String ref) {
	public static void main(String[] a) {
		SpecialValues nan1 = new SpecialValues(Float.NaN, Double.NaN, null, null);
		SpecialValues nan2 = new SpecialValues(Float.NaN, Double.NaN, null, null);
		System.out.print("nanEquals=");
		System.out.println(nan1.equals(nan2));
		System.out.print("nanHash=");
		System.out.println(nan1.hashCode() == nan2.hashCode());

		SpecialValues z1 = new SpecialValues(-0.0f, -0.0d, new int[]{1}, "x");
		SpecialValues z2 = new SpecialValues(0.0f, 0.0d, new int[]{1}, "x");
		System.out.print("signedZeroEquals=");
		System.out.println(z1.equals(z2));

		int[] shared = new int[]{3, 4};
		SpecialValues a1 = new SpecialValues(1f, 1d, shared, "r");
		SpecialValues a2 = new SpecialValues(1f, 1d, shared, "r");
		SpecialValues a3 = new SpecialValues(1f, 1d, new int[]{3, 4}, "r");
		System.out.print("sameArrRef=");
		System.out.println(a1.equals(a2));
		System.out.print("arrContent=");
		System.out.println(a1.equals(a3));

		SpecialValues n1 = new SpecialValues(1f, 1d, null, null);
		SpecialValues n2 = new SpecialValues(1f, 1d, null, null);
		System.out.print("nullRef=");
		System.out.println(n1.equals(n2));
		System.out.print("isRecord=");
		System.out.println(SpecialValues.class.isRecord());
	}
}

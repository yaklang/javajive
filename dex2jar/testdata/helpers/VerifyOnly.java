/** Loads and reflects classes without running their static initializers. */
public final class VerifyOnly {
  public static void main(String[] args) throws Exception {
    if (args.length == 0) {
      throw new IllegalArgumentException("class names required");
    }
    for (String name : args) {
      Class<?> type = Class.forName(name, false, VerifyOnly.class.getClassLoader());
      type.getDeclaredMethods();
      type.getDeclaredConstructors();
      type.getDeclaredFields();
    }
    System.out.println("VERIFIED_NOT_EXECUTED");
  }
}

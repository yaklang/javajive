public class IdentCastAdv {
  Object invoke() throws Exception { return null; }
  public Object m() throws Exception {
    try {
      return invoke();
    } catch (java.lang.reflect.InvocationTargetException e) {
      if (e.getTargetException() instanceof Exception) {
        throw (Exception) e.getTargetException();
      }
      throw e;
    }
  }
}

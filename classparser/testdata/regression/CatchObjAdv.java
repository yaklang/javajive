public class CatchObjAdv {
  static java.util.logging.Logger logger = java.util.logging.Logger.getLogger("x");
  Object get() { return null; }
  public void m() {
    Object prev = null;
    try {
      prev = get();
    } catch (Exception e) {
      if (e instanceof java.security.PrivilegedActionException) {
        e = ((java.security.PrivilegedActionException) e).getException();
      }
      logger.log(java.util.logging.Level.FINE, "x", e);
    }
  }
}

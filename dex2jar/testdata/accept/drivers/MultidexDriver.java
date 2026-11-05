package accept.gate;

import accept.apk.Primary;
import accept.apk.Secondary;

/** Reviewed behavioral oracle for both accepted multidex classes. */
public final class MultidexDriver {
  public static void main(String[] args) {
    Primary.main(new String[0]);
    Secondary.main(new String[0]);
    System.out.println("multidex:" + Primary.class.getName() + ":" + Secondary.class.getName());
  }
}

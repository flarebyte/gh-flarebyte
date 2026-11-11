package fixture

import org.junit.jupiter.api.Disabled
import kotlin.test.Test
import kotlin.test.assertEquals

class MathTest {
    @Test
    fun adds() {
        assertEquals(5, add(2, 3))
    }

    @Test
    @Disabled("fixture skip")
    fun skipped() {
        assertEquals(0, add(0, 0))
    }
}

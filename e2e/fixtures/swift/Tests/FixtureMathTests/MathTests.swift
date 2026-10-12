import FixtureMath
import Testing
import XCTest

final class MathTests: XCTestCase {
  func testAddition() {
    XCTAssertEqual(FixtureMath.add(2, 3), 5)
  }

  func testSkipped() throws {
    throw XCTSkip("Fixture skip")
  }
}

@Test func modernAddition() {
  #expect(add(-2, -3) == -5)
}

@Test(.disabled("Fixture skip")) func modernSkipped() {
  #expect(Bool(false))
}

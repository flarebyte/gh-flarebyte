public func add(_ a: Int, _ b: Int) -> Int {
  a + b
}

public func uncovered(_ value: Int) -> Int {
  if value > 0 {
    return value * 2
  }
  return -value
}

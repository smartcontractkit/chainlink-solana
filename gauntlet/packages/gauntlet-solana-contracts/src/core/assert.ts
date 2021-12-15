export function assert(value: any, expected: any): boolean {
  if (Array.isArray(expected) && Array.isArray(value)) return isEqualArrays(expected, value)
  return isEqual(value, expected)
}

export const isEqual = (a, b) => new String(a).toString() === new String(b).toString()

export function isEqualArrays(a, b) {
  a = a.map((x) => JSON.stringify(x))
  b = b.map((y) => JSON.stringify(y))

  const intersection = a.filter((x) => b.includes(x))
  const difference = arrayDifference(a, b)
  return a.length === b.length && intersection.length === a.length && difference.length === 0
}

export const arrayDifference = (a: any[], b: any[]) =>
  a.filter((x) => !b.includes(x)).concat(b.filter((x) => !a.includes(x)))

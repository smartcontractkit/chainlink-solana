import assert from 'assert'

export const isDeepEqual = (a: any, b: any) => {
  try {
    assert.deepStrictEqual(a, b)
  } catch (error) {
    if (error.name === 'AssertionError') {
      return false
    }
    throw error
  }
  return true
}

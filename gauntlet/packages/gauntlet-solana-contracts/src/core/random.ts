// Python's `.sample` ported over to TS: https://stackoverflow.com/a/45556840/8605244
export function sample<T>(population: T[], k: number): T[] {
  const n = population.length

  if (k < 0 || k > n) throw new RangeError('Sample size larger than population or is negative')

  const result = new Array(k)
  let setsize = 21 // size of a small set minus size of an empty list
  if (k > 5) {
    setsize += Math.pow(4, Math.ceil(Math.log(k * 3) / Math.log(4)))
  }

  if (n <= setsize) {
    // An n-length list is smaller than a sampleSize-length set
    const pool = population.slice()
    for (let i = 0; i < k; i++) {
      // invariant:  non-selected at [0,n-i)
      const j = (Math.random() * (n - i)) | 0
      result[i] = pool[j]
      pool[j] = pool[n - i - 1] // move non-selected item into vacancy
    }
  } else {
    const selected = new Set()
    for (let i = 0; i < k; i++) {
      let j = (Math.random() * n) | 0
      while (selected.has(j)) {
        j = (Math.random() * n) | 0
      }
      selected.add(j)
      result[i] = population[j]
    }
  }

  return result
}

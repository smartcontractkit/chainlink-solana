import BN from 'bn.js'

// time constants similar to Go: https://golang.org/pkg/time/#pkg-constants
export const Nanosecond = new BN(1),
  Microsecond = Nanosecond.mul(new BN(1000)),
  Millisecond = Microsecond.mul(new BN(1000)),
  Second = Millisecond.mul(new BN(1000)),
  Minute = Second.mul(new BN(60)),
  Hour = Minute.mul(new BN(60))

export function durationToNanoseconds(str: string): BN {
  const _toBN = (n: any) => new BN(n)
  let dur = _toBN(0)
  const hours = str.match(/(\d+)\s*h/)
  const minutes = str.match(/(\d+)\s*m(?!s)/)
  const seconds = str.match(/(\d+)\s*s/)
  const milliseconds = str.match(/(\d+)\s*ms/)
  const microseconds = str.match(/(\d+)\s*µs/)
  const nanoseconds = str.match(/(\d+)\s*ns/)
  if (hours) {
    dur = dur.add(Hour.mul(_toBN(hours[1])))
  }
  if (minutes) {
    dur = dur.add(Minute.mul(_toBN(minutes[1])))
  }
  if (seconds) {
    dur = dur.add(Second.mul(_toBN(seconds[1])))
  }
  if (milliseconds) {
    dur = dur.add(Millisecond.mul(_toBN(milliseconds[1])))
  }
  if (microseconds) {
    dur = dur.add(Microsecond.mul(_toBN(microseconds[1])))
  }
  if (nanoseconds) {
    dur = dur.add(Nanosecond.mul(_toBN(nanoseconds[1])))
  }
  return dur
}

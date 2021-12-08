export const divideIntoChunks = (arr: Array<any> | Buffer, chunkSize: number): any[][] => {
  const chunks: any[] = []
  let prevIndex = 0
  while (prevIndex < arr.length) {
    chunks.push(arr.slice(prevIndex, prevIndex + chunkSize))
    prevIndex += chunkSize
  }
  return chunks
}

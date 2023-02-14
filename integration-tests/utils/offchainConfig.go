package utils

func ChunkSlice(items []byte, chunkSize int) (chunks [][]byte) {
	for chunkSize < len(items) {
		chunks = append(chunks, items[0:chunkSize])
		items = items[chunkSize:]
	}
	return append(chunks, items)
}

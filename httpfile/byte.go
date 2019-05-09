package httpfile

type ByteSize uint64

const (
	B  ByteSize = 1
	KB ByteSize = 1 << (10 * iota)
	MB
	GB
	TB
	PB
	EB
)

const (
	MinChunkSize int64 = int64(1 * MB)
)

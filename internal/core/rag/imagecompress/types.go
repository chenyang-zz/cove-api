package imagecompress

type Input struct {
	Data    []byte
	FileExt string
}

type Output struct {
	Data          []byte
	MIME          string
	Compressed    bool
	OriginalBytes int
	OutputBytes   int
}

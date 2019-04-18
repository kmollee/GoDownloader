package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

type Chunk struct {
	start int64
	end   int64
	path  string
	size  int64
}

func (c *Chunk) create() error {
	f, err := os.OpenFile(c.path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0660)
	if nil != err {
		return errors.Wrapf(err, "could not create dst file: %s", c.path)
	}
	f.Close()

	return nil
}

func (c *Chunk) isDone() (bool, error) {
	f, err := os.Stat(c.path)
	if err != nil {
		return false, errors.Wrapf(err, "could not stat file: %s", c.path)
	}
	return f.Size() == c.size, nil
}

func newChunks(path string, size int64) []*Chunk {
	chunks := make([]*Chunk, 0)

	chuckID := 0
	var start int64
	var end int64
	var receiveSize int64

	for start, chuckID = 0, 0; start < size; start, chuckID = start+MinChunkSize, chuckID+1 {

		chunkFilename := fmt.Sprintf("chunk-%d", chuckID)
		chunkPath := filepath.Join(path, chunkFilename)
		if (start + MinChunkSize) < size {
			receiveSize = MinChunkSize

		} else {
			receiveSize = size - start
		}
		end = start + receiveSize - 1
		c := &Chunk{path: chunkPath, size: receiveSize, start: start, end: end}
		chunks = append(chunks, c)
	}
	return chunks
}

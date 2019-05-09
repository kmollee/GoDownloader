package httpfile

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

type Range struct {
	start, end int64
}

type chunk struct {
	r    *Range
	path string
	size int64
	f    io.WriteCloser
}

func (c *chunk) Create() error {
	f, err := os.OpenFile(c.path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0660)
	if nil != err {
		return errors.Wrapf(err, "could not create dst file: %s", c.path)
	}
	c.f = f
	return nil
}

func (c *chunk) Close() error {
	return c.f.Close()
}

func (c *chunk) Write(r io.Reader) (int64, error) {
	return io.Copy(c.f, r)
}

func (c *chunk) isDone() (bool, error) {
	f, err := os.Stat(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, errors.Wrapf(err, "could not stat file: %s", c.path)
	}
	return f.Size() == c.size, nil
}

func newChunks(path string, size int64) []*chunk {
	chunks := make([]*chunk, 0)

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
		c := &chunk{path: chunkPath, size: receiveSize, r: &Range{start, end}}
		chunks = append(chunks, c)
	}
	return chunks
}

func downloadChunk(client *http.Client, url string, c *chunk) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return errors.Wrap(err, "could not create http request")
	}

	if c.r != nil {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", c.r.start, c.r.end))
	}

	resp, err := client.Do(req)
	if nil != err {
		return errors.Wrap(err, "could not get src file")
	}
	defer resp.Body.Close()

	err = c.Create()
	if nil != err {
		return errors.Wrap(err, "could not create dst file")
	}
	defer c.Close()

	_, err = c.Write(resp.Body)
	if err != nil {
		return errors.Wrap(err, "could not copy download content into dst file")
	}

	return nil
}

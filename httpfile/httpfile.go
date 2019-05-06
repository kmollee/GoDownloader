package httpfile

import (
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"os"

	"github.com/pkg/errors"
)

const (
	dirMode = 0755
)

// HTTPFile :describe http remote file
type HTTPFile struct {
	Client *http.Client
	URL    string
	Size   int
	Range  bool

	store  string
	chunks []*chunk
	worker int
}

func NewHTTPFile(c *http.Client, url string, storeRoot string) (*HTTPFile, error) {
	res, err := c.Head(url)
	if err != nil {
		return nil, errors.Wrap(err, "could not get url")
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not get resource on url: %s", url)
	}

	content, ok := res.Header["Accept-Ranges"]
	var isAcceptRange bool
	if ok && len(content) > 0 && content[0] != "none" {
		isAcceptRange = true
	}

	var (
		hashID    uint32
		storePath string
	)

	hashID = hash(url)
	storePath = fmt.Sprintf("%s/%d", storeRoot, hashID)

	var chunks []*chunk
	if isAcceptRange {
		// if more than one chunk, create dir then create chunks
		err = createDir(storePath)
		if err != nil {
			return nil, errors.Wrap(err, "could not create cache dir")
		}
		chunks = newChunks(storePath, res.ContentLength)
	} else {
		// if only one chunk, create single file chunk instead
		chunks = []*chunk{&chunk{path: storePath, size: res.ContentLength}}
	}

	return &HTTPFile{
		Client: c,
		URL:    url,
		chunks: chunks,
		Size:   len(chunks),
		worker: 1,
		store:  storePath,
		Range:  isAcceptRange,
	}, nil
}

// Download :download chunks
func (h *HTTPFile) Download() (chan struct{}, chan error) {
	errs := make(chan error)
	finish := make(chan struct{})

	chunks := make(chan *chunk)

	// worker: consumer
	for i := 0; i < h.worker; i++ {
		go func() {
			for c := range chunks {
				done, err := c.isDone()
				if err != nil {
					errs <- err
					return
				}

				if !done {
					err := downloadChunk(h.Client, h.URL, c)
					if err != nil {
						errs <- err
						return
					}
				}

				finish <- struct{}{}
			}
		}()
	}

	// producer
	go func() {
		for _, c := range h.chunks {
			chunks <- c

		}
		close(chunks)
	}()

	return finish, errs
}

// SaveTo :merge chunks and save to dst
func (h *HTTPFile) SaveTo(dst string) error {
	// TODO: check dst is not exist, ok to write

	if h.Size == 1 {
		// just move chunk to dst, no need merge
		c := h.chunks[0]
		return os.Rename(c.path, dst)
	}

	// merge chunks into target file
	// output file
	of, err := os.Create(dst)
	if err != nil {
		return errors.Wrapf(err, "could not create output file: %s", dst)
	}
	defer of.Close()

	// concate chunks into one
	for _, c := range h.chunks {
		cf, err := os.Open(c.path)
		if err != nil {
			return errors.Wrapf(err, "could not open chunk:%s to merge", c.path)
		}
		_, err = io.Copy(of, cf)
		if err != nil {
			return errors.Wrapf(err, "could not write chunk:%s into merge file", c.path)
		}
		cf.Close()
	}
	return nil
}

// Clean :remove all cache chunks and dir
func (h *HTTPFile) Clean() error {
	return os.RemoveAll(h.store)
}

func (h *HTTPFile) SetWorker(n int) error {
	if n < 1 {
		return fmt.Errorf("worker must larger or equal to 1")
	}

	h.worker = n
	return nil

}

func hash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

func createDir(p string) error {

	if _, err := os.Stat(p); os.IsNotExist(err) {
		return os.Mkdir(p, dirMode)
	}
	return nil
}

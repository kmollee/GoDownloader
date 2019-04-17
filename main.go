package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/pkg/errors"
)

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
	tmpPath            = "./.godownloadtmp"
)

func main() {
	url := flag.String("u", "", "the url to download")
	output := flag.String("o", "", "output path")
	worker := flag.Int("w", 6, "worker to download")
	flag.Parse()

	fmt.Printf("url: %s output: %s", *url, *output)

	client := http.DefaultClient

	size, isRangeAccept, err := fetchResourceLength(client, *url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERR: %v", err)
		os.Exit(1)
	}

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	_ = cancel

	if isRangeAccept {

		chunks := newChunks(size)

		m := MultiDownloader{client: client, worker: *worker, ctx: ctx, chunks: chunks, url: *url, chunk: make(chan *Chunk), done: make(chan struct{}), err: make(chan error)}
		err := m.Start()
		if err != nil {
			log.Fatal(err)
		}
		// go m.Display()

	} else {
		log.Println("not accept multiple downloader, start single downloader to downloader file")
		if err := downloadFile(*client, *url, *output, ""); err != nil {
			log.Fatal(err)
		}
	}

}

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

	log.Printf("compare size: %v want %v", f.Size(), c.size)
	return f.Size() == c.size, nil
}

func newChunks(size int64) []*Chunk {
	log.Printf("start create chunk")
	chunks := make([]*Chunk, 0)

	chuckID := 0
	var start int64
loop:
	for size > 0 {
		chunkPath := fmt.Sprintf("chunk-%d", chuckID)

		if size < MinChunkSize {
			c := &Chunk{path: chunkPath, size: size, start: start, end: start + size}
			chunks = append(chunks, c)
			break loop
		}
		c := &Chunk{path: chunkPath, size: MinChunkSize, start: start, end: start + MinChunkSize}
		chunks = append(chunks, c)

		chuckID++
		start += MinChunkSize
		size -= MinChunkSize
	}

	return chunks

}

// func defaultCheckRedirect(req *Request, via []*Request) error {
// 	if len(via) >= 10 {
// 		return errors.New("stopped after 10 redirects")
// 	}
// 	return nil
// }

func fetchResourceLength(client *http.Client, url string) (int64, bool, error) {
	log.Printf("URL: %v", url)
	res, err := client.Head(url)
	if err != nil {
		return 0, false, errors.Wrap(err, "could not get url")
	}
	// switch res.StatusCode {
	// case http.StatusSeeOther:
	// 	return 0, errors.Wrap(err, "could not get content")
	// case http.StatusMovedPermanently, http.StatusTemporaryRedirect, http.StatusPermanentRedirect, http.StatusFound:
	if res.StatusCode != http.StatusOK {
		return 0, false, fmt.Errorf("could not get resource")
	}

	content, ok := res.Header["Accept-Ranges"]
	var isAcceptRange bool
	if ok && len(content) > 0 && content[0] != "none" {
		isAcceptRange = true
	}
	return res.ContentLength, isAcceptRange, nil

}

type MultiDownloader struct {
	client *http.Client
	worker int

	url string

	chunks []*Chunk
	chunk  chan *Chunk
	err    chan error

	done      chan struct{}
	doneCount int

	ctx context.Context
}

func (m *MultiDownloader) startWorker() {

	for c := range m.chunk {
		log.Printf("start receive chunk")

		log.Printf("create chunk")
		if err := c.create(); err != nil {
			m.err <- err
			return
		}

		log.Printf("check chunk")
		done, err := c.isDone()
		if err != nil {
			m.err <- err
			return
		}

		if done {
			m.done <- struct{}{}
			continue
		}

		byterange := fmt.Sprintf("%d-%d", c.start, c.end)
		log.Printf(">>>> download chunk %s", byterange)
		if err := downloadFile(*m.client, m.url, c.path, byterange); err != nil {
			m.err <- err
			return
		}
		m.done <- struct{}{}
	}

}

func (m *MultiDownloader) startFeeder() {
	for i := range m.chunks {
		m.chunk <- m.chunks[i]
	}
}

func (m *MultiDownloader) mergeChunks() {
	log.Printf("merge chunks")
	// for _, c := range m.chunks {
	// 	m.chunk <- c
	// }
}

func (m *MultiDownloader) Start() error {
	log.Printf("chunks: %v", m.chunks)
	log.Printf("start worker")

	if m.worker > len(m.chunks) {
		m.worker = len(m.chunks)
	}

	for i := 0; i < m.worker; i++ {
		go m.startWorker()
	}
	log.Printf("start feeder")
	go m.startFeeder()

	log.Printf("start wait all finish")

loop:
	for {
		select {
		case <-m.done:
			m.doneCount++
			if m.doneCount == len(m.chunks) {
				break loop
			}
		case err := <-m.err:
			close(m.chunk)
			return err
		}
	}

	m.mergeChunks()

	return nil
}

func downloadFile(client http.Client, src, dst, rang string) error {
	log.Printf("start download %s to %s [%s]", src, dst, rang)

	req, err := http.NewRequest(http.MethodGet, src, nil)
	if err != nil {
		return errors.Wrap(err, "could not create http request")
	}

	// TODO: using regex examine format correct
	if len(rang) > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%s", rang))
	}

	resp, err := client.Do(req)
	if nil != err {
		return errors.Wrap(err, "could not get src file")
	}
	defer resp.Body.Close()

	f, err := os.Create(dst)
	if nil != err {
		return errors.Wrap(err, "could not create dst file")
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	if err != nil {
		return errors.Wrap(err, "could not copy download content into dst file")
	}

	return nil
}

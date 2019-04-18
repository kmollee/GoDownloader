package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/cheggaaa/pb.v1"
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
)

type multiDownloader struct {
	client *http.Client
	worker int

	url string
	dst string

	chunks []*Chunk
	chunk  chan *Chunk
	err    chan error

	done      chan struct{}
	doneCount int

	ctx context.Context

	bar *pb.ProgressBar
}

func newMiltiDownloader(ctx context.Context, client *http.Client, worker int, url string, dst string, chunks []*Chunk) *multiDownloader {

	bar := pb.StartNew(len(chunks))
	bar.SetRefreshRate(time.Second)

	bar.ShowTimeLeft = false

	m := &multiDownloader{
		client: client,
		worker: worker,
		ctx:    ctx,
		chunks: chunks,
		url:    url,
		dst:    dst,
		chunk:  make(chan *Chunk),
		done:   make(chan struct{}),
		err:    make(chan error),
		bar:    bar,
	}
	return m
}

func (m *multiDownloader) startWorker() {

	for {
		select {
		case <-m.ctx.Done():
			return
		case c := <-m.chunk:
			if err := c.create(); err != nil {
				m.err <- err
				return
			}

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
			if err := downloadFile(*m.client, m.url, c.path, byterange); err != nil {
				m.err <- err
				return
			}
			m.done <- struct{}{}
		}

	}

}

func (m *multiDownloader) startFeeder() {

	for i := range m.chunks {
		m.chunk <- m.chunks[i]
	}

}

func (m *multiDownloader) mergeChunks() error {
	f, err := os.Create(m.dst)
	if err != nil {
		return errors.Wrapf(err, "could not create output file: %s", m.dst)
	}
	defer f.Close()

	for _, c := range m.chunks {
		cf, err := os.Open(c.path)
		if err != nil {
			return errors.Wrapf(err, "could not open chunk:%s to merge", c.path)
		}
		_, err = io.Copy(f, cf)
		if err != nil {
			return errors.Wrapf(err, "could not write chunk:%s into merge file", c.path)
		}

		cf.Close()
	}
	return nil
}

func (m *multiDownloader) Start() error {
	if m.worker > len(m.chunks) {
		m.worker = len(m.chunks)
	}

	for i := 0; i < m.worker; i++ {
		go m.startWorker()
	}
	go m.startFeeder()

loop:
	for {
		select {
		case <-m.done:
			m.bar.Increment()
			m.doneCount++
			if m.doneCount == len(m.chunks) {
				m.bar.Finish()
				break loop
			}
		case <-m.ctx.Done():
			return fmt.Errorf("got cancel during downloading")
		case err := <-m.err:
			return err
		}
	}

	if err := m.mergeChunks(); err != nil {
		return err
	}

	return nil
}

func downloadFile(client http.Client, src, dst, rang string) error {
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

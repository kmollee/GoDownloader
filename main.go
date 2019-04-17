package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/pkg/errors"
)

func main() {
	url := flag.String("u", "", "the url to download")
	output := flag.String("o", "", "output path")
	flag.Parse()

	fmt.Printf("url: %s output: %s", *url, *output)

	c := http.Client{}
	_, isRangeAccept, err := fetchResourceLength(c, *url)
	log.Printf("isRangeAccept: %v", isRangeAccept)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERR: %v", err)
		os.Exit(1)
	}

	if isRangeAccept {

	} else {
		log.Println("not accept multiple downloader, start single downloader to downloader file")
		if err := downloadFile(c, *url, *output, ""); err != nil {
			log.Fatal(err)
		}
	}

}

// func defaultCheckRedirect(req *Request, via []*Request) error {
// 	if len(via) >= 10 {
// 		return errors.New("stopped after 10 redirects")
// 	}
// 	return nil
// }

func fetchResourceLength(client http.Client, url string) (int64, bool, error) {
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

const MinChunkSize = 100000

type MultiDownloader struct {
	c       http.Client
	worker  int
	src     string
	dst     string
	job     chan int64
	ctx     context.Context
	tmpfile []string
	chunk   int64
	lock    sync.Mutex
}

// func (m *MultiDownloader) Start() error {
// 	for i := 0; i < m.worker; i++ {
// 		go func(ctx context.Context, job <-chan int64) {
// 			for j := range job {
// 			}
// 		}(m.ctx, m.job)
// 	}
// 	return nil
// }

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

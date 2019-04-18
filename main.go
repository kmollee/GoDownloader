package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/pkg/errors"
)

const (
	tmpDir     = ".godownloader"
	tmpDirMode = 0744
)

func main() {
	url := flag.String("u", "", "the url to download")
	output := flag.String("o", "", "output path")
	worker := flag.Int("w", 6, "worker to download")
	flag.Parse()

	if len(*url) == 0 || len(*output) == 0 {
		fmt.Fprintf(os.Stderr, "%s -u <<url>> -o <<output path>>", os.Args[0])
		os.Exit(1)
	}

	fmt.Fprintf(os.Stdout, "download from %s => %s\n", *url, *output)

	// to change the flags on the default logger
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	client := http.DefaultClient

	size, isRangeAccept, err := fetchResourceLength(client, *url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not fetch resource: %v", err)
		os.Exit(1)
	}

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	if isRangeAccept {

		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		go func() {
			<-c
			cancel()
		}()

		if err := createDir(tmpDir, tmpDirMode); err != nil {
			log.Fatalf("could not create tmp dir '%s': %v", tmpDir, err)
		}
		chunks := newChunks(tmpDir, size)
		m := newMiltiDownloader(ctx, client, *worker, *url, *output, chunks)
		err := m.Start()
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not download due error: %v", err)
			os.Exit(1)
		}
		if err := removeDir(tmpDir); err != nil {
			log.Fatalf("could not remove tmp dir '%s': %v", tmpDir, err)
		}

		fmt.Fprintf(os.Stdout, "finish download file: %s", *output)
	} else {
		log.Println("not accept multiple downloader, start single downloader to downloader file")
		if err := downloadFile(*client, *url, *output, ""); err != nil {
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

func fetchResourceLength(client *http.Client, url string) (int64, bool, error) {
	res, err := client.Head(url)
	if err != nil {
		return 0, false, errors.Wrap(err, "could not get url")
	}

	if res.StatusCode != http.StatusOK {
		return 0, false, fmt.Errorf("could not get resource on url: %s", url)
	}

	content, ok := res.Header["Accept-Ranges"]
	var isAcceptRange bool
	if ok && len(content) > 0 && content[0] != "none" {
		isAcceptRange = true
	}
	return res.ContentLength, isAcceptRange, nil

}

func createDir(path string, mode os.FileMode) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.Mkdir(path, mode)
	}
	return nil
}

func removeDir(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(path)
}

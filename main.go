package main

import (
	"flag"
	"fmt"
	"godownloader/httpfile"
	"log"
	"net/http"
	"os"
	"os/user"
	"path"
	"strings"
	"time"

	pb "gopkg.in/cheggaaa/pb.v1"
)

const (
	BaseDir     = ".godownloader"
	BaseDirMode = 0755
)

// below variable assign by compiler
var (
	Version string
	Build   string
)

func main() {
	url := flag.String("u", "", "the url to download")
	output := flag.String("o", "", "output path")
	worker := flag.Int("w", 6, "worker to download")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Version %s\n", Version)
		fmt.Fprintf(os.Stderr, "Build %s\n", Build)
		fmt.Fprintln(os.Stderr, "usage:")
		flag.PrintDefaults()
	}
	flag.Parse()

	// setup source and destination path
	var src string
	if len(*url) == 0 {
		if flag.NArg() == 0 {
			fmt.Fprintf(os.Stderr, "%s <<url>>", os.Args[0])
			os.Exit(1)
		}
		src = flag.Arg(0)
	} else {
		src = *url
	}

	var dst string
	if len(*output) == 0 {
		dst = subLastSlash(src)
	} else {
		dst = *output
	}

	// setup base dir
	home, err := getUserHome()
	failOnErr(err)

	dir := path.Join(home, BaseDir)
	err = createDir(dir)
	failOnErr(err)

	// create file client
	client := http.DefaultClient
	h, err := httpfile.NewHTTPFile(client, src, dir)
	failOnErr(err)

	if h.Range {
		err = h.SetWorker(*worker)
		failOnErr(err)
	}

	// download chuncks
	fmt.Fprintf(os.Stdout, "start download %s", src)
	chuncks, errs := h.Download()
	failOnErr(err)

	bar := pb.StartNew(h.Size)
	bar.SetRefreshRate(time.Second)
	bar.ShowTimeLeft = false

	var count int
loop:
	for {
		select {
		case <-chuncks:

			bar.Increment()
			count++

			if count == h.Size {
				bar.Finish()
				break loop
			}
		case err := <-errs:
			log.Fatal(err)
			return
		}
	}

	// merge chunks and save
	fmt.Fprintf(os.Stdout, "save to %s\n", dst)
	err = h.SaveTo(dst)
	failOnErr(err)

	// clean cache
	err = h.Clean()
	failOnErr(err)
}

func failOnErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func createDir(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.Mkdir(dir, BaseDirMode)
	}
	return nil
}

func getUserHome() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	return usr.HomeDir, nil
}

func subLastSlash(str string) string {
	index := strings.LastIndex(str, "/")
	if index != -1 {
		return str[index+1:]
	}
	return ""
}

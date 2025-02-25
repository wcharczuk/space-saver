package main

import (
	"crypto/sha512"
	"io"
	"os"
	"path/filepath"
)

func main() {

	filepath.Walk
}

// use https://www.manpagez.com/man/2/clonefile/

func checksumFile(path string) (checksum []byte, err error) {
	var f *os.File
	f, err = os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	h := sha512.New()
	if _, err = io.Copy(h, f); err != nil {
		return
	}
	checksum = h.Sum(nil)
	return
}

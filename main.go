package main

import (
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/sys/unix"
)

var (
	flagMinSize = flag.String("min-size", "", "The minimum filesize to consider")
	flagVerbose = flag.Bool("verbose", false, "If we should show verbose output")
	flagReal    = flag.Bool("real", false, "If we should perform the dedupe")
)

func main() {
	flag.Parse()
	if len(flag.Args()) == 0 {
		fmt.Fprintln(os.Stderr, "Please provide a target directory")
		os.Exit(1)
	}
	if len(flag.Args()) > 1 {
		fmt.Fprintln(os.Stderr, "Please provide a single target directory")
		os.Exit(1)
	}

	minSizeBytes, err := parseFileSizeOrDefault(*flagMinSize, 5*(1<<20))
	fmt.Fprintf(os.Stdout, "using min size bytes: %d\n", minSizeBytes)
	var hashes = make(map[string][]fullFileInfo)
	targetDir := flag.Args()[0]
	err = filepath.Walk(targetDir, filepath.WalkFunc(func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if info.Size() < minSizeBytes {
			return nil
		}
		cs, err := checksumFile(path)
		if err != nil {
			return err
		}
		for _, existing := range hashes[cs] {
			if os.SameFile(info, existing) {
				return nil
			}
		}
		hashes[cs] = append(hashes[cs], fullFileInfo{Path: filepath.Join(path, info.Name()), FileInfo: info})
		return nil
	}))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to process target directory; %v\n", err)
		os.Exit(1)
	}
	var totalPossibleSavingsBytes uint64
	for _, fileset := range hashes {
		if len(fileset) < 2 {
			continue
		}
		slices.SortFunc(fileset, func(a, b fullFileInfo) int {
			if a.ModTime().Before(b.ModTime()) {
				return -1
			}
			if a.ModTime().Equal(b.ModTime()) {
				return 0
			}
			return 1
		})

		srcFile := fileset[0]
		for _, fileInfo := range fileset[1:] {
			totalPossibleSavingsBytes += uint64(fileInfo.Size())
			if *flagReal {
				fmt.Fprintf(os.Stdout, "%s => %s (%s)\n", srcFile.Path, fileInfo.Path, formatFileSize(uint64(fileInfo.Size())))
				if err := cloneFile(srcFile.Path, fileInfo.Path); err != nil {
					fmt.Fprintf(os.Stderr, "clone file error: %v\n", err)
					os.Exit(1)
				}
			} else {
				fmt.Fprintf(os.Stdout, "[DRY-RUN] %s => %s (%s)\n", srcFile.Path, fileInfo.Path, formatFileSize(uint64(fileInfo.Size())))
			}
		}
	}
	fmt.Fprintf(os.Stdout, "Total savings: %s\n", formatFileSize(totalPossibleSavingsBytes))
}

type fullFileInfo struct {
	fs.FileInfo
	Path string
}

func cloneFile(source, target string) error {
	if err := unix.Clonefile(source, target, unix.CLONE_NOFOLLOW); err != nil {
		if !errors.Is(err, unix.ENOTSUP) && !errors.Is(err, unix.EXDEV) {
			return fmt.Errorf("clonefile failed: %w", err)
		}
	}
	return nil
}

func checksumFile(path string) (checksum string, err error) {
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
	checksum = hex.EncodeToString(h.Sum(nil))
	return
}

func parseFileSizeOrDefault(rawFileSize string, defaultValue int64) (int64, error) {
	if rawFileSize == "" {
		return defaultValue, nil
	}
	value, unit, err := leadingInt(rawFileSize)
	if err != nil {
		return 0, err
	}
	unitFactor, ok := units[strings.ToLower(strings.TrimSpace(unit))]
	if !ok {
		return 0, fmt.Errorf("parse filesize: invalid unit; %v", unit)
	}
	return int64(value) * int64(unitFactor), nil
}

var units = map[string]int64{
	"gib": 1 << 30,
	"gb":  1 << 30,
	"mib": 1 << 20,
	"mb":  1 << 20,
	"kib": 1 << 10,
	"kb":  1 << 10,
	"b":   1,
}

// leadingInt consumes the leading [0-9]* from s.
func leadingInt[bytes []byte | string](s bytes) (x uint64, rem bytes, err error) {
	i := 0
	for ; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			break
		}
		if x > 1<<63/10 {
			// overflow
			return 0, rem, errLeadingInt
		}
		x = x*10 + uint64(c) - '0'
		if x > 1<<63 {
			// overflow
			return 0, rem, errLeadingInt
		}
	}
	return x, s[i:], nil
}

var errLeadingInt = errors.New("parse filesize: bad [0-9]*") // never printed

func formatFileSize(sizeBytes uint64) string {
	if sizeBytes >= 1<<30 {
		return fmt.Sprintf("%dGiB", sizeBytes/(1<<30))
	} else if sizeBytes >= 1<<20 {
		return fmt.Sprintf("%dMiB", sizeBytes/(1<<20))
	} else if sizeBytes >= 1<<10 {
		return fmt.Sprintf("%dKiB", sizeBytes/(1<<10))
	}
	return fmt.Sprintf("%dB", sizeBytes)
}

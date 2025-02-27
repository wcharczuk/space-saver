package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/urfave/cli/v3"
	"github.com/wcharczuk/space-saver/pkg/filesize"
	"golang.org/x/sys/unix"
)

func main() {
	if err := commandRoot.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

var commandRoot = &cli.Command{
	Name:  "space-saver",
	Usage: "Space Saver finds duplicate files and saves space on disk by cloning them.",
	Commands: []*cli.Command{
		commandFindDuplicates,
		commandCloneDuplicates,
		commandCloneFile,
		commandSameFile,
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		cli.ShowAppHelp(cmd)
		return nil
	},
}

var commandFindDuplicates = &cli.Command{
	Name:      "find",
	Usage:     "Find duplicate files by comparing sha256 hashes.",
	ArgsUsage: "[TARGET_DIR]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "min-size",
			Value: "5MiB",
			Usage: "The minimum filesize (in kubernetes size format, e.g. 4500MiB)",
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		if !c.Args().Present() {
			return fmt.Errorf("Must provide a TARGET_DIR")
		}
		if len(c.Args().Slice()) > 1 {
			return fmt.Errorf("Must only provide a TARGET_DIR")
		}
		minSizeBytes, err := filesize.Parse(c.String("min-size"))
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Using min size bytes: %v\n", c.String("min-size"))
		targetDir := c.Args().First()
		hashes, err := findDuplicateFiles(targetDir, minSizeBytes)
		if err != nil {
			return err
		}
		var totalPossibleSavingsBytes uint64
		for _, fileset := range hashes {
			if len(fileset) < 2 {
				continue
			}
			srcFile := fileset[0]
			for _, fileInfo := range fileset[1:] {
				totalPossibleSavingsBytes += uint64(fileInfo.Size())
				fmt.Fprintf(os.Stdout, "%s is a duplicate of %s (%s)\n", truncateStringPrefix(fileInfo.Path, 32), truncateStringPrefix(srcFile.Path, 32), filesize.Format(uint64(fileInfo.Size())))
			}
		}
		fmt.Fprintf(os.Stdout, "Total savings: %s\n", filesize.FormatFraction(totalPossibleSavingsBytes))
		return nil
	},
}

var commandCloneDuplicates = &cli.Command{
	Name:      "clone-duplicates",
	Usage:     "Clone duplicate files by comparing sha256 hashes and replacing them with cloned files.",
	ArgsUsage: "[TARGET_DIR]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "min-size",
			Usage: "The minimum filesize (in kubernetes size format, e.g. 4500MiB)",
			Value: "5MiB",
		},
		&cli.BoolFlag{
			Name:  "real",
			Usage: "If we should proceed with replacing duplicate files with cloned files",
			Value: false,
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		if !c.Args().Present() {
			return fmt.Errorf("Must provide a TARGET_DIR")
		}
		if len(c.Args().Slice()) > 1 {
			return fmt.Errorf("Must only provide a TARGET_DIR")
		}
		minSizeBytes, err := filesize.Parse(c.String("min-size"))
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Using min size bytes: %v\n", c.String("min-size"))
		targetDir := c.Args().First()
		hashes, err := findDuplicateFiles(targetDir, minSizeBytes)
		if err != nil {
			return err
		}
		var totalPossibleSavingsBytes uint64
		real := c.Bool("real")
		for _, fileset := range hashes {
			if len(fileset) < 2 {
				continue
			}
			srcFile := fileset[0]
			for _, fileInfo := range fileset[1:] {
				totalPossibleSavingsBytes += uint64(fileInfo.Size())
				if real {
					if err := cloneFile(srcFile.Path, fileInfo.Path); err != nil {
						return err
					}
					fmt.Fprintf(os.Stdout, "Cloned %s to %s\n", truncateStringPrefix(srcFile.Path, 64), truncateStringPrefix(fileInfo.Path, 64))
				} else {
					fmt.Fprintf(os.Stdout, "[DRY-RUN] Would clone %s to %s\n", truncateStringPrefix(srcFile.Path, 64), truncateStringPrefix(fileInfo.Path, 64))
				}
			}
		}
		fmt.Fprintf(os.Stdout, "Total savings: %s\n", filesize.FormatFraction(totalPossibleSavingsBytes))
		return nil
	},
}

var commandCloneFile = &cli.Command{
	Name:      "clone-file",
	Usage:     "Clone an indivdiual file.",
	ArgsUsage: "[SOURCE_FILE] [DEST_FILE]",
	Action: func(ctx context.Context, c *cli.Command) error {
		if !c.Args().Present() {
			return fmt.Errorf("must provide a source an destination")
		}
		sourceFile := c.Args().Get(0)
		destFile := c.Args().Get(1)
		fmt.Fprintf(os.Stdout, "Cloning %s to %s\n", truncateStringPrefix(sourceFile, 32), truncateStringPrefix(destFile, 32))
		if err := cloneFile(sourceFile, destFile); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Cloning %s to %s done!\n", truncateStringPrefix(sourceFile, 32), truncateStringPrefix(destFile, 32))
		return nil
	},
}

var commandSameFile = &cli.Command{
	Name:      "same-file",
	Usage:     "Test if two files are the same (i.e. one is a clone of the other)",
	ArgsUsage: "[SOURCE_FILE] [DEST_FILE]",
	Action: func(ctx context.Context, c *cli.Command) error {
		if !c.Args().Present() {
			return fmt.Errorf("Must provide [SOURCE_FILE] and [DEST_FILE].")
		}
		if len(c.Args().Slice()) != 2 {
			return fmt.Errorf("Must provide exactly [SOURCE_FILE] and [DEST_FILE].")
		}
		sourceFile := c.Args().Get(0)
		destFile := c.Args().Get(1)

		sourceInfo, err := os.Stat(sourceFile)
		if err != nil {
			fmt.Fprintln(os.Stdout, "[SOURCE_FILE] is missing")
			return nil
		}
		destInfo, err := os.Stat(destFile)
		if err != nil {
			fmt.Fprintln(os.Stdout, "[DEST_FILE] is missing")
			return nil
		}
		if os.SameFile(sourceInfo, destInfo) {
			fmt.Fprintln(os.Stdout, "Files are the same!")
			return nil
		}
		return fmt.Errorf("Files are not the same!")
	},
}

func truncateStringPrefix(s string, length int) string {
	if len(s) < length {
		return s
	}
	return "..." + string([]rune(s)[length:])
}

func findDuplicateFiles(targetPath string, minSizeBytes uint64) (hashes map[string][]fullFileInfo, err error) {
	hashes = make(map[string][]fullFileInfo)
	err = filepath.Walk(targetPath, filepath.WalkFunc(func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if uint64(info.Size()) < minSizeBytes {
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
		ffi := fullFileInfo{Path: path, FileInfo: info}
		if seenFiles, ok := hashes[cs]; ok {
			hashes[cs] = insertSorted(seenFiles, ffi, func(a, b fullFileInfo) int {
				if a.ModTime().Before(b.ModTime()) {
					return -1
				}
				if a.ModTime().Equal(b.ModTime()) {
					return 0
				}
				return 1
			})
		} else {
			hashes[cs] = []fullFileInfo{ffi}
		}
		return nil
	}))
	return
}

type fullFileInfo struct {
	fs.FileInfo
	Path string
}

func cloneFile(source, target string) error {
	sourceAbsolute, err := filepath.Abs(source)
	if err != nil {
		return fmt.Errorf("clone-file failed: unable to make source path absolute; %w", err)
	}
	targetAbsolute, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("clone-file failed: unable to make target path absolute; %w", err)
	}
	if !fileExists(sourceAbsolute) {
		return fmt.Errorf("clone-file failed: source not found; %s", sourceAbsolute)
	}
	targetExists := fileExists(targetAbsolute)
	if targetExists {
		_ = os.Remove(targetAbsolute)
	}
	if err := unix.Clonefile(sourceAbsolute, targetAbsolute, unix.CLONE_NOFOLLOW); err != nil {
		if !errors.Is(err, unix.ENOTSUP) && !errors.Is(err, unix.EXDEV) {
			return fmt.Errorf("clone-file failed: %w", err)
		}
	}
	return nil
}

func fileExists(target string) bool {
	_, err := os.Stat(target)
	return err == nil
}

func checksumFile(path string) (checksum string, err error) {
	var f *os.File
	f, err = os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return
	}
	checksum = hex.EncodeToString(h.Sum(nil))
	return
}

func insertSorted[A any](working []A, v A, sorter func(A, A) int) []A {
	insertAt, _ := slices.BinarySearchFunc(working, v, sorter)
	working = append(working, v)
	copy(working[insertAt+1:], working[insertAt:])
	working[insertAt] = v
	return working
}

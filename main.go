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

var commandRoot = &cli.Command{
	Name:  "space-saver",
	Usage: "Space Saver finds duplicate files and saves space on disk by cloning them.",
	Commands: []*cli.Command{
		commandFindDuplicates,
		commandCloneDuplicates,
		commandCloneFile,
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
		minSizeBytes, err := filesize.Parse(c.String("min-size"))
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "using min size bytes: %d\n", minSizeBytes)
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
				fmt.Fprintf(os.Stdout, "%s is a duplicate of %s (%s)\n", truncatePrefix(fileInfo.Path, 32), truncatePrefix(srcFile.Path, 32), filesize.Format(uint64(fileInfo.Size())))
			}
		}
		fmt.Fprintf(os.Stdout, "Total savings: %s\n", filesize.Format(totalPossibleSavingsBytes))
		return nil
	},
}

var commandCloneDuplicates = &cli.Command{
	Name:      "clone-duplicates",
	Usage:     "Clone duplicate files by comparing sha256 hashes and replacing them with cloned files.",
	ArgsUsage: "[TARGET_DIR]",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name: "TARGET_DIR",
			Min:  1,
			Max:  1,
		},
	},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "min-size",
			Usage: "The minimum filesize (in kubernetes size format, e.g. 4500MiB)",
		},
		&cli.BoolFlag{
			Name:  "real",
			Usage: "If we should proceed with replacing duplicate files with cloned files",
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		if !c.Args().Present() {
			return fmt.Errorf("Must provide a TARGET_DIR")
		}
		minSizeBytes, err := filesize.Parse(c.String("min-size"))
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "using min size bytes: %d\n", minSizeBytes)
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
				if real {
					if err := cloneFile(srcFile.Path, fileInfo.Path); err != nil {
						return err
					}
					fmt.Fprintf(os.Stdout, "Cloned %s to %s\n", truncatePrefix(srcFile.Path, 32), truncatePrefix(fileInfo.Path, 32))
				} else {
					fmt.Fprintf(os.Stdout, "%s is a duplicate of %s (%s)\n", truncatePrefix(fileInfo.Path, 32), truncatePrefix(srcFile.Path, 32), filesize.Format(uint64(fileInfo.Size())))
				}
			}
		}
		fmt.Fprintf(os.Stdout, "Total savings: %s\n", filesize.Format(totalPossibleSavingsBytes))
		return nil
	},
}

func truncatePrefix(s string, length int) string {
	if len(s) < length {
		return s
	}
	return "..." + string([]rune(s)[length:])
}

var commandCloneFile = &cli.Command{
	Name:      "clone-file",
	Usage:     "Clone an indivdiual file.",
	ArgsUsage: "[SOURCE_FILE] [DEST_FILE]",
	// Arguments: []cli.Argument{
	// 	&cli.StringArg{
	// 		Name: "SOURCE_FILE",
	// 		Min:  1,
	// 		Max:  1,
	// 	},
	// 	&cli.StringArg{
	// 		Name: "DEST_FILE",
	// 		Min:  1,
	// 		Max:  1,
	// 	},
	// },
	Action: func(ctx context.Context, c *cli.Command) error {
		if !c.Args().Present() {
			return fmt.Errorf("must provide a source an destination")
		}
		sourceFile := c.Args().Get(0)
		destFile := c.Args().Get(1)
		fmt.Fprintf(os.Stdout, "Cloning %s to %s\n", truncatePrefix(sourceFile, 32), truncatePrefix(destFile, 32))
		if err := cloneFile(sourceFile, destFile); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Cloning %s to %s done!\n", truncatePrefix(sourceFile, 32), truncatePrefix(destFile, 32))
		return nil
	},
}

func main() {
	if err := commandRoot.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
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
		hashes[cs] = append(hashes[cs], fullFileInfo{Path: filepath.Join(path, info.Name()), FileInfo: info})
		return nil
	}))
	return
}

type fullFileInfo struct {
	fs.FileInfo
	Path string
}

func cloneFile(source, target string) error {
	if err := unix.Clonefile(source, target, unix.CLONE_NOFOLLOW); err != nil {
		if !errors.Is(err, unix.ENOTSUP) && !errors.Is(err, unix.EXDEV) {
			return fmt.Errorf("clone-file failed: %w", err)
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
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return
	}
	checksum = hex.EncodeToString(h.Sum(nil))
	return
}

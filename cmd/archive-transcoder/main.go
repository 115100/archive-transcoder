package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/115100/archive-transcoder/internal/encoder"
	"github.com/spf13/pflag"
)

func main() {
	if err := run(); err != nil {
		slog.Error("failed to run", slog.Any("err", err))
	}
}

func run() error {
	var outDir, searchDir string
	var inPlace, recurse bool
	pflag.StringVarP(&outDir, "output-dir", "o", "", "output directory")
	pflag.StringVarP(&searchDir, "search-dir", "s", ".", "search directory")
	pflag.BoolVarP(&inPlace, "in-place", "i", false, "replace source directory with new archive")
	pflag.BoolVarP(&recurse, "recurse", "r", false, "recurse into search directory")
	pflag.Parse()

	if !inPlace {
		if outDir == "" {
			return errors.New("--output-dir/-o must be set")
		}
		ods, err := os.Stat(outDir)
		if err != nil {
			return err
		}
		sds, err := os.Stat(searchDir)
		if err != nil {
			return err
		}
		if os.SameFile(ods, sds) {
			return errors.New("--output-dir/-o must be different from --search-dir/-s")
		}
	} else {
		if outDir != "" {
			return errors.New("--output-dir/-o shouldn't be set with --in-place/-i")
		}
	}

	enc, err := encoder.NewEncoder()
	if err != nil {
		return err
	}
	defer enc.Close()

	start := time.Now()
	return filepath.Walk(searchDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("failed to Walk: %w", err)
		}

		relPath, _ := filepath.Rel(searchDir, path)
		if !recurse && strings.Count(relPath, "/") > 1 {
			return nil
		}
		// Walk will catch new archives if they turn up under the search dir
		if info.ModTime().After(start) {
			return nil
		}

		switch filepath.Ext(path) {
		case ".zip", ".cbz":
			slog.Info(
				"processing archive",
				slog.String("path", path),
			)

			var outArchive string
			if inPlace {
				tf, err := os.CreateTemp(searchDir, filepath.Base(path))
				if err != nil {
					return err
				}
				if err := tf.Close(); err != nil {
					return err
				}
				outArchive = tf.Name()
				defer os.Remove(outArchive) // gone if successful
			} else {
				outArchive = filepath.Join(outDir, path)
			}

			if err := processArchive(enc, outArchive, path); err != nil {
				return err
			}
			if inPlace {
				if err := os.Rename(path, path+".bak"); err != nil {
					return err
				}
				if err := os.Rename(outArchive, path); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func processArchive(enc *encoder.Encoder, outArchive, archive string) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("failed to OpenReader: %w", err)
	}
	defer r.Close()

	if err := os.MkdirAll(filepath.Dir(outArchive), 0755); err != nil {
		return fmt.Errorf("failed to MkdirAll: %w", err)
	}

	w, err := os.Create(outArchive)
	if err != nil {
		return fmt.Errorf("failed to create output archive: %w", err)
	}
	defer w.Close()
	zw := zip.NewWriter(w)
	defer zw.Close()

	for _, f := range r.File {
		if err := processEntry(zw, enc, f); err != nil {
			return fmt.Errorf("failed to processEntry: %w", err)
		}
	}

	return nil
}

func processEntry(zw *zip.Writer, enc *encoder.Encoder, f *zip.File) error {
	if f.FileInfo().IsDir() {
		return nil
	}

	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("failed to Open zip entry: %w", err)
	}
	defer rc.Close()

	switch filepath.Ext(f.Name) {
	case ".jpeg", ".jpg", ".png":
		rawImg, err := io.ReadAll(rc)
		if err != nil {
			return fmt.Errorf("failed to ReadAll on image: %w", err)
		}

		v, err := enc.EncodeImage(bytes.NewReader(rawImg))
		if err != nil {
			return fmt.Errorf("failed to EncodeImage: %w", err)
		}

		zc, err := zw.CreateHeader(&zip.FileHeader{
			Name:   strings.TrimSuffix(f.Name, filepath.Ext(f.Name)) + ".jxl",
			Method: f.Method,
		})
		if err != nil {
			return fmt.Errorf("failed to CreateHeader: %w", err)
		}
		if _, err := zc.Write(v); err != nil {
			return fmt.Errorf("failed to Write: %w", err)
		}
	default:
		zc, err := zw.CreateHeader(&zip.FileHeader{
			Name:   f.Name,
			Method: f.Method,
		})
		if err != nil {
			return fmt.Errorf("failed to CreateHeader: %w", err)
		}

		if _, err := io.Copy(zc, rc); err != nil {
			return fmt.Errorf("failed to Copy: %w", err)
		}
	}
	return nil
}

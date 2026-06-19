package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
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
	var recurse bool
	var threads int
	pflag.StringVarP(&outDir, "output-dir", "o", "", "output directory")
	pflag.StringVarP(&searchDir, "search-dir", "s", ".", "search directory")
	pflag.BoolVarP(&recurse, "recurse", "r", false, "recurse into search directory")
	pflag.IntVarP(&threads, "threads", "t", runtime.NumCPU(), "number of encoding threads")
	pflag.Parse()

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

	enc, err := encoder.NewEncoder(threads)
	if err != nil {
		return err
	}
	defer enc.Close()

	start := time.Now()
	var startSize, endSize int64
	if err := filepath.Walk(searchDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("failed to Walk: %w", err)
		}

		relPath, _ := filepath.Rel(searchDir, path)
		if !recurse && strings.Count(relPath, "/") > 0 {
			return nil
		}
		// Walk will catch new archives if they turn up under the search dir
		if info.ModTime().After(start) {
			return nil
		}

		switch filepath.Ext(path) {
		case ".zip", ".cbz":
			startSize += info.Size()
			slog.Info(
				"processing archive",
				slog.String("path", path),
			)

			outArchive := filepath.Join(outDir, relPath)
			if err := processArchive(enc, outArchive, path); err != nil {
				return err
			}
			ofi, err := os.Stat(outArchive)
			if err != nil {
				return err
			}
			endSize += ofi.Size()
		}
		return nil
	}); err != nil {
		return err
	}

	slog.Info(
		"finished processing directory",
		slog.String("search_dir", searchDir),
		slog.Int("start_size_mebibytes", toMiB(startSize)),
		slog.Int("end_size_mebibytes", toMiB(endSize)),
		slog.Int("saved_mebibytes", toMiB(startSize-endSize)),
	)
	return nil
}

func toMiB(nb int64) int {
	return int(float64(nb) / math.Pow(2, 20))
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
		zn := strings.TrimSuffix(f.Name, filepath.Ext(f.Name)) + ".jxl"
		if err != nil {
			slog.Warn(
				"failed to EncodeImage so writing original",
				slog.Any("err", err),
				slog.String("filename", f.Name),
			)
			v = rawImg
			zn = f.Name
		}

		zc, err := zw.CreateHeader(&zip.FileHeader{
			Name:   zn,
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

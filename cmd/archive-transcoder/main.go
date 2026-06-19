package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/115100/archive-transcoder/internal/encoder"
	"github.com/115100/archive-transcoder/internal/handler"
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
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		slog.Warn("received signal; cleaning up...")
		stop()
	}()

	enc, err := encoder.NewEncoder(threads)
	if err != nil {
		return err
	}
	defer enc.Close()

	handler := &handler.Handler{Encoder: enc}

	start := time.Now()
	var startSize, endSize int64
	defer func() {
		slog.Info(
			"finished processing directory",
			slog.String("search_dir", searchDir),
			slog.Int("start_size_mebibytes", toMiB(startSize)),
			slog.Int("end_size_mebibytes", toMiB(endSize)),
			slog.Int("saved_mebibytes", toMiB(startSize-endSize)),
		)
	}()
	return filepath.Walk(searchDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("failed to walk directory: %w", err)
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
			outArchive := filepath.Join(outDir, relPath)
			if err := handler.ProcessArchive(ctx, outArchive, path); err != nil {
				return err
			}
			ofi, err := os.Stat(outArchive)
			if err != nil {
				return err
			}
			startSize += info.Size()
			endSize += ofi.Size()
		}
		return nil
	})
}

func toMiB(nb int64) int {
	return int(float64(nb) / math.Pow(2, 20))
}

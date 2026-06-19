package handler

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/115100/archive-transcoder/internal/encoder"
)

type Handler struct {
	Encoder *encoder.Encoder
}

func (h *Handler) ProcessArchive(ctx context.Context, outArchive, archive string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	slog.Info(
		"processing archive",
		slog.String("path", archive),
	)

	r, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", archive, err)
	}
	defer r.Close()

	if err := os.MkdirAll(filepath.Dir(outArchive), 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", filepath.Dir(outArchive), err)
	}

	var alreadyProcessed bool
	for _, f := range r.File {
		if filepath.Ext(f.Name) == ".jxl" {
			alreadyProcessed = true
		}
	}
	if alreadyProcessed {
		slog.Info(
			"hardlinking previously-processed file",
			slog.String("path", archive),
		)
		return os.Link(archive, outArchive)
	}

	w, err := os.Create(outArchive)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", outArchive, err)
	}
	defer w.Close()
	zw := zip.NewWriter(w)
	defer zw.Close()

	for _, f := range r.File {
		if err := h.processEntry(zw, f); err != nil {
			return fmt.Errorf("failed to process %s: %w", f.Name, err)
		}
	}
	return nil
}

func (h *Handler) processEntry(zw *zip.Writer, f *zip.File) error {
	if f.FileInfo().IsDir() {
		return nil
	}

	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer rc.Close()

	switch filepath.Ext(f.Name) {
	case ".jpeg", ".jpg", ".png":
		rawImg, err := io.ReadAll(rc)
		if err != nil {
			return fmt.Errorf("failed to read image: %w", err)
		}

		v, err := h.Encoder.EncodeImage(rawImg)
		fh := &zip.FileHeader{
			Name:     strings.TrimSuffix(f.Name, filepath.Ext(f.Name)) + ".jxl",
			Comment:  f.Comment,
			Method:   f.Method,
			Modified: time.Now(),
		}
		if err != nil {
			slog.Warn(
				"failed to EncodeImage so writing original",
				slog.Any("err", err),
				slog.String("filename", f.Name),
			)
			v = rawImg
			fh.Name = f.Name
			fh.Modified = f.Modified
		}

		zc, err := zw.CreateHeader(fh)
		if err != nil {
			return fmt.Errorf("failed to create header: %w", err)
		}
		if _, err := zc.Write(v); err != nil {
			return fmt.Errorf("failed to write image: %w", err)
		}
	default:
		zc, err := zw.CreateHeader(&zip.FileHeader{
			Name:    f.Name,
			Comment: f.Comment,
			Method:  f.Method,
		})
		if err != nil {
			return fmt.Errorf("failed to create header: %w", err)
		}

		if _, err := io.Copy(zc, rc); err != nil {
			return fmt.Errorf("failed to copy file: %w", err)
		}
	}
	return nil
}

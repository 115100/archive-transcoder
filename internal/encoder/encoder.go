package encoder

// #cgo LDFLAGS: -ljxl -ljxl_threads
// #include <jxl/encode.h>
// #include <jxl/thread_parallel_runner.h>
// #include <jxl/types.h>
// #include <stdlib.h>
import "C"
import (
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"sync"
	"unsafe"
)

var (
	bufSize = C.size_t(C.sizeof_uint8_t << 20)
)

type Encoder struct {
	jxlEnc *C.JxlEncoder
	runner unsafe.Pointer
	once   *sync.Once
}

func NewEncoder(threads int) (*Encoder, error) {
	return &Encoder{
		runner: C.JxlThreadParallelRunnerCreate(nil, C.size_t(threads)),
		once:   new(sync.Once),
	}, nil
}

func (enc *Encoder) EncodeImage(r io.ReadSeeker) ([]byte, error) {
	if err := enc.initOrResetEncoder(); err != nil {
		return nil, fmt.Errorf("failed to initOrResetEncoder: %w", err)
	}

	img, format, err := image.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("failed to Decode image: %w", err)
	}

	fs := C.JxlEncoderFrameSettingsCreate(enc.jxlEnc, nil)
	if C.JxlEncoderSetFrameLossless(fs, C.JXL_TRUE) != C.JXL_ENC_SUCCESS {
		return nil, errors.New("JxlEncoderSetFrameLossless failed")
	}

	if _, err := r.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("failed to seek: %w", err)
	}

	switch format {
	case "jpeg", "jpg":
		if err := enc.jpegFrame(r, fs); err != nil {
			return nil, fmt.Errorf("failed to process jpegFrame: %w", err)
		}
	case "png":
		if err := enc.pngFrame(r, img, fs); err != nil {
			return nil, fmt.Errorf("failed to process pngFrame: %w", err)
		}
	}
	C.JxlEncoderCloseInput(enc.jxlEnc)

	var v []byte
	var status C.JxlEncoderStatus = C.JXL_ENC_NEED_MORE_OUTPUT
	for status == C.JXL_ENC_NEED_MORE_OUTPUT {
		buf := C.malloc(bufSize)
		defer C.free(buf)

		next_out := (*C.uint8_t)(buf)
		avail_bytes := bufSize
		status = C.JxlEncoderProcessOutput(enc.jxlEnc, &next_out, &avail_bytes)

		v = append(v, C.GoBytes(buf, C.int(bufSize-avail_bytes))...)
	}
	if status != C.JXL_ENC_SUCCESS {
		return nil, errors.New("JxlEncodeProcessOutput failed")
	}
	return v, nil
}

func (enc *Encoder) Close() error {
	var err error
	enc.once.Do(func() {
		if enc.jxlEnc != nil {
			C.JxlEncoderDestroy(enc.jxlEnc)
		}
		C.JxlThreadParallelRunnerDestroy(enc.runner)
	})
	return err
}

// -----------------------------------------------------------------------------

func (enc *Encoder) initOrResetEncoder() error {
	if enc.jxlEnc == nil {
		enc.jxlEnc = C.JxlEncoderCreate(nil)
	} else {
		C.JxlEncoderReset(enc.jxlEnc)
	}

	if C.JxlEncoderSetParallelRunner(
		enc.jxlEnc,
		(*[0]byte)(C.JxlThreadParallelRunner),
		enc.runner,
	) != C.JXL_ENC_SUCCESS {
		return errors.New("JxlEncoderSetParallelRunner failed")
	}

	return nil
}

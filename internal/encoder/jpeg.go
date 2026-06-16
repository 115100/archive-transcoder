package encoder

// #cgo LDFLAGS: -ljxl -ljxl_threads
// #include <jxl/encode.h>
// #include <stdlib.h>
import "C"
import (
	"errors"
	"io"
)

func (enc *Encoder) jpegFrame(r io.Reader, fs *C.JxlEncoderFrameSettings) error {
	v, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	if C.JxlEncoderStoreJPEGMetadata(enc.jxlEnc, C.JXL_TRUE) != C.JXL_ENC_SUCCESS {
		return errors.New("JxlEncoderStoreJPEGMetadata failed")
	}

	buffer := C.CBytes(v)
	defer C.free(buffer)
	if C.JxlEncoderAddJPEGFrame(
		fs,
		(*C.uint8_t)(buffer),
		(C.size_t)(len(v)),
	) != C.JXL_ENC_SUCCESS {
		return errors.New("JxlEncoderAddJPEGFrame failed")
	}

	return nil
}

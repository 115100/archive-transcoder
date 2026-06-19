package encoder

// #cgo LDFLAGS: -ljxl -ljxl_threads
// #include <jxl/encode.h>
// #include <stdlib.h>
import "C"
import "errors"

func (enc *Encoder) jpegFrame(rawImg []byte, fs *C.JxlEncoderFrameSettings) error {
	if C.JxlEncoderStoreJPEGMetadata(enc.jxlEnc, C.JXL_TRUE) != C.JXL_ENC_SUCCESS {
		return errors.New("JxlEncoderStoreJPEGMetadata failed")
	}

	buffer := C.CBytes(rawImg)
	defer C.free(buffer)
	if C.JxlEncoderAddJPEGFrame(
		fs,
		(*C.uint8_t)(buffer),
		C.size_t(len(rawImg)),
	) != C.JXL_ENC_SUCCESS {
		return errors.New("JxlEncoderAddJPEGFrame failed")
	}

	return nil
}

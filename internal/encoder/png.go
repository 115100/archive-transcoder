package encoder

// #include <jxl/encode.h>
// #include <stdlib.h>
import "C"
import (
	"errors"
	"fmt"
	"image"
	"io"
	"log/slog"
	"unsafe"

	"github.com/mandykoh/prism/meta/pngmeta"
)

func (enc *Encoder) pngFrame(r io.Reader, img image.Image, fs *C.JxlEncoderFrameSettings) error {
	bi := new(C.JxlBasicInfo)
	C.JxlEncoderInitBasicInfo(bi)
	bi.xsize = C.uint32_t(img.Bounds().Dx())
	bi.ysize = C.uint32_t(img.Bounds().Dy())
	bi.uses_original_profile = C.JXL_TRUE
	pf := new(C.JxlPixelFormat)

	buffer, n, err := populateInfo(bi, pf, img)
	if err != nil {
		return err
	}
	defer C.free(buffer)

	if C.JxlEncoderSetBasicInfo(enc.jxlEnc, bi) != C.JXL_ENC_SUCCESS {
		return errors.New("JxlEncoderSetBasicInfo failed")
	}

	var icc []byte
	md, _, err := pngmeta.Load(r)
	if err != nil {
		slog.Warn(
			"failed to load image metadata",
			slog.Any("err", err),
		)
	} else {
		icc, err = md.ICCProfileData()
		if err != nil {
			return fmt.Errorf("failed to load ICC profile data: %w", err)
		}
	}
	if len(icc) > 0 {
		buffer := C.CBytes(icc)
		defer C.free(buffer)

		if C.JxlEncoderSetICCProfile(enc.jxlEnc, (*C.uint8_t)(buffer), C.size_t(len(icc))) != C.JXL_ENC_SUCCESS {
			return errors.New("JxlEncoderSetICCProfile failed")
		}
	} else {
		// Go's png parser doesn't process ancillary chunks so just default to sRGB
		// Images with gamma of non-2.2 etc will look wrong
		ce := new(C.JxlColorEncoding)

		var isGray C.int = C.JXL_FALSE
		if bi.num_color_channels == 1 {
			isGray = C.JXL_TRUE
		}
		C.JxlColorEncodingSetToSRGB(ce, isGray)

		if C.JxlEncoderSetColorEncoding(enc.jxlEnc, ce) != C.JXL_ENC_SUCCESS {
			return errors.New("JxlEncoderSetColorEncoding failed")
		}
	}

	if C.JxlEncoderAddImageFrame(fs, pf, buffer, C.size_t(n)) != C.JXL_ENC_SUCCESS {
		return errors.New("JxlEncoderAddImageFrame failed")
	}
	return nil
}

func populateInfo(bi *C.JxlBasicInfo, pf *C.JxlPixelFormat, img image.Image) (unsafe.Pointer, int, error) {
	var buffer unsafe.Pointer
	var n int
	switch v := img.(type) {
	case *image.Paletted:
		bi.alpha_bits = 8
		bi.bits_per_sample = 8
		bi.num_extra_channels = 1
		pf.data_type = C.JXL_TYPE_UINT8

		pixs := make([]uint8, 0, 4*img.Bounds().Dx()*img.Bounds().Dy())
		for y := range img.Bounds().Dy() {
			for x := range img.Bounds().Dx() {
				r, g, b, a := v.At(x, y).RGBA()
				pixs = append(pixs, uint8(r), uint8(g), uint8(b), uint8(a))
			}
		}

		buffer = C.CBytes(pixs)
		n = len(pixs)
	case *image.RGBA:
		bi.alpha_bits = 8
		bi.alpha_premultiplied = C.JXL_TRUE
		bi.bits_per_sample = 8
		bi.num_extra_channels = 1
		pf.data_type = C.JXL_TYPE_UINT8

		buffer = C.CBytes(v.Pix)
		n = len(v.Pix)
	case *image.NRGBA:
		bi.alpha_bits = 8
		bi.bits_per_sample = 8
		bi.num_extra_channels = 1
		pf.data_type = C.JXL_TYPE_UINT8

		buffer = C.CBytes(v.Pix)
		n = len(v.Pix)
	case *image.RGBA64:
		bi.alpha_bits = 16
		bi.alpha_premultiplied = C.JXL_TRUE
		bi.bits_per_sample = 16
		bi.num_extra_channels = 1
		pf.data_type = C.JXL_TYPE_UINT16
		pf.endianness = C.JXL_BIG_ENDIAN

		buffer = C.CBytes(v.Pix)
		n = len(v.Pix)
	case *image.NRGBA64:
		bi.alpha_bits = 16
		bi.bits_per_sample = 16
		bi.num_extra_channels = 1
		pf.data_type = C.JXL_TYPE_UINT16
		pf.endianness = C.JXL_BIG_ENDIAN

		buffer = C.CBytes(v.Pix)
		n = len(v.Pix)
	case *image.Gray:
		bi.bits_per_sample = 8
		bi.num_color_channels = 1
		pf.data_type = C.JXL_TYPE_UINT8

		buffer = C.CBytes(v.Pix)
		n = len(v.Pix)
	case *image.Gray16:
		bi.bits_per_sample = 16
		bi.num_color_channels = 1
		pf.data_type = C.JXL_TYPE_UINT16
		pf.endianness = C.JXL_BIG_ENDIAN

		buffer = C.CBytes(v.Pix)
		n = len(v.Pix)
	default:
		return nil, 0, fmt.Errorf("unsupported image type: %T", img)
	}
	pf.num_channels = bi.num_color_channels + bi.num_extra_channels

	return buffer, n, nil
}

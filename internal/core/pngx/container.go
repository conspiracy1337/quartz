package pngx

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image/png"
)

// ErrIconSize is returned for a PNG an icon container can't hold.
var ErrIconSize = errors.New("pngx: icon must be a square PNG of 16 to 1024 pixels")

func squareSide(p []byte) (int, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(p))
	if err != nil || cfg.Width != cfg.Height || cfg.Width < 16 || cfg.Width > 1024 {
		return 0, ErrIconSize
	}
	return cfg.Width, nil
}

// ICO wraps square PNGs (at most 256 px each) in a Windows .ico file. PNG
// entries are read by Windows Vista and later.
func ICO(pngs ...[]byte) ([]byte, error) {
	var head, body bytes.Buffer
	_ = binary.Write(&head, binary.LittleEndian, [3]uint16{0, 1, uint16(len(pngs))}) //nolint:gosec // a handful of entries
	offset := 6 + 16*len(pngs)
	for _, p := range pngs {
		side, err := squareSide(p)
		if err != nil || side > 256 {
			return nil, ErrIconSize
		}
		dim := byte(side % 256) //nolint:gosec // side is 16..256; 256 is written as 0
		head.Write([]byte{dim, dim, 0, 0})
		_ = binary.Write(&head, binary.LittleEndian, [2]uint16{1, 32})                                       // planes, bits per pixel
		_ = binary.Write(&head, binary.LittleEndian, [2]uint32{uint32(len(p)), uint32(offset + body.Len())}) //nolint:gosec // icons are small
		body.Write(p)
	}
	return append(head.Bytes(), body.Bytes()...), nil
}

// icnsTypes are the PNG element types of a macOS .icns file by side.
var icnsTypes = map[int]string{16: "icp4", 32: "icp5", 64: "icp6", 128: "ic07", 256: "ic08", 512: "ic09", 1024: "ic10"}

// ICNS wraps square PNGs of the sizes macOS knows (16 to 1024) in an .icns file.
func ICNS(pngs ...[]byte) ([]byte, error) {
	var body bytes.Buffer
	for _, p := range pngs {
		side, err := squareSide(p)
		if err != nil {
			return nil, err
		}
		t, ok := icnsTypes[side]
		if !ok {
			return nil, ErrIconSize
		}
		body.WriteString(t)
		_ = binary.Write(&body, binary.BigEndian, uint32(8+len(p))) //nolint:gosec // icons are small
		body.Write(p)
	}
	var out bytes.Buffer
	out.WriteString("icns")
	_ = binary.Write(&out, binary.BigEndian, uint32(8+body.Len())) //nolint:gosec // icons are small
	out.Write(body.Bytes())
	return out.Bytes(), nil
}

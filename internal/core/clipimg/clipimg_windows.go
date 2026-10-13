package clipimg

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/draw"
	_ "image/png" // decode screenshots
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	openClipboard    = user32.NewProc("OpenClipboard")
	closeClipboard   = user32.NewProc("CloseClipboard")
	emptyClipboard   = user32.NewProc("EmptyClipboard")
	setClipboardData = user32.NewProc("SetClipboardData")
	registerFormat   = user32.NewProc("RegisterClipboardFormatW")
	globalAlloc      = kernel32.NewProc("GlobalAlloc")
	globalLock       = kernel32.NewProc("GlobalLock")
	globalUnlock     = kernel32.NewProc("GlobalUnlock")
	globalFree       = kernel32.NewProc("GlobalFree")
	moveMemory       = kernel32.NewProc("RtlMoveMemory")
)

const (
	cfDIB         = 8
	gmemMoveable  = 0x0002
	maxClipSource = 64 << 20
)

func copyPNG(path string) error {
	raw, err := os.ReadFile(path) //nolint:gosec // a screenshot the caller checked
	if err != nil {
		return err
	}
	if len(raw) > maxClipSource {
		return errors.New("clipimg: picture too large")
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if r, _, err := openClipboard.Call(0); r == 0 {
		return err
	}
	defer func() { _, _, _ = closeClipboard.Call() }()
	_, _, _ = emptyClipboard.Call()
	// PNG keeps transparency for apps that read it; DIB is what every app reads.
	if name, err := windows.UTF16PtrFromString("PNG"); err == nil {
		if f, _, _ := registerFormat.Call(uintptr(unsafe.Pointer(name))); f != 0 {
			_ = set(uint32(f), raw)
		}
	}
	return set(cfDIB, dib(img))
}

// set hands a copy of data to the clipboard, which owns it afterwards.
func set(format uint32, data []byte) error {
	h, _, err := globalAlloc.Call(gmemMoveable, uintptr(len(data)))
	if h == 0 {
		return err
	}
	p, _, err := globalLock.Call(h)
	if p == 0 {
		_, _, _ = globalFree.Call(h)
		return err
	}
	if len(data) > 0 {
		_, _, _ = moveMemory.Call(p, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
	}
	_, _, _ = globalUnlock.Call(h)
	if r, _, err := setClipboardData.Call(uintptr(format), h); r == 0 {
		_, _, _ = globalFree.Call(h)
		return err
	}
	return nil
}

// dib is a 32-bit bottom-up device-independent bitmap of img.
func dib(img image.Image) []byte {
	b := img.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
	w, h := b.Dx(), b.Dy()
	var buf bytes.Buffer
	hdr := struct {
		Size                         uint32
		Width, Height                int32
		Planes, BitCount             uint16
		Compression, SizeImage       uint32
		XPels, YPels, ClrUsed, ClrIm int32
	}{Size: 40, Width: int32(w), Height: int32(h), Planes: 1, BitCount: 32, SizeImage: uint32(w * h * 4)} //nolint:gosec // screenshot sizes fit
	_ = binary.Write(&buf, binary.LittleEndian, hdr)
	row := make([]byte, w*4)
	for y := h - 1; y >= 0; y-- {
		src := rgba.Pix[y*rgba.Stride : y*rgba.Stride+w*4]
		for x := 0; x < w; x++ {
			row[x*4], row[x*4+1], row[x*4+2], row[x*4+3] = src[x*4+2], src[x*4+1], src[x*4], src[x*4+3]
		}
		buf.Write(row)
	}
	return buf.Bytes()
}

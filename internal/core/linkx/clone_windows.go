//go:build windows

package linkx

import (
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Block cloning (ReFS and Dev Drive), after Pandora's vendored reflink-copy.
const fileSupportsBlockRefcounting = 0x08000000

type duplicateExtentsData struct {
	FileHandle       windows.Handle
	SourceFileOffset int64
	TargetFileOffset int64
	ByteCount        int64
}

type integrityInformation struct {
	ChecksumAlgorithm        uint16
	Reserved                 uint16
	Flags                    uint32
	ChecksumChunkSizeInBytes uint32
	ClusterSizeInBytes       uint32
}

func clonePath(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src) //nolint:gosec // callers pass paths they validated
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_RDWR, perm) //nolint:gosec // a fresh temp name
	if err != nil {
		return err
	}
	err = cloneHandle(out, in, info.Size())
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(dst)
	}
	return err
}

func cloneHandle(dst, src *os.File, size int64) error {
	sh, dh := windows.Handle(src.Fd()), windows.Handle(dst.Fd())
	var flags uint32
	if err := windows.GetVolumeInformationByHandle(dh, nil, 0, nil, nil, &flags, nil, 0); err != nil || flags&fileSupportsBlockRefcounting == 0 {
		return ErrUnsupported
	}
	var ii integrityInformation
	var n uint32
	cluster := int64(0)
	if windows.DeviceIoControl(sh, windows.FSCTL_GET_INTEGRITY_INFORMATION, nil, 0, (*byte)(unsafe.Pointer(&ii)), uint32(unsafe.Sizeof(ii)), &n, nil) == nil {
		cluster = int64(ii.ClusterSizeInBytes)
	}
	if cluster != 0 && cluster != 4<<10 && cluster != 64<<10 {
		return ErrUnsupported
	}
	if err := windows.DeviceIoControl(dh, windows.FSCTL_SET_SPARSE, nil, 0, nil, 0, &n, nil); err != nil {
		return errors.Join(ErrUnsupported, err)
	}
	if err := dst.Truncate(size); err != nil {
		return err
	}
	total := size
	if cluster != 0 {
		total = (size + cluster - 1) &^ (cluster - 1)
	}
	step := int64(4<<30) - max(cluster, 64<<10)
	for off := int64(0); off < total; off += step {
		d := duplicateExtentsData{FileHandle: sh, SourceFileOffset: off, TargetFileOffset: off, ByteCount: min(step, total-off)}
		if err := windows.DeviceIoControl(dh, windows.FSCTL_DUPLICATE_EXTENTS_TO_FILE, (*byte)(unsafe.Pointer(&d)), uint32(unsafe.Sizeof(d)), nil, 0, &n, nil); err != nil {
			return errors.Join(ErrUnsupported, err)
		}
	}
	var sparse byte // FILE_SET_SPARSE_BUFFER{SetSparse: FALSE}
	_ = windows.DeviceIoControl(dh, windows.FSCTL_SET_SPARSE, &sparse, 1, nil, 0, &n, nil)
	return nil
}

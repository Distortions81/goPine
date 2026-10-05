//go:build !baremetal

package main

import (
	"errors"
	"os"

	"github.com/Distortions81/goPine/internal/checkpoint"
)

// Persistence is opt-in so ordinary simulator runs do not modify host files.
// The backing file uses the same bytes and NOR constraints as PineTime flash.
type desktopFlash struct{ path string }

func openClockJournal() (*checkpoint.Journal, error) {
	path := os.Getenv("GOPINE_SIM_STORAGE")
	if path == "" {
		return nil, nil
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		var erased [checkpoint.Size]byte
		for i := range erased {
			erased[i] = 255
		}
		_, err = f.Write(erased[:])
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
	} else if !os.IsExist(err) {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() != checkpoint.Size {
		return nil, errors.New("invalid simulator storage file")
	}
	return checkpoint.Open(desktopFlash{path: path})
}

func (f desktopFlash) ReadAt(b []byte, off int64) (int, error) {
	file, err := os.Open(f.path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	return file.ReadAt(b, off)
}

func (f desktopFlash) write(b []byte, off int64) (int, error) {
	file, err := os.OpenFile(f.path, os.O_RDWR, 0600)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	n, err := file.WriteAt(b, off)
	if err == nil {
		err = file.Sync()
	}
	return n, err
}

func (f desktopFlash) WriteAt(b []byte, off int64) (int, error) {
	if off < 0 || off%4 != 0 || len(b)%4 != 0 || off+int64(len(b)) > checkpoint.Size {
		return 0, errors.New("invalid simulator flash write")
	}
	old := make([]byte, len(b))
	if _, err := f.ReadAt(old, off); err != nil {
		return 0, err
	}
	for i, v := range b {
		if old[i]&v != v {
			return 0, errors.New("simulator flash is not erased")
		}
	}
	return f.write(b, off)
}

func (f desktopFlash) ErasePage(off int64) error {
	if off < 0 || off%checkpoint.PageSize != 0 || off+checkpoint.PageSize > checkpoint.Size {
		return errors.New("invalid simulator flash erase")
	}
	var erased [checkpoint.PageSize]byte
	for i := range erased {
		erased[i] = 255
	}
	_, err := f.write(erased[:], off)
	return err
}

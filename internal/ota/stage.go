package ota

import (
	"errors"
	"io"
)

type Flash interface {
	io.ReaderAt
	WriteAt([]byte, int64) (int, error)
	EraseSector(int64) error
}

// StageRecovery schedules the factory BLE receiver as a permanent update.
// It must never run while testing an update: the secondary slot is then the
// only rollback copy. The boot magic is invalidated first and committed last.
func StageRecovery(f Flash, confirmed bool, progress func(int)) error {
	if !confirmed {
		return errors.New("keep or revert this update first")
	}
	source, err := Validate(f, 0, RecoverySize)
	if err != nil {
		return err
	}
	lastSector := int64(Secondary + SlotSize - SectorSize)
	if err := f.EraseSector(lastSector); err != nil {
		return err
	}
	for pos := int64(Secondary); pos < lastSector; pos += SectorSize {
		if err := f.EraseSector(pos); err != nil {
			return err
		}
		report(progress, int((pos-Secondary)*40/SlotSize))
	}
	if err := copyImage(f, Secondary, f, source.Size, func(percent int) {
		report(progress, 40+percent/2)
	}); err != nil {
		return err
	}
	copy, err := Validate(f, Secondary, MaxImageSize)
	if err != nil {
		return err
	}
	if copy != source {
		return errors.New("recovery readback differs")
	}
	// Recovery is a stable fallback, not itself a trial. Mark it permanent
	// before the final magic write makes the swap visible to MCUboot.
	if err := writeAt(f, []byte{1, 0, 0, 0}, Secondary+ImageOK); err != nil {
		return err
	}
	var flag [4]byte
	if err := readAt(f, flag[:], Secondary+ImageOK); err != nil {
		return err
	}
	if flag != [4]byte{1, 0, 0, 0} {
		return errors.New("recovery flag readback failed")
	}
	if err := writeAt(f, BootMagic[:], Secondary+MagicOffset); err != nil {
		return err
	}
	var magic [16]byte
	if err := readAt(f, magic[:], Secondary+MagicOffset); err != nil {
		return err
	}
	if magic != BootMagic {
		return errors.New("recovery commit readback failed")
	}
	report(progress, 100)
	return nil
}

// InstallRecovery is used only by the wired bootstrap build. It never touches
// the update slot or filesystem. Identical recovery images are not rewritten.
func InstallRecovery(f Flash, source io.ReaderAt, size int64, progress func(int)) error {
	if size > RecoverySize {
		return errors.New("recovery exceeds reserved area")
	}
	wanted, err := Validate(source, 0, size)
	if err != nil {
		return err
	}
	if actual, err := Validate(f, 0, RecoverySize); err == nil && actual == wanted {
		report(progress, 100)
		return nil
	}
	for pos := int64(0); pos < RecoverySize; pos += SectorSize {
		if err := f.EraseSector(pos); err != nil {
			return err
		}
		report(progress, int(pos*40/RecoverySize))
	}
	if err := copyImage(f, 0, source, wanted.Size, func(percent int) {
		report(progress, 40+percent/2)
	}); err != nil {
		return err
	}
	actual, err := Validate(f, 0, RecoverySize)
	if err != nil {
		return err
	}
	if actual != wanted {
		return errors.New("installed recovery readback differs")
	}
	report(progress, 100)
	return nil
}

func copyImage(f Flash, dest int64, r io.ReaderAt, size int64, progress func(int)) error {
	var buffer [256]byte
	for pos := int64(0); pos < size; {
		n := min(int64(len(buffer)), size-pos)
		if err := readAt(r, buffer[:n], pos); err != nil {
			return err
		}
		if err := writeAt(f, buffer[:n], dest+pos); err != nil {
			return err
		}
		pos += n
		report(progress, int(pos*100/size))
	}
	return nil
}

func writeAt(f Flash, p []byte, offset int64) error {
	n, err := f.WriteAt(p, offset)
	if err != nil {
		return err
	}
	if n != len(p) {
		return io.ErrShortWrite
	}
	return nil
}

func report(progress func(int), percent int) {
	if progress != nil {
		progress(percent)
	}
}

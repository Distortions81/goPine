package timesync

import "errors"

// Normalize accepts standard CTS or InfiniLink's legacy calendar packet.
// InfiniLink's SetTime.swift writes Sunday-based weekday and a variable-width
// hexadecimal ten-thousandths field, omitting adjustReason. Preserve the seven
// unambiguous calendar bytes; derive weekday and drop ambiguous sub-seconds.
// This compatibility path never relaxes actual calendar/time validation.
func Normalize(value []byte) ([ValueSize]byte, error) {
	var out [ValueSize]byte
	if _, err := Decode(value); err == nil {
		copy(out[:], value)
		return out, nil
	}
	if (len(value) != 9 && len(value) != 10) || value[7] < 1 || value[7] > 7 {
		return out, errors.New("unsupported time packet")
	}
	copy(out[:7], value[:7])
	out[9] = 1
	stamp, err := Decode(out[:])
	if err != nil {
		return out, err
	}
	if value[7] != byte(stamp.Weekday())+1 {
		return out, errors.New("unsupported weekday encoding")
	}
	return Encode(stamp)
}

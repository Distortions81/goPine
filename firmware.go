package main

var firmwareVersion = "dev"

type updateState uint8

const (
	firmwareUnavailable updateState = iota
	firmwareConfirmed
	firmwareTrial
	firmwareInvalid
)

func updatePowerOK(p powerStatus) bool {
	return p.State != chargeDischarging || p.Percent >= 20
}

// Call only after checking the primary header and trailer magic. MCUboot sets
// a byte and pads with 0xff; InfiniTime writes the whole confirmation word.
func stateFromTrailer(imageOK, copyDone uint32) updateState {
	if byte(imageOK) == 1 {
		return firmwareConfirmed
	}
	if byte(imageOK) == 0xff && byte(copyDone) == 1 {
		return firmwareTrial
	}
	return firmwareInvalid
}

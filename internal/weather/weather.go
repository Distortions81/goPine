// Package weather decodes InfiniTime's Simple Weather Service wire format.
// It keeps bounded values only; receiving data never changes the watch clock.
package weather

import "encoding/binary"

const MaxPacket = 53

type Day struct {
	Low, High int16 // Hundredths of a degree Celsius.
	Icon      uint8
}

type Current struct {
	Timestamp int64 // Local civil seconds, per the companion protocol.
	Location  [32]byte
	Temp      int16
	Day
}

type Forecast struct {
	Timestamp int64
	Days      [5]Day
	Count     uint8
}

type Cache struct {
	Current     Current
	Forecast    Forecast
	HasCurrent  bool
	HasForecast bool
}

func validIcon(icon byte) bool { return icon <= 8 || icon == 255 }

func day(data []byte) (Day, bool) {
	d := Day{int16(binary.LittleEndian.Uint16(data)), int16(binary.LittleEndian.Uint16(data[2:])), data[4]}
	return d, d.Low <= d.High && validIcon(d.Icon)
}

// Apply validates a complete ATT value before replacing either cached record.
// Current v0/v1 and padded or compact v0 forecasts are supported. Truncated,
// oversized and future protocol versions leave the last good record intact.
func (c *Cache) Apply(data []byte) bool {
	if len(data) < 10 || len(data) > MaxPacket {
		return false
	}
	ts := binary.LittleEndian.Uint64(data[2:10])
	if ts < 946684800 || ts >= 4102444800 { // Supported calendar: 2000–2099.
		return false
	}
	switch data[0] {
	case 0:
		if !((data[1] == 0 && len(data) == 49) || (data[1] == 1 && len(data) == 53)) {
			return false
		}
		v := Current{Timestamp: int64(ts), Temp: int16(binary.LittleEndian.Uint16(data[10:]))}
		v.Low, v.High, v.Icon = int16(binary.LittleEndian.Uint16(data[12:])), int16(binary.LittleEndian.Uint16(data[14:])), data[48]
		if v.Low > v.High || !validIcon(v.Icon) {
			return false
		}
		for i, b := range data[16:48] {
			if b == 0 {
				break
			}
			if b < 32 || b > 126 {
				b = '?' // Existing watch fonts cover printable ASCII.
			}
			v.Location[i] = b
		}
		// Sunrise/sunset in v1 are not displayed by this first integration.
		c.Current, c.HasCurrent = v, true
	case 1:
		if data[1] != 0 || len(data) < 11 || data[10] > 5 {
			return false
		}
		n := int(data[10])
		if len(data) != 11+n*5 && len(data) != 36 {
			return false
		}
		v := Forecast{Timestamp: int64(ts), Count: uint8(n)}
		for i := 0; i < n; i++ {
			d, ok := day(data[11+i*5:])
			if !ok {
				return false
			}
			v.Days[i] = d
		}
		c.Forecast, c.HasForecast = v, true
	default:
		return false
	}
	return true
}

func (c *Current) City() string {
	n := 0
	for n < len(c.Location) && c.Location[n] != 0 {
		n++
	}
	return string(c.Location[:n])
}

func Condition(icon uint8) string {
	switch icon {
	case 0:
		return "Clear"
	case 1:
		return "Partly cloudy"
	case 2:
		return "Cloudy"
	case 3:
		return "Overcast"
	case 4:
		return "Showers"
	case 5:
		return "Rain"
	case 6:
		return "Thunderstorm"
	case 7:
		return "Snow"
	case 8:
		return "Mist"
	default:
		return "Unknown"
	}
}

func Degrees(value int16, fahrenheit bool) int {
	n := int(value)
	if fahrenheit {
		n = n*9/5 + 3200
	}
	if n < 0 {
		return (n - 50) / 100
	}
	return (n + 50) / 100
}

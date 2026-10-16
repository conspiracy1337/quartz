package tomlx

import (
	"fmt"
	"strings"
)

// LocalDate is a date without a time or offset (go-toml's toml.LocalDate).
type LocalDate struct{ Year, Month, Day int }

func (d LocalDate) String() string { return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day) }

// LocalTime is a time of day without a date or offset (go-toml's toml.LocalTime).
type LocalTime struct {
	Hour, Minute, Second, Nanosecond int
	Precision                        int // digits written for Nanosecond; 0 trims trailing zeros
}

func (t LocalTime) String() string {
	s := fmt.Sprintf("%02d:%02d:%02d", t.Hour, t.Minute, t.Second)
	if t.Precision > 0 {
		s += fmt.Sprintf(".%09d", t.Nanosecond)[:t.Precision+1]
	} else if t.Nanosecond > 0 {
		s += strings.TrimRight(fmt.Sprintf(".%09d", t.Nanosecond), "0")
	}
	return s
}

// LocalDateTime is a date and time without an offset (go-toml's toml.LocalDateTime).
type LocalDateTime struct {
	LocalDate
	LocalTime
}

func (d LocalDateTime) String() string { return d.LocalDate.String() + "T" + d.LocalTime.String() }

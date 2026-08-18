package db

import (
	"time"
)

const (
	TIME_FORMAT = "2006-01-02 15:04:05"
)

func FormatTime(t time.Time) string {
	return t.Format(TIME_FORMAT)
}

func ParseTime(t string) (time.Time, error) {
	parsedTime, err := time.ParseInLocation(TIME_FORMAT, t, time.UTC)
	if err != nil {
		return time.Time{}, err
	}

	return parsedTime, nil
}

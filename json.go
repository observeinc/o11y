package o11y

import (
	"encoding/json"
	"io"
)

func jsonDecode[T any](stream io.Reader, result *T) error {
	data, err := io.ReadAll(stream)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, result)
}

func cutPoint(data []byte, maxSize int) ([]byte, []byte) {
	// don't work on ludicrously small arguments
	if maxSize < 32 || data == nil {
		return data, nil
	}
	ld := len(data)
	if ld > maxSize {
		for i := maxSize; i > maxSize/2; i-- {
			if data[i] == '\n' {
				return data[:i+1], data[i+1:]
			}
		}
		for i := maxSize; i < ld; i++ {
			if data[i] == '\n' {
				return data[:i+1], data[i+1:]
			}
		}
	}
	// don't know what to do -- there's no place to cut!
	return data, nil
}

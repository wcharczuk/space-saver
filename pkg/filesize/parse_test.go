package filesize

import (
	"math"
	"testing"
)

func Test_Parse(t *testing.T) {
	testCases := [...]struct {
		Input    string
		Expected uint64
	}{
		{"5b", 5},
		{"5mb", 5 * uint64(Megabyte)},
		{"5.4mb", (5 * uint64(Megabyte)) + uint64(math.Floor(0.4*float64(Megabyte)))},
		{"5mb10kb", (5 * uint64(Megabyte)) + (10 * uint64(Kilobyte))},
	}

	for _, tc := range testCases {
		actual, err := Parse(tc.Input)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			continue
		}
		if actual != tc.Expected {
			t.Errorf("Input=%v Expected=%v vs. Actual=%v", tc.Input, tc.Expected, actual)
		}
	}
}

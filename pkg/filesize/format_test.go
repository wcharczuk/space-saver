package filesize

import (
	"testing"
)

func Test_Format(t *testing.T) {
	testCases := [...]struct {
		Input    uint64
		Expected string
	}{
		{5, "5b"},
		{5 * uint64(Megabyte), "5mb"},
		{(5 * uint64(Megabyte)) + (10 * uint64(Kilobyte)), "5mb10kb"},
	}

	for _, tc := range testCases {
		if actual := Format(tc.Input); actual != tc.Expected {
			t.Errorf("Input=%d Expected=%s vs. Actual=%s", tc.Input, tc.Expected, actual)
		}
	}
}

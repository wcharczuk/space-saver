package filesize

import (
	"fmt"
	"strconv"
)

// Format takes a count of bytes and returns a string representation.
func Format(fileSizeInBytes uint64) string {
	if fileSizeInBytes < uint64(Kilobyte) {
		return strconv.FormatUint(fileSizeInBytes, 10) + suffixByte
	}
	var output []rune
	var remainder = fileSizeInBytes
	for remainder > uint64(Kilobyte) {
		if remainder > uint64(Terabyte) {
			value := remainder / uint64(Terabyte)
			output = append(output, []rune(strconv.FormatUint(value, 10)+suffixTerabyte)...)
			remainder = remainder - (value * uint64(Terabyte))
		} else if remainder > uint64(Gigabyte) {
			value := remainder / uint64(Gigabyte)
			output = append(output, []rune(strconv.FormatUint(value, 10)+suffixGigabyte)...)
			remainder = remainder - (value * uint64(Gigabyte))
		} else if remainder > uint64(Megabyte) {
			value := remainder / uint64(Megabyte)
			output = append(output, []rune(strconv.FormatUint(value, 10)+suffixMegabyte)...)
			remainder = remainder - (value * uint64(Megabyte))
		} else if remainder > uint64(Kilobyte) {
			value := remainder / uint64(Kilobyte)
			output = append(output, []rune(strconv.FormatUint(value, 10)+suffixKilobyte)...)
			remainder = remainder - (value * uint64(Kilobyte))
		}
	}
	if remainder > 0 {
		return string(append(output, []rune(strconv.FormatUint(remainder, 10)+suffixByte)...))
	}
	return string(output)
}

func FormatFraction(fileSizeInBytes uint64) string {
	if fileSizeInBytes > uint64(Terabyte) {
		return strconv.FormatFloat(float64(fileSizeInBytes)/float64(Terabyte), 'f', 3, 64) + suffixTerabyte
	} else if fileSizeInBytes > uint64(Gigabyte) {
		return strconv.FormatFloat(float64(fileSizeInBytes)/float64(Gigabyte), 'f', 3, 64) + suffixGigabyte
	} else if fileSizeInBytes > uint64(Megabyte) {
		return strconv.FormatFloat(float64(fileSizeInBytes)/float64(Megabyte), 'f', 3, 64) + suffixMegabyte
	} else if fileSizeInBytes > uint64(Kilobyte) {
		return strconv.FormatFloat(float64(fileSizeInBytes)/float64(Kilobyte), 'f', 3, 64) + suffixKilobyte
	}
	return fmt.Sprintf("%db", fileSizeInBytes)
}

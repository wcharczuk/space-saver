package filesize

type Unit uint64

const (
	Terabyte Unit = 1 << 40
	Gigabyte Unit = 1 << 30
	Megabyte Unit = 1 << 20
	Kilobyte Unit = 1 << 10
	Byte     Unit = 1
)

const (
	suffixTerabyte = "tb"
	suffixGigabyte = "gb"
	suffixMegabyte = "mb"
	suffixKilobyte = "kb"
	suffixByte     = "b"
)

var unitMap = map[string]Unit{
	"tib":   Terabyte,
	"tb":    Terabyte,
	"gib":   Gigabyte,
	"gb":    Gigabyte,
	"mib":   Megabyte,
	"mb":    Megabyte,
	"kib":   Kilobyte,
	"kb":    Kilobyte,
	"bytes": Byte,
	"b":     Byte,
}

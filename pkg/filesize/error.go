package filesize

// New returns an error that formats as the given text.
// Each call to New returns a distinct error value even if the text is identical.
func Error(text string) error {
	return &errorString{text}
}

// errorString is a trivial implementation of error.
type errorString struct {
	s string
}

func (e *errorString) Error() string {
	return e.s
}

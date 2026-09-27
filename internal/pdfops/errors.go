package pdfops

import "errors"

// Sentinel errors classify failures so the HTTP layer can map them to
// meaningful status codes. They are always wrapped with more context.
var (
	// ErrNoInput is returned when an operation receives no input files.
	ErrNoInput = errors.New("no input files")
	// ErrInvalidPDF is returned when an input file is not a readable PDF.
	ErrInvalidPDF = errors.New("invalid pdf")
	// ErrUnsupported is returned for file types this service cannot process.
	ErrUnsupported = errors.New("unsupported file type")
	// ErrPageRange is returned for malformed or out-of-bounds page selections.
	ErrPageRange = errors.New("invalid page range")
	// ErrMissingParam is returned when a required parameter is absent.
	ErrMissingParam = errors.New("missing parameter")
	// ErrInvalidParam is returned when a parameter value is not acceptable.
	ErrInvalidParam = errors.New("invalid parameter")
)

// OpError ties a failing operation name to its underlying cause.
type OpError struct {
	Op  string
	Err error
}

func (e *OpError) Error() string { return e.Op + ": " + e.Err.Error() }
func (e *OpError) Unwrap() error { return e.Err }

func opErr(op string, err error) error {
	if err == nil {
		return nil
	}
	return &OpError{Op: op, Err: err}
}

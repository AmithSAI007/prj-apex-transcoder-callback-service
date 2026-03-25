package service

import "errors"

type PermanentError struct {
	Err error
}

type TransientError struct {
	Err error
}

func NewPermanentError(err error) *PermanentError {
	return &PermanentError{Err: err}
}
func (e *PermanentError) Error() string {
	return e.Err.Error()
}
func (e *PermanentError) Unwrap() error {
	return e.Err
}

func NewTransientError(err error) *TransientError {
	return &TransientError{Err: err}
}
func (e *TransientError) Error() string {
	return e.Err.Error()
}
func (e *TransientError) Unwrap() error {
	return e.Err
}

// IsPermanent reports whether err (or any error in its chain) is a PermanentError.
func IsPermanent(err error) bool {
	var perr *PermanentError
	return errors.As(err, &perr)
}

// IsTransient reports whether err (or any error in its chain) is a TransientError.
func IsTransient(err error) bool {
	var terr *TransientError
	return errors.As(err, &terr)
}

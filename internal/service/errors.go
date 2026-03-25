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

func isPermanent(err error) bool {
	var perr *PermanentError
	return err != nil && (errors.As(err, &perr) || isPermanent(errors.Unwrap(err)))
}

func isTransient(err error) bool {
	var terr *TransientError
	return err != nil && (errors.As(err, &terr) || isTransient(errors.Unwrap(err)))
}

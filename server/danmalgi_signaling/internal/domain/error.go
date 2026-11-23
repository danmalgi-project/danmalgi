package domain

import (
	"github.com/pkg/errors"
)

const (
	INVALID_TOKEN = iota
	INVALID_INPUT
	DUPLICATED_DATA
	RECORD_NOT_FOUND
	OAUTH_ERROR
	TOKEN_CREATION_FAILED
	PERSISTENCE_ERROR
	INTERNAL
)

// type
type CustomError struct {
	BaseErr    error
	StatusCode int
	Message    string
	// DetailMessage string
}

func (c *CustomError) Error() string {
	return c.Message
}

func (c *CustomError) ChangeMessage(message string) *CustomError {
	c.Message = message
	return c
}

func NewError(err error, status int, message string) *CustomError {
	return &CustomError{
		BaseErr:    err,
		StatusCode: status,
		Message:    message,
	}
}

var (
	ErrNotFindIdInToken = errors.New("failed to parse token or missing id")
)

// build-in error
func NewErrTokenCreationFailed(err error) *CustomError {
	return &CustomError{
		BaseErr:    err,
		StatusCode: TOKEN_CREATION_FAILED,
		Message:    "failed to create token",
	}
}

func NewErrInvalidToken(err error) *CustomError {
	return &CustomError{
		BaseErr:    err,
		StatusCode: INVALID_TOKEN,
		Message:    "request with invalid token",
	}
}

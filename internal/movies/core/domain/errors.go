package domain

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNotFound = errors.New("movie not found")

	ErrAlreadyExists = errors.New("movie already exists")

	ErrInvalid = errors.New("invalid movie")
)

type Violation struct {
	Field   string
	Message string
}

type ValidationError struct {
	Violations []Violation
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Violations))
	for _, v := range e.Violations {
		parts = append(parts, fmt.Sprintf("%s %s", v.Field, v.Message))
	}
	return "invalid movie: " + strings.Join(parts, "; ")
}

func (e *ValidationError) Is(target error) bool {
	return target == ErrInvalid
}

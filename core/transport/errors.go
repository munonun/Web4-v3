package transport

import "errors"

var (
	ErrBadMagic           = errors.New("bad frame magic")
	ErrUnsupportedVersion = errors.New("unsupported frame version")
	ErrFrameTooLarge      = errors.New("frame too large")
	ErrTruncatedFrame     = errors.New("truncated frame")
	ErrEmptyFrame         = errors.New("empty frame")
	ErrInvalidFrame       = errors.New("invalid frame")
)

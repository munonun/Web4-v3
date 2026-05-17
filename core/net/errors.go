package net

import "errors"

var (
	ErrInvalidRuntime    = errors.New("invalid peer runtime")
	ErrInvalidServer     = errors.New("invalid server")
	ErrInvalidClient     = errors.New("invalid client")
	ErrPeerKeyRequired   = errors.New("peer public key is required")
	ErrConnectionMissing = errors.New("connection is required")
)

package handler

import "errors"

var (
	ErrDuplicateMessage     = errors.New("duplicate message")
	ErrUnexpectedMessage    = errors.New("unexpected message")
	ErrInvalidState         = errors.New("invalid state")
	ErrInvalidPeer          = errors.New("invalid peer")
	ErrInvalidPayload       = errors.New("invalid payload")
	ErrTradeExecutionFailed = errors.New("trade execution failed")
)

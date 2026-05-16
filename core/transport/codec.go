package transport

import (
	"fmt"
	"io"

	"web4-v3/core/message"
)

func EncodeFrame(msg message.Message) ([]byte, error) {
	payload, err := message.EncodeMessage(msg)
	if err != nil {
		return nil, err
	}
	return EncodePayloadFrame(payload)
}

func EncodePayloadFrame(payload []byte) ([]byte, error) {
	return encodePayloadFrame(payload)
}

func DecodeFrame(data []byte) (message.Message, error) {
	payload, err := DecodePayloadFrame(data)
	if err != nil {
		return message.Message{}, err
	}
	msg, err := message.DecodeMessage(payload)
	if err != nil {
		return message.Message{}, fmt.Errorf("%w: %v", ErrInvalidFrame, err)
	}
	return msg, nil
}

func DecodePayloadFrame(data []byte) ([]byte, error) {
	return decodePayloadFrame(data)
}

func ReadFrame(r io.Reader) (message.Message, error) {
	payload, err := ReadPayloadFrame(r)
	if err != nil {
		return message.Message{}, err
	}
	msg, err := message.DecodeMessage(payload)
	if err != nil {
		return message.Message{}, fmt.Errorf("%w: %v", ErrInvalidFrame, err)
	}
	return msg, nil
}

func ReadPayloadFrame(r io.Reader) ([]byte, error) {
	headerBytes := make([]byte, FrameHeaderSize)
	if _, err := io.ReadFull(r, headerBytes); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTruncatedFrame, err)
	}
	header, err := decodeFrameHeader(headerBytes)
	if err != nil {
		return nil, err
	}
	payload := make([]byte, int(header.length))
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTruncatedFrame, err)
	}
	return payload, nil
}

func WriteFrame(w io.Writer, msg message.Message) error {
	frame, err := EncodeFrame(msg)
	if err != nil {
		return err
	}
	n, err := w.Write(frame)
	if err != nil {
		return err
	}
	if n != len(frame) {
		return io.ErrShortWrite
	}
	return nil
}

package transport

import (
	"encoding/binary"
	"fmt"
)

type frameHeader struct {
	version uint16
	length  uint32
}

func encodePayloadFrame(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return nil, ErrEmptyFrame
	}
	if len(payload) > MaxFrameSize {
		return nil, ErrFrameTooLarge
	}
	frame := make([]byte, FrameHeaderSize+len(payload))
	copy(frame[:4], FrameMagic)
	binary.BigEndian.PutUint16(frame[4:6], FrameVersion)
	binary.BigEndian.PutUint32(frame[6:10], uint32(len(payload)))
	copy(frame[FrameHeaderSize:], payload)
	return frame, nil
}

func decodeFrameHeader(data []byte) (frameHeader, error) {
	if len(data) < FrameHeaderSize {
		return frameHeader{}, ErrTruncatedFrame
	}
	if string(data[:4]) != FrameMagic {
		return frameHeader{}, ErrBadMagic
	}
	version := binary.BigEndian.Uint16(data[4:6])
	if version != FrameVersion {
		return frameHeader{}, ErrUnsupportedVersion
	}
	length := binary.BigEndian.Uint32(data[6:10])
	if length == 0 {
		return frameHeader{}, ErrEmptyFrame
	}
	if length > MaxFrameSize {
		return frameHeader{}, ErrFrameTooLarge
	}
	return frameHeader{version: version, length: length}, nil
}

func decodePayloadFrame(data []byte) ([]byte, error) {
	header, err := decodeFrameHeader(data)
	if err != nil {
		return nil, err
	}
	want := FrameHeaderSize + int(header.length)
	if len(data) < want {
		return nil, ErrTruncatedFrame
	}
	if len(data) > want {
		return nil, fmt.Errorf("%w: trailing bytes", ErrInvalidFrame)
	}
	payload := make([]byte, int(header.length))
	copy(payload, data[FrameHeaderSize:want])
	return payload, nil
}

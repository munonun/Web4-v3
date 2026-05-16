package transport

const (
	FrameMagic      = "W4M1"
	FrameVersion    = uint16(1)
	MaxFrameSize    = 1 << 20
	FrameHeaderSize = 10
)

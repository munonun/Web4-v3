package transport

import (
	"bytes"

	"web4-v3/core/message"
)

type MemoryPipe struct {
	Buffer bytes.Buffer
}

func (p *MemoryPipe) Write(msg message.Message) error {
	return WriteFrame(&p.Buffer, msg)
}

func (p *MemoryPipe) Read() (message.Message, error) {
	return ReadFrame(&p.Buffer)
}

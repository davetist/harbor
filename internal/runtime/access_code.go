package runtime

import (
	"bytes"
	"errors"
)

const maxSensitive = 1 << 20

type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > maxSensitive {
		return 0, errors.New("private query exceeds output limit")
	}
	return b.Buffer.Write(p)
}

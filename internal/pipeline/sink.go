package pipeline

import (
	"bufio"
	"io"

	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

// JSONLSink writes hello and ticks as JSON Lines (--dump-json, golden tests).
type JSONLSink struct {
	w   *bufio.Writer
	seq uint64
}

// NewJSONLSink writes hello as the first line.
func NewJSONLSink(w io.Writer, hello *protocol.Hello) (*JSONLSink, error) {
	s := &JSONLSink{w: bufio.NewWriter(w)}
	b, err := protocol.EncodeHello(hello)
	if err != nil {
		return nil, err
	}
	return s, s.line(b)
}

// Tick writes one tick with the next sequence number.
func (s *JSONLSink) Tick(t *protocol.Tick) error {
	s.seq++
	t.Seq = s.seq
	b, err := protocol.EncodeTick(t)
	if err != nil {
		return err
	}
	return s.line(b)
}

// Close flushes buffered output.
func (s *JSONLSink) Close() error { return s.w.Flush() }

func (s *JSONLSink) line(b []byte) error {
	if _, err := s.w.Write(b); err != nil {
		return err
	}
	return s.w.WriteByte('\n')
}

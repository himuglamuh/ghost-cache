package modem

// StreamDecoder finds zero-delimited frames. A malformed frame is reported and
// discarded; the next delimiter starts a clean frame.
type StreamDecoder struct {
	buffer  []byte
	discard bool
}

func (d *StreamDecoder) Push(data []byte) ([]Frame, []error) {
	var frames []Frame
	var errs []error
	for _, b := range data {
		if b == 0 {
			if d.discard {
				d.discard = false
				d.buffer = d.buffer[:0]
				continue
			}
			if len(d.buffer) == 0 {
				continue
			}
			frame, err := DecodeFrame(d.buffer)
			if err != nil {
				errs = append(errs, err)
			} else {
				frames = append(frames, frame)
			}
			d.buffer = d.buffer[:0]
			continue
		}
		if d.discard {
			continue
		}
		if len(d.buffer) >= MaxFramePayload+32 {
			d.discard = true
			d.buffer = d.buffer[:0]
			errs = append(errs, ErrMalformed)
			continue
		}
		d.buffer = append(d.buffer, b)
	}
	return frames, errs
}

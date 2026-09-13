package genx

import "errors"

type sliceStream struct {
	chunks  []*MessageChunk
	idx     int
	doneErr error
}

func (s *sliceStream) Next() (*MessageChunk, error) {
	if s.idx < len(s.chunks) {
		v := s.chunks[s.idx]
		s.idx++
		return v, nil
	}
	if s.doneErr == nil {
		return nil, ErrDone
	}
	return nil, s.doneErr
}

func (s *sliceStream) Close() error {
	return nil
}

func (s *sliceStream) CloseWithError(err error) error {
	if !errors.Is(err, ErrDone) {
		s.doneErr = err
	}
	return nil
}

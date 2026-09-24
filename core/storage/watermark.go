package storage

import (
	"bytes"
	"io"
)

// WriteWatermarked copies src to dst and inserts injection immediately
// before the last </body>. Lesson files are a few hundred kilobytes, but
// the copy still follows the storage response instead of buffering the
// whole document: only a short tail is held back so the marker can be
// split across two reads. When the document has no </body>, injection is
// written after the last byte.
func WriteWatermarked(dst io.Writer, src io.Reader, injection string) error {
	marker := []byte("</body>")
	hold := len(marker) - 1
	tail := make([]byte, 0, hold)
	buf := make([]byte, 32*1024)
	inject := []byte(injection)

	flush := func(b []byte) error {
		if len(b) == 0 {
			return nil
		}
		_, err := dst.Write(b)
		return err
	}

	for {
		n, err := src.Read(buf)
		chunk := make([]byte, 0, len(tail)+n)
		chunk = append(chunk, tail...)
		chunk = append(chunk, buf[:n]...)
		tail = tail[:0]

		if idx := bytes.LastIndex(chunk, marker); idx >= 0 {
			if err := flush(chunk[:idx]); err != nil {
				return err
			}
			if err := flush(inject); err != nil {
				return err
			}
			if err := flush(chunk[idx:]); err != nil {
				return err
			}
			_, copyErr := io.Copy(dst, src)
			if copyErr != nil {
				return copyErr
			}
			return nil
		}

		if len(chunk) > hold {
			if err := flush(chunk[:len(chunk)-hold]); err != nil {
				return err
			}
			tail = append(tail, chunk[len(chunk)-hold:]...)
		} else {
			tail = append(tail, chunk...)
		}

		if err == io.EOF {
			if err := flush(tail); err != nil {
				return err
			}
			return flush(inject)
		}
		if err != nil {
			return err
		}
	}
}

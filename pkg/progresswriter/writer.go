package progresswriter

import (
	"io"
	"sync/atomic"
)

// ProgressWriter wraps an io.Writer to track bytes written
type ProgressWriter struct {
	writer      io.Writer
	total       int64
	written     int64
	onProgress  func(written, total int64)
}

// NewProgressWriter creates a new progress tracking writer
func NewProgressWriter(writer io.Writer, total int64, onProgress func(written, total int64)) *ProgressWriter {
	return &ProgressWriter{
		writer:     writer,
		total:      total,
		onProgress: onProgress,
	}
}

// Write implements io.Writer
func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	if n > 0 {
		written := atomic.AddInt64(&pw.written, int64(n))
		if pw.onProgress != nil {
			pw.onProgress(written, pw.total)
		}
	}
	return n, err
}

// Written returns total bytes written
func (pw *ProgressWriter) Written() int64 {
	return atomic.LoadInt64(&pw.written)
}

// Total returns expected total bytes
func (pw *ProgressWriter) Total() int64 {
	return pw.total
}

// Progress returns current progress percentage
func (pw *ProgressWriter) Progress() int {
	if pw.total == 0 {
		return 0
	}
	return int((pw.Written() * 100) / pw.total)
}

// ProgressReader wraps an io.Reader to track bytes read
type ProgressReader struct {
	reader     io.Reader
	total      int64
	read       int64
	onProgress func(read, total int64)
}

// NewProgressReader creates a new progress tracking reader
func NewProgressReader(reader io.Reader, total int64, onProgress func(read, total int64)) *ProgressReader {
	return &ProgressReader{
		reader:     reader,
		total:      total,
		onProgress: onProgress,
	}
}

// Read implements io.Reader
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		read := atomic.AddInt64(&pr.read, int64(n))
		if pr.onProgress != nil {
			pr.onProgress(read, pr.total)
		}
	}
	return n, err
}

// Read returns total bytes read
func (pr *ProgressReader) ReadBytes() int64 {
	return atomic.LoadInt64(&pr.read)
}

// Progress returns current progress percentage
func (pr *ProgressReader) Progress() int {
	if pr.total == 0 {
		return 0
	}
	return int((pr.ReadBytes() * 100) / pr.total)
}

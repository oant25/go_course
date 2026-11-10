package response

import (
	"bytes"
	"errors"
	"io"
	"net/http"
)

var ErrTooLarge = errors.New("response exceeds buffer limit")

type Buffer struct {
	header http.Header
	body   bytes.Buffer
	status int
	limit  int
	err    error
}

func New(limit int) *Buffer           { return &Buffer{header: make(http.Header), limit: limit} }
func (b *Buffer) Header() http.Header { return b.header }
func (b *Buffer) WriteHeader(status int) {
	if status < 100 || status > 999 {
		panic("invalid HTTP response status")
	}
	if b.status != 0 || status < 200 {
		return
	}
	b.status = status
}

func (b *Buffer) Write(data []byte) (int, error) {
	if b.err != nil {
		return 0, b.err
	}
	if len(data) > b.limit-b.body.Len() {
		b.err = ErrTooLarge
		return 0, b.err
	}
	if b.status == 0 {
		b.status = http.StatusOK
	}
	return b.body.Write(data)
}
func (b *Buffer) Err() error { return b.err }
func (b *Buffer) Reset() {
	clear(b.header)
	b.body.Reset()
	b.status = 0
	b.err = nil
}

func (b *Buffer) Commit(w http.ResponseWriter) error {
	if b.err != nil {
		return b.err
	}
	for key, values := range b.header {
		w.Header()[key] = append([]string(nil), values...)
	}
	status := b.status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, err := io.Copy(w, bytes.NewReader(b.body.Bytes()))
	return err
}

func (b *Buffer) Status() int {
	if b.status == 0 {
		return 200
	}
	return b.status
}

// Copyright © 2018 The wormhole-connector authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package streamio

import (
	"io"
	"net/http"
	"sync"
)

// DualStream copies data from r1 to w1 and from r2 to w2, flushes as needed,
// and returns when both streams are done, closing the Readers/Writers if they
// implement the io.Closer interface.
//
// bufferPool should be a *sync.Pool of byte arrays.
func DualStream(w1 io.Writer, r1 io.Reader, w2 io.Writer, r2 io.Reader, bufferPool *sync.Pool) error {
	errChan := make(chan error)

	stream := func(w io.Writer, r io.Reader) {
		buf := bufferPool.Get().([]byte)
		buf = buf[0:cap(buf)]

		wCloser, ok := w.(io.Closer)
		if ok {
			defer wCloser.Close()
		}
		rCloser, ok := r.(io.Closer)
		if ok {
			defer rCloser.Close()
		}

		_, err := flushingIoCopy(w, r, buf)

		errChan <- err
	}

	go stream(w1, r1)
	go stream(w2, r2)

	err1 := <-errChan
	err2 := <-errChan
	if err1 != nil {
		return err1
	}
	return err2
}

// flushingIoCopy is like io.CopyBuffer() but also attempts to flush on each
// iteration (since, e.g. io.Writer doesn't flush by default).
//
// If dst does not implement http.Flusher (e.g. net.TCPConn), it will do a
// simple io.CopyBuffer().
func flushingIoCopy(dst io.Writer, src io.Reader, buf []byte) (written int64, err error) {
	flusher, ok := dst.(http.Flusher)
	if !ok {
		return io.CopyBuffer(dst, src, buf)
	}
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			flusher.Flush()
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return
}

package connector

import (
	"io"
	"net/http"
)

// flushingIoCopy is analogous to buffering io.Copy(), but also attempts to
// flush on each iteration. If dst does not implement http.Flusher (e.g.
// net.TCPConn), it will do a simple io.CopyBuffer(). Reasoning:
// http2ResponseWriter will not flush on its own, so we have to do it manually.
func flushingIoCopy(dst io.Writer, src io.Reader, buf []byte) (written int64, err error) {
	dstCloser, ok := dst.(io.Closer)
	if ok {
		defer dstCloser.Close()
	}
	srcCloser, ok := src.(io.Closer)
	if ok {
		defer srcCloser.Close()
	}

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

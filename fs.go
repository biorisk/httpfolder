// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// HTTP file system request handler

package main

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// A Dir implements http.FileSystem using the native file
// system restricted to a specific directory tree.
//
// An empty Dir is treated as ".".
type Dir string
const (
	sniffLen = 512
	StatusRequestedRangeNotSatisfiable = 416
	StatusPartialContent = 206
	StatusNotModified = 304
	StatusMovedPermanently  = 301
)

// func main() {
// 	http.Handle("/", http.StripPrefix("/", http.FileServer(http.Dir("."))))
// 	http.ListenAndServe(":8181", nil)
// }

func (d Dir) Open(name string) (File, string, error) {
	if filepath.Separator != '/' && strings.IndexRune(name, filepath.Separator) >= 0 ||
		strings.Contains(name, "\x00") {
		return nil, "", errors.New("http: invalid character in file path")
	}
	dir := string(d)
	if dir == "" {
		dir = "."
	}
	fullPath := filepath.Join(dir, filepath.FromSlash(path.Clean("/"+name)))
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, "", err
	}
	return f, fullPath, nil
}

// A FileSystem implements access to a collection of named files.
// The elements in a file path are separated by slash ('/', U+002F)
// characters, regardless of host operating system convention.
type FileSystem interface {
	Open(name string) (File, string, error)
}

// A File is returned by a FileSystem's Open method and can be
// served by the FileServer implementation.
type File interface {
	Close() error
	Stat() (os.FileInfo, error)
	Readdir(count int) ([]os.FileInfo, error)
	Read([]byte) (int, error)
	Seek(offset int64, whence int) (int64, error)
}

func dirList(w http.ResponseWriter, f File, fullPath string, atRoot bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<b>%s/</b> ", fullPath)
	fmt.Fprintf(w, "<a href=\"?form=form.html\">upload here</a> <a href=\"?images=1\">view images</a><br>\n")
	fmt.Fprintf(w, "<pre>\n")
	if !atRoot {
		fmt.Fprintf(w, "<a href=\"%s\">%s</a>\n", "..", "..") //go up one directory, will not go past base directory
	}
	for {
		dirs, err := f.Readdir(100)
		if err != nil || len(dirs) == 0 {
			break
		}
		for _, d := range dirs {
			name := d.Name()
			if d.IsDir() {
				name += "/"
			}
			// TODO htmlescape
			fmt.Fprintf(w, "<a href=\"%s\">%s</a>\n", name, name)
		}
	}
	fmt.Fprintf(w, "</pre>\n")
}

func dirListImages(w http.ResponseWriter, f File, fullPath string, atRoot bool) {
       validImg := map[string]bool{ ".jpg": true, ".png": true}
       w.Header().Set("Content-Type", "text/html; charset=utf-8")
       fmt.Fprintf(w, "<b>%s/</b> ", fullPath)
       fmt.Fprintf(w, "<a href=\"?form=form.html\">upload here</a> <a href=\".\">hide images</a><br>\n")
       fmt.Fprintf(w, "<pre>\n")
       if !atRoot {
               fmt.Fprintf(w, "<a href=\"%s\">%s</a>\n", "..", "..") //go up one directory, will not go past base directory
       }
       for {
               dirs, err := f.Readdir(100)
               if err != nil || len(dirs) == 0 {
                       break
               }
               for _, d := range dirs {
                       name := d.Name()
                       if d.IsDir() {
                               name += "/"
                       }
                       ext := strings.ToLower(name[len(name)-4:])
                       // TODO htmlescape
                       fmt.Fprintf(w, "<a href=\"%s\">%s</a>\n", name, name)
                       if _, ok := validImg[ext]; ok {
                               fmt.Fprintf(w, "<img src=\"./%s\">\n", name)
                       }
               }
       }
       fmt.Fprintf(w, "</pre>\n")
}

// ServeContent replies to the request using the content in the
// provided ReadSeeker.  The main benefit of ServeContent over io.Copy
// is that it handles Range requests properly, sets the MIME type, and
// handles If-Modified-Since requests.
//
// If the response's Content-Type header is not set, ServeContent
// first tries to deduce the type from name's file extension and,
// if that fails, falls back to reading the first block of the content
// and passing it to DetectContentType.
// The name is otherwise unused; in particular it can be empty and is
// never sent in the response.
//
// If modtime is not the zero time, ServeContent includes it in a
// Last-Modified header in the response.  If the request includes an
// If-Modified-Since header, ServeContent uses modtime to decide
// whether the content needs to be sent at all.
//
// The content's Seek method must work: ServeContent uses
// a seek to the end of the content to determine its size.
//
// If the caller has set w's ETag header, ServeContent uses it to
// handle requests using If-Range and If-None-Match.
//
// Note that *os.File implements the io.ReadSeeker interface.
func ServeContent(w http.ResponseWriter, req *http.Request, name string, modtime time.Time, content io.ReadSeeker) {
	sizeFunc := func() (int64, error) {
		size, err := content.Seek(0, os.SEEK_END)
		if err != nil {
			return 0, errSeeker
		}
		_, err = content.Seek(0, os.SEEK_SET)
		if err != nil {
			return 0, errSeeker
		}
		return size, nil
	}
	serveContent(w, req, name, modtime, sizeFunc, content)
}

// errSeeker is returned by ServeContent's sizeFunc when the content
// doesn't seek properly. The underlying Seeker's error text isn't
// included in the sizeFunc reply so it's not sent over HTTP to end
// users.
var errSeeker = errors.New("seeker can't seek")

// if name is empty, filename is unknown. (used for mime type, before sniffing)
// if modtime.IsZero(), modtime is unknown.
// content must be seeked to the beginning of the file.
// The sizeFunc is called at most once. Its error, if any, is sent in the HTTP response.
func serveContent(w http.ResponseWriter, r *http.Request, name string, modtime time.Time, sizeFunc func() (int64, error), content io.ReadSeeker) {
	if checkLastModified(w, r, modtime) {
		return
	}
	rangeReq, done := checkETag(w, r)
	if done {
		return
	}

	code := http.StatusOK

	// If Content-Type isn't set, use the file's extension to find it, but
	// if the Content-Type is unset explicitly, do not sniff the type.
	ctypes, haveType := w.Header()["Content-Type"]
	var ctype string
	if !haveType {
		ctype = mime.TypeByExtension(filepath.Ext(name))
		if ctype == "" {
			// read a chunk to decide between utf-8 text and binary
			var buf [sniffLen]byte
			n, _ := io.ReadFull(content, buf[:])
			ctype = http.DetectContentType(buf[:n])
			_, err := content.Seek(0, os.SEEK_SET) // rewind to output whole file
			if err != nil {
				http.Error(w, "seeker can't seek", http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", ctype)
	} else if len(ctypes) > 0 {
		ctype = ctypes[0]
	}

	size, err := sizeFunc()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// handle Content-Range header.
	sendSize := size
	var sendContent io.Reader = content
	if size >= 0 {
		ranges, err := parseRange(rangeReq, size)
		if err != nil {
			http.Error(w, err.Error(), StatusRequestedRangeNotSatisfiable)
			return
		}
		if sumRangesSize(ranges) > size {
			// The total number of bytes in all the ranges
			// is larger than the size of the file by
			// itself, so this is probably an attack, or a
			// dumb client.  Ignore the range request.
			ranges = nil
		}
		switch {
		case len(ranges) == 1:
			// RFC 2616, Section 14.16:
			// "When an HTTP message includes the content of a single
			// range (for example, a response to a request for a
			// single range, or to a request for a set of ranges
			// that overlap without any holes), this content is
			// transmitted with a Content-Range header, and a
			// Content-Length header showing the number of bytes
			// actually transferred.
			// ...
			// A response to a request for a single range MUST NOT
			// be sent using the multipart/byteranges media type."
			ra := ranges[0]
			if _, err := content.Seek(ra.start, os.SEEK_SET); err != nil {
				http.Error(w, err.Error(), StatusRequestedRangeNotSatisfiable)
				return
			}
			sendSize = ra.length
			code = StatusPartialContent
			w.Header().Set("Content-Range", ra.contentRange(size))
		case len(ranges) > 1:
			for _, ra := range ranges {
				if ra.start > size {
					http.Error(w, err.Error(), StatusRequestedRangeNotSatisfiable)
					return
				}
			}
			sendSize = rangesMIMESize(ranges, ctype, size)
			code = StatusPartialContent

			pr, pw := io.Pipe()
			mw := multipart.NewWriter(pw)
			w.Header().Set("Content-Type", "multipart/byteranges; boundary="+mw.Boundary())
			sendContent = pr
			defer pr.Close() // cause writing goroutine to fail and exit if CopyN doesn't finish.
			go func() {
				for _, ra := range ranges {
					part, err := mw.CreatePart(ra.mimeHeader(ctype, size))
					if err != nil {
						pw.CloseWithError(err)
						return
					}
					if _, err := content.Seek(ra.start, os.SEEK_SET); err != nil {
						pw.CloseWithError(err)
						return
					}
					if _, err := io.CopyN(part, content, ra.length); err != nil {
						pw.CloseWithError(err)
						return
					}
				}
				mw.Close()
				pw.Close()
			}()
		}

		w.Header().Set("Accept-Ranges", "bytes")
		if w.Header().Get("Content-Encoding") == "" {
			w.Header().Set("Content-Length", strconv.FormatInt(sendSize, 10))
		}
	}

	w.WriteHeader(code)

	if r.Method != "HEAD" {
		io.CopyN(w, sendContent, sendSize)
	}
}

// modtime is the modification time of the resource to be served, or IsZero().
// return value is whether this request is now complete.
func checkLastModified(w http.ResponseWriter, r *http.Request, modtime time.Time) bool {
	if modtime.IsZero() {
		return false
	}

	// The Date-Modified header truncates sub-second precision, so
	// use mtime < t+1s instead of mtime <= t to check for unmodified.
	if t, err := time.Parse(http.TimeFormat, r.Header.Get("If-Modified-Since")); err == nil && modtime.Before(t.Add(1*time.Second)) {
		h := w.Header()
		delete(h, "Content-Type")
		delete(h, "Content-Length")
		w.WriteHeader(StatusNotModified)
		return true
	}
	w.Header().Set("Last-Modified", modtime.UTC().Format(http.TimeFormat))
	return false
}

// checkETag implements If-None-Match and If-Range checks.
// The ETag must have been previously set in the http.ResponseWriter's headers.
//
// The return value is the effective request "Range" header to use and
// whether this request is now considered done.
func checkETag(w http.ResponseWriter, r *http.Request) (rangeReq string, done bool) {
	etag := w.Header().Get("Etag")
	rangeReq = r.Header.Get("Range")

	// Invalidate the range request if the entity doesn't match the one
	// the client was expecting.
	// "If-Range: version" means "ignore the Range: header unless version matches the
	// current file."
	// We only support ETag versions.
	// The caller must have set the ETag on the response already.
	if ir := r.Header.Get("If-Range"); ir != "" && ir != etag {
		// TODO(bradfitz): handle If-Range requests with Last-Modified
		// times instead of ETags? I'd rather not, at least for
		// now. That seems like a bug/compromise in the RFC 2616, and
		// I've never heard of anybody caring about that (yet).
		rangeReq = ""
	}

	if inm := r.Header.Get("If-None-Match"); inm != "" {
		// Must know ETag.
		if etag == "" {
			return rangeReq, false
		}

		// TODO(bradfitz): non-GET/HEAD requests require more work:
		// sending a different status code on matches, and
		// also can't use weak cache validators (those with a "W/
		// prefix).  But most users of ServeContent will be using
		// it on GET or HEAD, so only support those for now.
		if r.Method != "GET" && r.Method != "HEAD" {
			return rangeReq, false
		}

		// TODO(bradfitz): deal with comma-separated or multiple-valued
		// list of If-None-match values.  For now just handle the common
		// case of a single item.
		if inm == etag || inm == "*" {
			h := w.Header()
			delete(h, "Content-Type")
			delete(h, "Content-Length")
			w.WriteHeader(StatusNotModified)
			return "", true
		}
	}
	return rangeReq, false
}

// name is '/'-separated, not filepath.Separator.
func serveFile(w http.ResponseWriter, r *http.Request, fs FileSystem, name string, redirect bool) {
// 	const indexPage = "/index.html"

	// redirect .../index.html to .../
	// can't use Redirect() because that would make the path absolute,
	// which would be a problem running under StripPrefix
// 	if strings.HasSuffix(r.URL.Path, indexPage) {
// 		localRedirect(w, r, "./")
// 		return
// 	}

	f, fullPath, err := fs.Open(name)
	if err != nil {
		// TODO expose actual error?
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	d, err1 := f.Stat()
	if err1 != nil {
		// TODO expose actual error?
		http.NotFound(w, r)
		return
	}

	if redirect {
		// redirect to canonical path: / at end of directory url
		// r.URL.Path always begins with /
		url := r.URL.Path
		if d.IsDir() {
			if url[len(url)-1] != '/' {
				localRedirect(w, r, path.Base(url)+"/")
				return
			}
		} else {
			if url[len(url)-1] == '/' {
				localRedirect(w, r, "../"+path.Base(url))
				return
			}
		}
	}

	// use contents of index.html for directory, if present
// 	if d.IsDir() {
// 		index := name + indexPage
// 		ff, err := fs.Open(index)
// 		if err == nil {
// 			defer ff.Close()
// 			dd, err := ff.Stat()
// 			if err == nil {
// 				name = index
// 				d = dd
// 				f = ff
// 			}
// 		}
// 	}

	// Still a directory? (we didn't find an index.html file)
	if d.IsDir() {
		atRoot := false
		if checkLastModified(w, r, d.ModTime()) {
			return
		}
 		if name=="/" {
 			atRoot = true
 		}
		if _, ok := r.URL.Query()["upload"]; ok {
			fmt.Println("upload")
			receiveUpload(w, r, fullPath)
			return
		}
		if form, ok := r.URL.Query()["form"]; ok {
			fmt.Println("form")
			sendForm(w, fullPath, form[0])
			return
		}
		if _, ok := r.URL.Query()["images"]; ok {
                       dirListImages(w, f, fullPath, atRoot)
                       return
                }

		dirList(w, f, fullPath, atRoot)
		return
	}

	// serverContent will check modification time
	sizeFunc := func() (int64, error) { return d.Size(), nil }
	serveContent(w, r, d.Name(), d.ModTime(), sizeFunc, f)
}

// localRedirect gives a Moved Permanently response.
// It does not convert relative paths to absolute paths like Redirect does.
func localRedirect(w http.ResponseWriter, r *http.Request, newPath string) {
	if q := r.URL.RawQuery; q != "" {
		newPath += "?" + q
	}
	w.Header().Set("Location", newPath)
	w.WriteHeader(StatusMovedPermanently)
}

// ServeFile replies to the request with the contents of the named file or directory.
func ServeFile(w http.ResponseWriter, r *http.Request, name string) {
	dir, file := filepath.Split(name)
	serveFile(w, r, Dir(dir), file, false)
}

type fileHandler struct {
	root FileSystem
}

// FileServer returns a handler that serves HTTP requests
// with the contents of the file system rooted at root.
//
// To use the operating system's file system implementation,
// use http.Dir:
//
//     http.Handle("/", http.FileServer(http.Dir("/tmp")))
func FileServer(root FileSystem) http.Handler {
	return &fileHandler{root}
}

func (f *fileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for key, value := range r.URL.Query() {
		fmt.Println("Key:", key, "Value:", value)
	}
	fmt.Printf("got: %s\n", r.URL.Path)
	upath := r.URL.Path
	if !strings.HasPrefix(upath, "/") {
		upath = "/" + upath
		r.URL.Path = upath
	}
// 	if _, ok := r.URL.Query()["upload"]; ok {
// 		fmt.Println("upload")
// 		receiveUpload(w, r, f.root, path.Clean(upath))
// 	}
	serveFile(w, r, f.root, path.Clean(upath), true)
}

// httpRange specifies the byte range to be sent to the client.
type httpRange struct {
	start, length int64
}

func (r httpRange) contentRange(size int64) string {
	return fmt.Sprintf("bytes %d-%d/%d", r.start, r.start+r.length-1, size)
}

func (r httpRange) mimeHeader(contentType string, size int64) textproto.MIMEHeader {
	return textproto.MIMEHeader{
		"Content-Range": {r.contentRange(size)},
		"Content-Type":  {contentType},
	}
}

// parseRange parses a Range header string as per RFC 2616.
func parseRange(s string, size int64) ([]httpRange, error) {
	if s == "" {
		return nil, nil // header not present
	}
	const b = "bytes="
	if !strings.HasPrefix(s, b) {
		return nil, errors.New("invalid range")
	}
	var ranges []httpRange
	for _, ra := range strings.Split(s[len(b):], ",") {
		ra = strings.TrimSpace(ra)
		if ra == "" {
			continue
		}
		i := strings.Index(ra, "-")
		if i < 0 {
			return nil, errors.New("invalid range")
		}
		start, end := strings.TrimSpace(ra[:i]), strings.TrimSpace(ra[i+1:])
		var r httpRange
		if start == "" {
			// If no start is specified, end specifies the
			// range start relative to the end of the file.
			i, err := strconv.ParseInt(end, 10, 64)
			if err != nil {
				return nil, errors.New("invalid range")
			}
			if i > size {
				i = size
			}
			r.start = size - i
			r.length = size - r.start
		} else {
			i, err := strconv.ParseInt(start, 10, 64)
			if err != nil || i > size || i < 0 {
				return nil, errors.New("invalid range")
			}
			r.start = i
			if end == "" {
				// If no end is specified, range extends to end of the file.
				r.length = size - r.start
			} else {
				i, err := strconv.ParseInt(end, 10, 64)
				if err != nil || r.start > i {
					return nil, errors.New("invalid range")
				}
				if i >= size {
					i = size - 1
				}
				r.length = i - r.start + 1
			}
		}
		ranges = append(ranges, r)
	}
	return ranges, nil
}

// countingWriter counts how many bytes have been written to it.
type countingWriter int64

func (w *countingWriter) Write(p []byte) (n int, err error) {
	*w += countingWriter(len(p))
	return len(p), nil
}

// rangesMIMESize returns the nunber of bytes it takes to encode the
// provided ranges as a multipart response.
func rangesMIMESize(ranges []httpRange, contentType string, contentSize int64) (encSize int64) {
	var w countingWriter
	mw := multipart.NewWriter(&w)
	for _, ra := range ranges {
		mw.CreatePart(ra.mimeHeader(contentType, contentSize))
		encSize += ra.length
	}
	mw.Close()
	encSize += int64(w)
	return
}

func sumRangesSize(ranges []httpRange) (size int64) {
	for _, ra := range ranges {
		size += ra.length
	}
	return
}

// code to receive posted forms with large file uploads by streaming to disk then parsing.
func receiveUpload(w http.ResponseWriter, req *http.Request, dir string) { //need to add current directory here
	reader, err := req.MultipartReader()
	if err != nil {
		fmt.Println(err)
		http.Error(w, "not a form", http.StatusBadRequest)
	}
	form, err := reader.ReadForm(100000)
	defer form.RemoveAll()
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println("incoming form")
//change this line to dump uploaded files in current directory
	if len(form.File) > 0 { //check to make sure form has attached files before creating directory
		fmt.Printf("uploading file to directory: %s\n", dir)
	}
	fmt.Fprint(w, "<html><body><h2>Uploaded</h2>\n")
	for k, files := range form.File { //loop through each file selector in submitted form
		for i := range files { //loop through each file from current file selector
			fmt.Printf("key: %s  value: %s\n", k, files[i].Filename)
			srcfile, err := files[i].Open()
			if err != nil {
				fmt.Println(err)
			}
			dstfile, err := os.Create(fmt.Sprintf("%s/%s", dir, files[i].Filename))
			if err != nil {
				fmt.Println(err)
			}
			sizecopied, err := io.Copy(dstfile, srcfile)
			if err != nil {
				fmt.Println(err)
			}
			fmt.Printf("file size: %d\n", sizecopied)
			fmt.Fprintf(w, "<b>%s</b>: %d bytes<br>\n", files[i].Filename, sizecopied)
			srcfile.Close()
			dstfile.Close()
		}
	}
	fmt.Fprint(w, "<p><h2>Done uploading files.</h2><a href=\".\">return to folder</a></body></html>")
}


func sendForm(w http.ResponseWriter, dir string, form string) {
	data, err := Asset("html/" + form)
	if err != nil {
		fmt.Println(err)
		return
	}
	out := string(data)
	if form=="form.html" {
		out = strings.Replace(out,"<!--current_directory-->","to " + dir,1)
	}
	fmt.Fprint(w, out)
}

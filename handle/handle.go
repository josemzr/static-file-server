package handle

import (
	"crypto/md5"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
)

var (
	// These assignments are for unit testing.
	listenAndServe    = http.ListenAndServe
	listenAndServeTLS = defaultListenAndServeTLS
	setHandler        = http.HandleFunc
)

var (
	// Server options to be set prior to calling the listening function.
	// minTLSVersion is the minimum allowed TLS version to be used by the
	// server.
	minTLSVersion uint16 = tls.VersionTLS10
)

// defaultListenAndServeTLS is the default implementation of the listening
// function for serving with TLS enabled. This is, effectively, a copy from
// the standard library but with the ability to set the minimum TLS version.
func defaultListenAndServeTLS(
	binding, certFile, keyFile string, handler http.Handler,
) error {
	if handler == nil {
		handler = http.DefaultServeMux
	}
	server := &http.Server{
		Addr:    binding,
		Handler: handler,
		TLSConfig: &tls.Config{
			MinVersion: minTLSVersion,
		},
	}
	return server.ListenAndServeTLS(certFile, keyFile)
}

// SetMinimumTLSVersion to be used by the server.
func SetMinimumTLSVersion(version uint16) {
	if version < tls.VersionTLS10 {
		version = tls.VersionTLS10
	} else if version > tls.VersionTLS13 {
		version = tls.VersionTLS13
	}
	minTLSVersion = version
}

// ListenerFunc accepts the {hostname:port} binding string required by HTTP
// listeners and the handler (router) function and returns any errors that
// occur.
type ListenerFunc func(string, http.HandlerFunc) error

// FileServerFunc is used to serve the file from the local file system to the
// requesting client.
type FileServerFunc func(http.ResponseWriter, *http.Request, string)

// WithReferrers returns a function that evaluates the HTTP 'Referer' header
// value and returns HTTP error 403 if the value is not found in the whitelist.
// If one of the whitelisted referrers are an empty string, then it is allowed
// for the 'Referer' HTTP header key to not be set.
func WithReferrers(serveFile FileServerFunc, referrers []string) FileServerFunc {
	return func(w http.ResponseWriter, r *http.Request, name string) {
		if !validReferrer(referrers, r.Referer()) {
			http.Error(
				w,
				fmt.Sprintf("Invalid source '%s'", r.Referer()),
				http.StatusForbidden,
			)
			return
		}
		serveFile(w, r, name)
	}
}

// WithLogging returns a function that logs information about the request prior
// to serving the requested file.
func WithLogging(serveFile FileServerFunc) FileServerFunc {
	return func(w http.ResponseWriter, r *http.Request, name string) {
		referer := r.Referer()
		if len(referer) == 0 {
			log.Printf(
				"REQ from '%s': %s %s %s%s -> %s\n",
				r.RemoteAddr,
				r.Method,
				r.Proto,
				r.Host,
				r.URL.Path,
				name,
			)
		} else {
			log.Printf(
				"REQ from '%s' (REFERER: '%s'): %s %s %s%s -> %s\n",
				r.RemoteAddr,
				referer,
				r.Method,
				r.Proto,
				r.Host,
				r.URL.Path,
				name,
			)
		}
		serveFile(w, r, name)
	}
}

// Basic file handler servers files from the passed folder.
func Basic(serveFile FileServerFunc, folder string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		serveFile(w, r, folder+r.URL.Path)
	}
}

// WithUploads wraps an HTTP handler so that PUT and POST requests write the
// request body into the upload directory (by default `folder/uploads`).
// Uploads are addressed either with or without the upload prefix in the URL:
//
//	PUT /uploads/file.txt    -> <folder>/uploads/file.txt
//	PUT /file.txt            -> <folder>/uploads/file.txt
//
// The response body is the absolute URL of the uploaded file (e.g.
// "https://host/uploads/file.txt"). The URL scheme is taken from the
// X-Forwarded-Proto header when present (reverse proxies) or from the
// request's TLS state; the host from the request. Pass uploadURLBase to
// override the detected base (e.g. "https://sfs.mcs.sh").
// Paths are sanitized so uploads cannot escape the upload directory.
// Upload subdirectories are created on demand. All other methods fall through
// to the wrapped handler.
func WithUploads(serve http.HandlerFunc, folder, uploadDir, uploadURLBase string) http.HandlerFunc {
	if uploadDir == "" {
		uploadDir = "uploads"
	}
	root := path.Join(folder, uploadDir)

	// Public URL prefix that clients may use to address uploads (e.g.
	// "/uploads/"). Stripped from the request path before joining, so the
	// prefix is not duplicated on disk.
	publicPrefix := path.Join("/", uploadDir) + "/"

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut && r.Method != http.MethodPost {
			serve(w, r)
			return
		}

		// Normalize the request path, stripping the public upload prefix if
		// present, then resolve against the upload root.
		clean := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
		hadPrefix := strings.HasPrefix(clean, publicPrefix)
		if hadPrefix {
			clean = path.Clean("/" + strings.TrimPrefix(clean, publicPrefix))
		}
		dest := path.Join(root, clean)
		if dest != root && !strings.HasPrefix(dest, root+"/") {
			http.Error(w, "invalid upload path", http.StatusBadRequest)
			return
		}

		// Create any missing parent directories.
		if err := os.MkdirAll(path.Dir(dest), 0755); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Stream the request body to the destination file.
		f, err := os.Create(dest)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer f.Close()
		if _, err := io.Copy(f, r.Body); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Build the absolute URL of the uploaded file.
		base := uploadURLBase
		if base == "" {
			proto := r.Header.Get("X-Forwarded-Proto")
			if proto == "" {
				proto = "http"
				if r.TLS != nil {
					proto = "https"
				}
			}
			base = proto + "://" + r.Host
		}
		// The published URL always carries the upload prefix, even when the
		// request was addressed without it.
		urlPath := path.Join("/", uploadDir, clean)
		url := strings.TrimSuffix(base, "/") + urlPath

		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, url)
	}
}

// Prefix file handler is an alternative to Basic where a URL prefix is removed
// prior to serving a file (http://my.machine/prefix/file.txt will serve
// file.txt from the root of the folder being served (ignoring 'prefix')).
func Prefix(serveFile FileServerFunc, folder, urlPrefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, urlPrefix) {
			http.NotFound(w, r)
			return
		}
		serveFile(w, r, folder+strings.TrimPrefix(r.URL.Path, urlPrefix))
	}
}

// PreventListings returns a function that prevents listing of directories but
// still allows index.html to be served.
func PreventListings(serve http.HandlerFunc, folder string, urlPrefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			// If the directory does not contain an index.html file, then
			// return 'NOT FOUND' to prevent listing of the directory.
			stat, err := os.Stat(path.Join(folder, strings.TrimPrefix(r.URL.Path, urlPrefix), "index.html"))
			if err != nil || (err == nil && !stat.Mode().IsRegular()) {
				http.NotFound(w, r)
				return
			}
		}
		serve(w, r)
	}
}

// IgnoreIndex wraps an HTTP request. In the event of a folder root request,
// this function will automatically return 'NOT FOUND' as opposed to default
// behavior where the index file for that directory is retrieved.
func IgnoreIndex(serve http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		serve(w, r)
	}
}

// AddCorsWildcardHeaders wraps an HTTP request to notify client browsers that
// resources should be allowed to be retrieved by any other domain.
func AddCorsWildcardHeaders(serve http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
		serve(w, r)
	}
}

// AddAccessKey provides Access Control through url parameters. The access key
// is set by ACCESS_KEY. md5sum is computed by queried path + access key
// (e.g. "/my/file" + ACCESS_KEY)
func AddAccessKey(serve http.HandlerFunc, accessKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get key or md5sum from this access.
		keys, keyOk := r.URL.Query()["key"]
		var code string
		if !keyOk || len(keys[0]) < 1 {
			// In case a code is provided
			codes, codeOk := r.URL.Query()["code"]
			if !codeOk || len(codes[0]) < 1 {
				http.NotFound(w, r)
				return
			}
			code = strings.ToUpper(codes[0])
		} else {
			// In case a key is provided, convert to code.
			data := []byte(r.URL.Path + keys[0])
			hash := md5.Sum(data)
			code = fmt.Sprintf("%X", hash)
		}

		// Compute the correct md5sum of this access.
		localData := []byte(r.URL.Path + accessKey)
		hash := md5.Sum(localData)
		localCode := fmt.Sprintf("%X", hash)

		// Compare the two.
		if code != localCode {
			http.NotFound(w, r)
			return
		}
		serve(w, r)
	}
}

// Listening function for serving the handler function.
func Listening() ListenerFunc {
	return func(binding string, handler http.HandlerFunc) error {
		setHandler("/", handler)
		return listenAndServe(binding, nil)
	}
}

// TLSListening function for serving the handler function with encryption.
func TLSListening(tlsCert, tlsKey string) ListenerFunc {
	return func(binding string, handler http.HandlerFunc) error {
		setHandler("/", handler)
		return listenAndServeTLS(binding, tlsCert, tlsKey, nil)
	}
}

// validReferrer returns true if the passed referrer can be resolved by the
// passed list of referrers.
func validReferrer(s []string, e string) bool {
	// Whitelisted referer list is empty. All requests are allowed.
	if len(s) == 0 {
		return true
	}

	for _, a := range s {
		// Handle blank HTTP Referer header, if configured
		if a == "" {
			if e == "" {
				return true
			}
			// Continue loop (all strings start with "")
			continue
		}

		// Compare header with allowed prefixes
		if strings.HasPrefix(e, a) {
			return true
		}
	}
	return false
}

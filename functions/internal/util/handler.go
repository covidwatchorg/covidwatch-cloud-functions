package util

import (
	"encoding/json"
	"log"
	"net/http"
)

// Handler is a handler for a request to this service. Use MakeHTTPHandler or
// MakeTestHTTPHandler to wrap a Handler with the logic necessary to produce a
// handler which can be registered with the "net/http" package.
type Handler = func(ctx *RequestContext) StatusError

// MakeHTTPHandler wraps a Handler, producing a handler which can be registered
// with the "net/http" package. The returned handler is responsible for:
//  - Constructing a *Context
//  - Converting any errors into an HTTP response
func MakeHTTPHandler(handler Handler) func(http.ResponseWriter, *http.Request) {
	return makeHTTPHandler(NewRequestContext, handler)
}

// MakeDevHTTPHandler is like MakeHTTPHandler, except that the generated handler
// will attempt to connect to the Firestore emulator at the
// FIRESTORE_EMULATOR_HOST environment variable rather than attempting to
// connect to a production Firestore instance.
func MakeDevHTTPHandler(handler Handler) func(http.ResponseWriter, *http.Request) {
	return makeHTTPHandler(NewDevRequestContext, handler)
}

func makeHTTPHandler(
	newContext func(w http.ResponseWriter, r *http.Request) (*RequestContext, StatusError),
	handler Handler,
) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// Add HSTS header.
		addHSTS(w)

		// Reject insecure HTTP requests.
		if err := checkHTTPS(r); err != nil {
			writeStatusError(w, r, err)
			return
		}

		ctx, err := newContext(w, r)
		if err != nil {
			writeStatusError(w, r, err)
			return
		}

		if err := handler(ctx); err != nil {
			writeStatusError(w, r, err)
		}
	}
}

func writeStatusError(w http.ResponseWriter, r *http.Request, err StatusError) {
	type response struct {
		Message string `json:"message"`
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(err.HTTPStatusCode())
	json.NewEncoder(w).Encode(response{Message: err.Message()})

	log.Printf("[%v %v]: responding with error code %v and message \"%v\" (error: %v)",
		r.Method, r.URL, err.HTTPStatusCode(), err.Message(), err)
}

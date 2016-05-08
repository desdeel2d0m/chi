package chi

import (
	"net/http"
	"strings"
)

// chain builds a http.Handler composed of middlewares and endpoint handler in the
// order they are passed.
func chain(middlewares []func(http.Handler) http.Handler, endpoint http.Handler) http.Handler {
	// Return ahead of time if there aren't any middlewares for the chain
	if middlewares == nil || len(middlewares) == 0 {
		return endpoint
	}

	// Wrap the end handler with the middleware chain
	h := middlewares[len(middlewares)-1](endpoint)
	for i := len(middlewares) - 2; i >= 0; i-- {
		h = middlewares[i](h)
	}

	return h
}

// Respond with just the allowed methods, as required by RFC2616 for
// 405 Method not allowed.
func methodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	methods := make([]string, len(methodMap))
	i := 0
	for m := range methodMap {
		methods[i] = m // still faster than append to array with capacity
		i++
	}

	w.Header().Add("Allow", strings.Join(methods, ","))
	w.WriteHeader(405)
	w.Write([]byte(http.StatusText(405)))
}

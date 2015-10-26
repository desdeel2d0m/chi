package middleware

import (
	"net/http"

	"github.com/pressly/chi"
	"golang.org/x/net/context"
)

// 499 Client Closed Request (Nginx)
// https://en.wikipedia.org/wiki/List_of_HTTP_status_codes
const StatusClientClosedRequest = 499

// CloseNotify cancels the ctx when the underlying connection has gone away.
// This middleware can be used to cancel long operations on the server
// if the client has disconnected before the response is ready.
func CloseNotify(next chi.Handler) chi.Handler {
	fn := func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		cn, ok := w.(http.CloseNotifier)
		if !ok {
			panic("middleware.CloseNotify expects http.ResponseWriter to implement http.CloseNotifier interface")
		}

		ctx, cancel := context.WithCancel(ctx)

		go func() {
			select {
			case <-ctx.Done():
				return
			case <-cn.CloseNotify():
				w.WriteHeader(StatusClientClosedRequest)
				cancel()
				return
			}
		}()

		next.ServeHTTPC(ctx, w, r)
	}

	return chi.HandlerFunc(fn)
}

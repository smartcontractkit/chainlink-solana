package monitoring

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"

	"github.com/smartcontractkit/chainlink/core/logger"
)

// HttpServer is the HTTP interface exposed by every monitoring.
// It's used to export metrics to prometheus, to query the node for configurations, etc.
type HttpServer interface {
	Handle(path string, handler http.Handler)
	Start(ctx context.Context, wg *sync.WaitGroup)
}

func NewHttpServer(baseCtx context.Context, addr string, log logger.Logger) HttpServer {
	mux := http.NewServeMux()
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
		BaseContext: func(_ net.Listener) context.Context {
			return baseCtx
		},
	}
	return &httpServer{srv, mux, log, addr}
}

type httpServer struct {
	server *http.Server
	mux    *http.ServeMux
	log    logger.Logger
	addr   string
}

func (h *httpServer) Handle(path string, handler http.Handler) {
	h.mux.Handle(path, handler)
}

// Start should be executed as a goroutine
func (h *httpServer) Start(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		h.log.Debug("starting http server")
		if err := h.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			h.log.Fatalw("failed to start http server", "address", h.addr, "error", err)
		} else {
			h.log.Info("http server stopped")
		}
	}()
	wg.Add(1)
	defer wg.Done()
	<-ctx.Done()
	if err := h.server.Shutdown(ctx); err != nil && !errors.Is(err, context.Canceled) {
		h.log.Errorw("failed to shut http server down", "error", err)
	}
}

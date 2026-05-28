package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/hotstream"
	"github.com/byte-v-forge/common-lib/httpsse"
	"github.com/byte-v-forge/common-lib/protojsonhttp"
	"google.golang.org/protobuf/proto"

	"mailboxapi/pb"
)

type dashboardServer struct {
	mailboxClient pb.MailboxServiceClient
	hotstream     hotstream.Subscriber
	staticDir     string
}

func startDashboardHTTP(ctx context.Context, listenAddr, staticDir string, mailboxClient pb.MailboxServiceClient, stream hotstream.Subscriber, errCh chan<- error) {
	if strings.TrimSpace(listenAddr) == "" {
		return
	}
	if strings.TrimSpace(staticDir) == "" {
		staticDir = "/app/dashboard/mailbox"
	}
	dashboard := &dashboardServer{mailboxClient: mailboxClient, hotstream: stream, staticDir: staticDir}
	mux := http.NewServeMux()
	mux.Handle("/api/mailbox/", http.StripPrefix("/api/mailbox", dashboard.routes()))
	mux.Handle("/mf/mailbox/", http.StripPrefix("/mf/mailbox/", noCacheFileServer(staticDir)))
	mux.HandleFunc("/healthz", dashboard.handleHealth)
	server := &http.Server{Addr: listenAddr, Handler: withCORS(mux), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	go func() {
		log.Printf("mailbox dashboard BFF listening on %s", listenAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("mailbox dashboard BFF failed: %w", err)
		}
	}()
}

func (s *dashboardServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/mailboxes/register", s.handleMailboxRegister)
	mux.HandleFunc("/mailboxes/oauth", s.handleMailboxOAuth)
	mux.HandleFunc("/mailboxes/inbox", s.handleMailboxInbox)
	mux.HandleFunc("/streams/state", s.streamState)
	mux.HandleFunc("/domains", s.handleMailboxDomains)
	mux.HandleFunc("/provider-capabilities", s.handleMailboxProviderCapabilities)
	mux.HandleFunc("/operations/", s.handleMailboxOperation)
	mux.HandleFunc("/operations", s.handleMailboxOperations)
	mux.HandleFunc("/mailboxes/", s.handleMailbox)
	mux.HandleFunc("/mailboxes", s.handleMailboxes)
	return mux
}

func (s *dashboardServer) streamState(w http.ResponseWriter, r *http.Request) {
	httpsse.ServeHotStream(w, r, s.hotstream, httpsse.FilterFromRequest(r, hotstream.Filter{
		SourceServices: []string{mailboxHotStreamSource},
	}), httpsse.ServeOptions{})
}

func (s *dashboardServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func readProtoJSON(r *http.Request, dst proto.Message) error {
	return protojsonhttp.ReadRequest(r, dst)
}

func writeProtoJSON(w http.ResponseWriter, status int, value proto.Message) {
	_ = protojsonhttp.WriteResponse(w, status, value)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,PUT,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func noCacheFileServer(dir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		path := filepath.Join(dir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			http.ServeFile(w, r, path)
			return
		}
		http.NotFound(w, r)
	})
}

func selfTarget(listenAddr string) string {
	addr := strings.TrimSpace(listenAddr)
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

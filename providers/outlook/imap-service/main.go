package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"google.golang.org/grpc"

	"outlookimapservice/pb"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := NewMailboxStore(ctx, envStr("PG_DSN", ""), envInt("OUTLOOK_ALIAS_RANDOM_LENGTH", defaultAliasTokenLength))
	if err != nil {
		log.Fatalf("initialize mailbox store: %v", err)
	}
	defer store.Close()

	watcher := NewMailWatcher(store)
	server := grpc.NewServer()
	pb.RegisterEmailServiceServer(server, &EmailService{store: store, watcher: watcher})
	errCh := make(chan error, 3)

	listenAddr := grpcListenAddr(envStr("LISTEN_ADDR", defaultListenAddr))
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("listen on %s: %v", listenAddr, err)
	}

	startWebhookServer(ctx, envStr("WEBHOOK_HTTP_ADDR", ""), store, watcher, errCh)
	if err := startFRP(ctx, errCh); err != nil {
		log.Fatalf("initialize embedded frpc: %v", err)
	}

	go func() {
		<-ctx.Done()
		logInfo("received stop signal; stopping")
		server.GracefulStop()
	}()

	go func() {
		logInfo("Starting Go Outlook mail gRPC server on %s", listenAddr)
		if err := server.Serve(listener); err != nil {
			if ctx.Err() != nil {
				return
			}
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		stop()
		log.Fatalf("runtime error: %v", err)
	}
}

func grpcListenAddr(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultListenAddr
	}
	return value
}

func logInfo(format string, args ...any) {
	log.Printf("[MAIL] "+format, args...)
}

func logWarning(format string, args ...any) {
	log.Printf("[MAIL] WARNING "+format, args...)
}

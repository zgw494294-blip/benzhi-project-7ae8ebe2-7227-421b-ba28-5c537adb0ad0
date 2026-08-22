package main

import (
	"context"
	"net/http"
	"time"
)

func shutdownServer(s interface{ Shutdown(context.Context) error }) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.Shutdown(ctx)
}
func isClosed(err error) bool { return err == http.ErrServerClosed }

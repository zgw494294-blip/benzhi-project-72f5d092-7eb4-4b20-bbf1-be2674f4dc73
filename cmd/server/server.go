package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"acoustic-annotation-release/internal/certificate"
	"acoustic-annotation-release/internal/httpapi"
	"acoustic-annotation-release/internal/persistence"
	"acoustic-annotation-release/internal/workflow"
)

type application struct {
	repository *persistence.Repository
	server     *http.Server
	listener   net.Listener
}

func buildApplication(configuration config, logger *slog.Logger) (*application, error) {
	repository, err := persistence.Open(configuration.dataDirectory)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", configuration.address)
	if err != nil {
		repository.Close()
		return nil, err
	}
	certificateService := certificate.NewService()
	workflowService := workflow.NewService(repository, certificateService)
	api := httpapi.New(workflowService, logger)
	server := &http.Server{Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 32 << 10}
	return &application{repository: repository, server: server, listener: listener}, nil
}

func (a *application) serve(errorsChannel chan<- error) {
	err := a.server.Serve(a.listener)
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	errorsChannel <- err
}

func (a *application) shutdown(context context.Context) error {
	serverError := a.server.Shutdown(context)
	repositoryError := a.repository.Close()
	if serverError != nil {
		return serverError
	}
	return repositoryError
}

func defaultLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

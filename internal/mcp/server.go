package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
)

const protocolVersion = "2025-11-25"
const maxJSONRPCLineBytes = 10 * 1024 * 1024

type Server struct {
	executor Executor
	tools    []Tool
	mu       sync.Mutex
	active   map[string]activeRequest
	canceled map[string]string
	sessions map[string]struct{}
}

type activeRequest struct {
	cancel context.CancelFunc
}

func NewServer(executor Executor) *Server {
	return &Server{
		executor: executor,
		tools:    defaultTools(),
		active:   map[string]activeRequest{},
		canceled: map[string]string{},
		sessions: map[string]struct{}{},
	}
}

func (s *Server) Serve(input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), maxJSONRPCLineBytes)
	writer := bufio.NewWriter(output)
	defer writer.Flush()
	var writerMu sync.Mutex
	var wg sync.WaitGroup
	var serveErr error
	var serveErrMu sync.Mutex

	writeMessage := func(message any) error {
		encoded, err := json.Marshal(message)
		if err != nil {
			return err
		}
		writerMu.Lock()
		defer writerMu.Unlock()
		if _, err := writer.Write(append(encoded, '\n')); err != nil {
			return err
		}
		return writer.Flush()
	}
	recordErr := func(err error) {
		if err == nil {
			return
		}
		serveErrMu.Lock()
		defer serveErrMu.Unlock()
		if serveErr == nil {
			serveErr = err
		}
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if requestKey, ok := asyncRequestKey([]byte(line)); ok {
			ctx, cancel := context.WithCancel(context.Background())
			s.markActive(requestKey, cancel)
			wg.Add(1)
			go func(raw string, key string) {
				defer wg.Done()
				defer s.clearRequest(key)
				response, shouldRespond, err := s.HandleMessageWithProgressContext(ctx, []byte(raw), func(notification any) error {
					if s.isCanceled(key) {
						return nil
					}
					return writeMessage(notification)
				})
				if err != nil {
					recordErr(err)
					return
				}
				if !shouldRespond || s.isCanceled(key) {
					return
				}
				recordErr(writeMessage(response))
			}(line, requestKey)
			continue
		}

		response, shouldRespond, err := s.HandleMessageWithProgress([]byte(line), writeMessage)
		if err != nil {
			return err
		}
		if !shouldRespond {
			continue
		}
		if err := writeMessage(response); err != nil {
			return err
		}
	}

	scanErr := scanner.Err()
	wg.Wait()
	if scanErr != nil {
		return scanErr
	}
	serveErrMu.Lock()
	defer serveErrMu.Unlock()
	return serveErr
}

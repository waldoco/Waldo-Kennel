package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters/harnesscommand"
	adapterpairing "github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters/harnesspairing"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/harnessconnection"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/harnesspairing"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

type harnessEndpointSet struct {
	PairingAddress, CommandAddress string
	pairing, command               net.Listener
	pairingDir, commandDir, runDir string
	once                           sync.Once
}

func (s *harnessEndpointSet) Close() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		if s.command != nil {
			_ = s.command.Close()
		}
		if s.pairing != nil {
			_ = s.pairing.Close()
		}
		_ = os.Remove(s.commandDir)
		_ = os.Remove(s.pairingDir)
		_ = os.Remove(s.runDir)
	})
}

func startHarnessEndpoints(ctx context.Context, dataDir string, store interface {
	ports.HarnessPairingChallengeStore
	ports.HarnessCommandStore
}, connections *harnessconnection.Kernel, pairing *harnesspairing.Coordinator, log *slog.Logger) (*harnessEndpointSet, error) {
	if store == nil || connections == nil || pairing == nil {
		return nil, fmt.Errorf("harness endpoints: store, connection kernel and pairing coordinator are required")
	}
	runtimeDir := filepath.Join(dataDir, "run")
	pairingDir := filepath.Join(runtimeDir, "harness-pairing")
	commandDir := filepath.Join(runtimeDir, "harness-command")
	pairingListener, pairingAddress, err := adapterpairing.Listen(pairingDir)
	if err != nil {
		return nil, fmt.Errorf("harness pairing listener: %w", err)
	}
	endpoints := &harnessEndpointSet{PairingAddress: pairingAddress, pairing: pairingListener, pairingDir: pairingDir, commandDir: commandDir, runDir: runtimeDir}
	pairingServer, err := adapterpairing.NewServer(pairingListener, adapterpairing.ServerConfig{Coordinator: pairing, IntentStore: store, Logger: log})
	if err != nil {
		endpoints.Close()
		return nil, err
	}
	commandListener, commandAddress, err := harnesscommand.Listen(commandDir)
	if err != nil {
		endpoints.Close()
		return nil, fmt.Errorf("harness command listener: %w", err)
	}
	endpoints.CommandAddress, endpoints.command = commandAddress, commandListener
	commandServer, err := harnesscommand.NewServer(commandListener, harnesscommand.ServerConfig{Connections: connections, Store: store})
	if err != nil {
		endpoints.Close()
		return nil, err
	}
	if log == nil {
		log = slog.Default()
	}
	log.Info("harness pairing: listening", "addr", pairingAddress)
	log.Info("harness command: listening", "addr", commandAddress)
	go func() {
		if err := pairingServer.Serve(ctx); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Warn("harness pairing: serve stopped with error", "err", err)
		}
	}()
	go func() {
		if err := commandServer.Serve(ctx); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Warn("harness command: serve stopped with error", "err", err)
		}
	}()
	return endpoints, nil
}

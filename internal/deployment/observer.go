package deployment

import (
	"context"
	"errors"
	"os"
)

// Observer is the complete read-only observation interface. It fetches an
// already-produced inventory document and has no stop/start/exec/log-action or
// other lifecycle method. M2 deliberately ships no Docker implementation.
type Observer interface {
	Observe(ctx context.Context) (Inventory, error)
}

// FakeObserver reads one explicit caller-supplied JSON document. It is a test
// fake, not a live observer: it never contacts Docker or a service.
type FakeObserver struct {
	Path string
}

// Observe implements Observer without performing any live probe.
func (f FakeObserver) Observe(ctx context.Context) (Inventory, error) {
	if f.Path == "" {
		return Inventory{}, errors.New("fake observer: path is required")
	}
	if ctx != nil {
		select {
		case <-ctx.Done():
			return Inventory{}, ctx.Err()
		default:
		}
	}
	b, err := os.ReadFile(f.Path)
	if err != nil {
		return Inventory{}, err
	}
	inv, err := DecodeInventory(b)
	if err != nil {
		return Inventory{}, err
	}
	return *inv, nil
}

var _ Observer = FakeObserver{}

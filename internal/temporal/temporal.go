// Package temporal opens the Temporal client and reports its reachability.
package temporal

import (
	"context"
	"fmt"
	"log/slog"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/log"
)

// NewClient returns a client that connects on first use. client.Dial connects
// eagerly and fails when the server is down, which would stop the process
// before it could report the condition through its readiness endpoint.
func NewClient(address, namespace string, logger *slog.Logger) (client.Client, error) {
	c, err := client.NewLazyClient(client.Options{
		HostPort:  address,
		Namespace: namespace,
		Logger:    log.NewStructuredLogger(logger),
	})
	if err != nil {
		return nil, fmt.Errorf("temporal: %w", err)
	}
	return c, nil
}

// Checker is the readiness check for Temporal.
type Checker struct {
	client client.Client
}

func NewChecker(c client.Client) Checker {
	return Checker{client: c}
}

func (Checker) Name() string { return "temporal" }

func (c Checker) Check(ctx context.Context) error {
	_, err := c.client.CheckHealth(ctx, &client.CheckHealthRequest{})
	return err
}

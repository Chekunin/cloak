package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newLogCmd() *cobra.Command {
	var follow bool
	var since string
	var secret string
	var eventType string
	var limit int

	c := &cobra.Command{
		Use:   "log",
		Short: "Show audit-log entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, _, ctx, cancel, err := dialBackground(true)
			if err != nil {
				return err
			}
			defer cancel()
			defer cli.Close()

			var sinceTime time.Time
			if since != "" {
				d, err := time.ParseDuration(since)
				if err != nil {
					return fmt.Errorf("invalid --since: %w", err)
				}
				sinceTime = time.Now().Add(-d)
			}

			print := func() error {
				entries, err := cli.AuditTail(ctx, limit)
				if err != nil {
					return err
				}
				for _, e := range entries {
					if !matchEntry(e, sinceTime, secret, eventType) {
						continue
					}
					if jsonOutput {
						b, _ := json.Marshal(e)
						fmt.Println(string(b))
					} else {
						fmt.Println(formatEntry(e))
					}
				}
				return nil
			}

			if err := print(); err != nil {
				return err
			}
			if !follow {
				return nil
			}
			t := time.NewTicker(1 * time.Second)
			defer t.Stop()
			for range t.C {
				if err := print(); err != nil {
					return err
				}
			}
			return nil
		},
	}
	c.Flags().BoolVar(&follow, "follow", false, "stream new entries as they arrive")
	c.Flags().StringVar(&since, "since", "", "only entries within the duration (e.g. 1h, 30m)")
	c.Flags().StringVar(&secret, "secret", "", "filter to a single secret name")
	c.Flags().StringVar(&eventType, "type", "", "filter by event type prefix")
	c.Flags().IntVar(&limit, "limit", 200, "max entries per fetch")
	return c
}

func matchEntry(e map[string]any, since time.Time, secret, eventType string) bool {
	if !since.IsZero() {
		if ts, ok := e["ts"].(string); ok {
			t, err := time.Parse(time.RFC3339Nano, ts)
			if err == nil && t.Before(since) {
				return false
			}
		}
	}
	if secret != "" {
		if name, _ := e["secret_name"].(string); name != secret {
			return false
		}
	}
	if eventType != "" {
		ev, _ := e["event"].(string)
		if !strings.HasPrefix(ev, eventType) {
			return false
		}
	}
	return true
}

func formatEntry(e map[string]any) string {
	ts, _ := e["ts"].(string)
	ev, _ := e["event"].(string)
	name, _ := e["secret_name"].(string)
	addr, _ := e["remote_addr"].(string)
	var extra string
	if d, ok := e["details"].(map[string]any); ok && len(d) > 0 {
		b, _ := json.Marshal(d)
		extra = " " + string(b)
	}
	parts := []string{ts, ev}
	if name != "" {
		parts = append(parts, "name="+name)
	}
	if addr != "" {
		parts = append(parts, "remote="+addr)
	}
	return strings.Join(parts, "\t") + extra
}

var _ = os.Stdout

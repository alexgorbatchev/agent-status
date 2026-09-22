package store

import (
	"fmt"
)

func (s *Store) PruneOldEventsAndExitedAgents(retentionCutoffMs int64, exitedRetentionCutoffMs int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// 1. Delete events older than retention cutoff (e.g. 48 hours)
	_, err = tx.Exec("DELETE FROM telemetry_events WHERE timestamp < ?", retentionCutoffMs)
	if err != nil {
		return fmt.Errorf("deleting old telemetry events: %w", err)
	}

	// 2. Delete agents untouched for >30 days (both exited and abandoned active agents)
	_, err = tx.Exec(`
		DELETE FROM agents
		WHERE updated_at < ?
	`, exitedRetentionCutoffMs)
	if err != nil {
		return fmt.Errorf("deleting old agents: %w", err)
	}

	// 3. Delete runs that ended before the cutoff or belong to non-existent agents
	_, err = tx.Exec(`
		DELETE FROM runs
		WHERE (ended_at IS NOT NULL AND ended_at < ?)
		   OR agent_id NOT IN (SELECT id FROM agents)
	`, exitedRetentionCutoffMs)
	if err != nil {
		return fmt.Errorf("deleting old and orphaned runs: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// 4. Execute incremental vacuum
	_, _ = s.db.Exec("PRAGMA incremental_vacuum(1000);")
	return nil
}

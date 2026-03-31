package db

import (
	"context"
	"database/sql"
	"fmt"
)

// SetupState represents the setup wizard state.
type SetupState struct {
	SetupTokenHash string
	CurrentStep    string
	CompletedSteps string // JSON array
	Completed      bool
	CompletedAt    *string
	CreatedAt      string
}

// CreateSetupState initializes the setup wizard state with the token hash.
func (d *DB) CreateSetupState(ctx context.Context, tokenHash string) error {
	_, err := d.conn.ExecContext(ctx,
		`INSERT INTO setup_state (id, setup_token_hash, current_step, completed_steps, completed, created_at)
		 VALUES (1, ?, 'domain_config', '[]', 0, ?)`,
		tokenHash, Now(),
	)
	if err != nil {
		return fmt.Errorf("creating setup state: %w", err)
	}
	return nil
}

// GetSetupState returns the current setup state, or nil if no setup has been initiated.
func (d *DB) GetSetupState(ctx context.Context) (*SetupState, error) {
	var s SetupState
	err := d.conn.QueryRowContext(ctx,
		`SELECT setup_token_hash, current_step, completed_steps, completed, completed_at, created_at
		 FROM setup_state WHERE id = 1`,
	).Scan(&s.SetupTokenHash, &s.CurrentStep, &s.CompletedSteps, &s.Completed, &s.CompletedAt, &s.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying setup state: %w", err)
	}
	return &s, nil
}

// UpdateSetupStep advances the setup wizard to the next step.
func (d *DB) UpdateSetupStep(ctx context.Context, step, completedSteps string) error {
	_, err := d.conn.ExecContext(ctx,
		`UPDATE setup_state SET current_step = ?, completed_steps = ? WHERE id = 1`,
		step, completedSteps,
	)
	if err != nil {
		return fmt.Errorf("updating setup step: %w", err)
	}
	return nil
}

// CompleteSetup marks setup as permanently finished.
func (d *DB) CompleteSetup(ctx context.Context) error {
	_, err := d.conn.ExecContext(ctx,
		`UPDATE setup_state SET completed = 1, completed_at = ?, current_step = 'complete' WHERE id = 1`,
		Now(),
	)
	if err != nil {
		return fmt.Errorf("completing setup: %w", err)
	}
	return nil
}

// DeleteSetupToken removes the setup token hash (production only — after passkey registration).
func (d *DB) DeleteSetupToken(ctx context.Context) error {
	_, err := d.conn.ExecContext(ctx,
		`UPDATE setup_state SET setup_token_hash = '' WHERE id = 1`,
	)
	if err != nil {
		return fmt.Errorf("deleting setup token: %w", err)
	}
	return nil
}

// IsSetupComplete returns true if setup has been completed.
func (d *DB) IsSetupComplete(ctx context.Context) (bool, error) {
	state, err := d.GetSetupState(ctx)
	if err != nil {
		return false, err
	}
	if state == nil {
		return false, nil // No setup state yet — not complete
	}
	return state.Completed, nil
}

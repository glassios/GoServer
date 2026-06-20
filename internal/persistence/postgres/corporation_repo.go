package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Home/galaxy-mmo/internal/domain"
)

type PostgresCorporationRepository struct {
	db *sql.DB
}

func NewPostgresCorporationRepository(db *sql.DB) *PostgresCorporationRepository {
	return &PostgresCorporationRepository{db: db}
}

func (r *PostgresCorporationRepository) Create(ctx context.Context, name string, founderID uint64) (*domain.Corporation, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 1. Insert Corporation
	var corpID uint32
	query := `INSERT INTO corporations (name, founder_id, wallet) VALUES ($1, $2, 0) RETURNING id`
	err = tx.QueryRowContext(ctx, query, name, founderID).Scan(&corpID)
	if err != nil {
		return nil, fmt.Errorf("failed to insert corporation: %w", err)
	}

	// 2. Add founder as Owner
	memberQuery := `INSERT INTO corporation_members (corp_id, account_id, role) VALUES ($1, $2, 'Owner')`
	_, err = tx.ExecContext(ctx, memberQuery, corpID, founderID)
	if err != nil {
		return nil, fmt.Errorf("failed to insert founder member: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &domain.Corporation{
		ID:        corpID,
		Name:      name,
		Wallet:    0,
		FounderID: founderID,
	}, nil
}

func (r *PostgresCorporationRepository) Get(ctx context.Context, corpID uint32) (*domain.Corporation, error) {
	var corp domain.Corporation
	query := `SELECT id, name, wallet, founder_id FROM corporations WHERE id = $1`
	err := r.db.QueryRowContext(ctx, query, corpID).Scan(&corp.ID, &corp.Name, &corp.Wallet, &corp.FounderID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // Not found
		}
		return nil, err
	}
	return &corp, nil
}

func (r *PostgresCorporationRepository) GetByName(ctx context.Context, name string) (*domain.Corporation, error) {
	var corp domain.Corporation
	query := `SELECT id, name, wallet, founder_id FROM corporations WHERE name = $1`
	err := r.db.QueryRowContext(ctx, query, name).Scan(&corp.ID, &corp.Name, &corp.Wallet, &corp.FounderID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // Not found
		}
		return nil, err
	}
	return &corp, nil
}

func (r *PostgresCorporationRepository) AddMember(ctx context.Context, corpID uint32, accountID uint64, role string) error {
	query := `
		INSERT INTO corporation_members (corp_id, account_id, role) 
		VALUES ($1, $2, $3) 
		ON CONFLICT (account_id) 
		DO UPDATE SET corp_id = EXCLUDED.corp_id, role = EXCLUDED.role`
	_, err := r.db.ExecContext(ctx, query, corpID, accountID, role)
	return err
}

func (r *PostgresCorporationRepository) RemoveMember(ctx context.Context, accountID uint64) error {
	query := `DELETE FROM corporation_members WHERE account_id = $1`
	_, err := r.db.ExecContext(ctx, query, accountID)
	return err
}

func (r *PostgresCorporationRepository) GetMemberRole(ctx context.Context, accountID uint64) (uint32, string, error) {
	var corpID uint32
	var role string
	query := `SELECT corp_id, role FROM corporation_members WHERE account_id = $1`
	err := r.db.QueryRowContext(ctx, query, accountID).Scan(&corpID, &role)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, "", nil // Not in a corp
		}
		return 0, "", err
	}
	return corpID, role, nil
}

func (r *PostgresCorporationRepository) GetMembers(ctx context.Context, corpID uint32) (map[uint64]string, error) {
	query := `SELECT account_id, role FROM corporation_members WHERE corp_id = $1`
	rows, err := r.db.QueryContext(ctx, query, corpID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := make(map[uint64]string)
	for rows.Next() {
		var accountID uint64
		var role string
		if err := rows.Scan(&accountID, &role); err != nil {
			return nil, err
		}
		members[accountID] = role
	}
	return members, nil
}

func (r *PostgresCorporationRepository) UpdateWallet(ctx context.Context, corpID uint32, amount int64) (int64, error) {
	var newBalance int64
	query := `
		UPDATE corporations 
		SET wallet = wallet + $1 
		WHERE id = $2 
		RETURNING wallet`
	err := r.db.QueryRowContext(ctx, query, amount, corpID).Scan(&newBalance)
	if err != nil {
		return 0, err
	}
	return newBalance, nil
}

package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"gemstore/internal/models"
)

// ErrEmailTaken is returned when an insert violates the unique index on
// users.email — detected via the Postgres error code rather than a
// SELECT-then-INSERT check, so a race between two concurrent signups for
// the same address can't both "win": only one INSERT succeeds, the
// other gets this error directly from the constraint.
var ErrEmailTaken = errors.New("repository: email already registered")

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// Create inserts a new user. Callers are responsible for hashing the
// password before calling this — the repository never sees a plaintext
// password, only whatever's already in user.PasswordHash.
func (r *UserRepository) Create(ctx context.Context, user models.User) (*models.User, error) {
	const query = `
		INSERT INTO users (email, password_hash, full_name, phone, role)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`

	err := r.pool.QueryRow(ctx, query,
		user.Email, user.PasswordHash, user.FullName, user.Phone, user.Role,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("repository: create user: %w", err)
	}

	return &user, nil
}

// GetByEmail is used by login. email is matched via the users.email
// CITEXT column, which is already case-insensitive at the database
// level — the case normalization callers apply before calling this is
// just for consistent storage/display, not required for correctness.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	const query = `
		SELECT id, email, password_hash, full_name, phone, role, created_at, updated_at
		FROM users WHERE email = $1`

	var u models.User
	err := r.pool.QueryRow(ctx, query, email).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &u.Phone, &u.Role,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repository: get user by email: %w", err)
	}

	return &u, nil
}

// GetByID backs GET /api/v1/auth/me.
func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	const query = `
		SELECT id, email, password_hash, full_name, phone, role, created_at, updated_at
		FROM users WHERE id = $1`

	var u models.User
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &u.Phone, &u.Role,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repository: get user by id: %w", err)
	}

	return &u, nil
}

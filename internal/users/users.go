package users

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrNotFound         = errors.New("user not found")
	ErrDuplicate        = errors.New("username already exists")
	ErrCannotDeleteSelf = errors.New("cannot delete your own account")
)

type User struct {
	ID        string
	Username  string
	IsAdmin   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Grant struct {
	ID           string
	UserID       string
	Action       string
	TopicPattern string
	CreatedAt    time.Time
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Create(ctx context.Context, username, plainPassword string, isAdmin bool) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	var u User
	err = s.pool.QueryRow(ctx,
		`INSERT INTO users (username, password_hash, is_admin)
         VALUES ($1, $2, $3)
         RETURNING id, username, is_admin, created_at, updated_at`,
		username, string(hash), isAdmin,
	).Scan(&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	return &u, nil
}

func (s *Store) GetByUsername(ctx context.Context, username string) (*User, string, error) {
	var u User
	var hash string
	err := s.pool.QueryRow(ctx,
		`SELECT id, username, password_hash, is_admin, created_at, updated_at
         FROM users WHERE username = $1`,
		username,
	).Scan(&u.ID, &u.Username, &hash, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	return &u, hash, err
}

func (s *Store) GetByID(ctx context.Context, id string) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT id, username, is_admin, created_at, updated_at
         FROM users WHERE id = $1`,
		id,
	).Scan(&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

func (s *Store) List(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, username, is_admin, created_at, updated_at FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, u)
	}
	return result, rows.Err()
}

func (s *Store) Update(ctx context.Context, id string, username *string, plainPassword *string, isAdmin *bool) (*User, error) {
	if username == nil && plainPassword == nil && isAdmin == nil {
		return s.GetByID(ctx, id)
	}
	// Build a partial update with only the provided fields.
	setClauses := []string{"updated_at = now()"}
	args := []any{}
	argN := 1

	if username != nil {
		setClauses = append(setClauses, fmt.Sprintf("username = $%d", argN))
		args = append(args, *username)
		argN++
	}
	if plainPassword != nil {
		hash, err := bcrypt.GenerateFromPassword([]byte(*plainPassword), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash password: %w", err)
		}
		setClauses = append(setClauses, fmt.Sprintf("password_hash = $%d", argN))
		args = append(args, string(hash))
		argN++
	}
	if isAdmin != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_admin = $%d", argN))
		args = append(args, *isAdmin)
		argN++
	}
	args = append(args, id)

	query := fmt.Sprintf(
		`UPDATE users SET %s WHERE id = $%d
         RETURNING id, username, is_admin, created_at, updated_at`,
		joinClauses(setClauses), argN,
	)
	var u User
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	return &u, nil
}

func (s *Store) Delete(ctx context.Context, id, callerID string) error {
	if id == callerID {
		return ErrCannotDeleteSelf
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) AddGrant(ctx context.Context, userID, action, topicPattern string) (*Grant, error) {
	var g Grant
	err := s.pool.QueryRow(ctx,
		`INSERT INTO user_grants (user_id, action, topic_pattern)
         VALUES ($1, $2, $3)
         RETURNING id, user_id, action, topic_pattern, created_at`,
		userID, action, topicPattern,
	).Scan(&g.ID, &g.UserID, &g.Action, &g.TopicPattern, &g.CreatedAt)
	return &g, err
}

func (s *Store) ListGrants(ctx context.Context, userID string) ([]Grant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, action, topic_pattern, created_at
         FROM user_grants WHERE user_id = $1 ORDER BY created_at`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Grant
	for rows.Next() {
		var g Grant
		if err := rows.Scan(&g.ID, &g.UserID, &g.Action, &g.TopicPattern, &g.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, g)
	}
	return result, rows.Err()
}

func (s *Store) GetUserGrants(ctx context.Context, userID string) ([]Grant, error) {
	return s.ListGrants(ctx, userID)
}

func (s *Store) DeleteGrant(ctx context.Context, grantID, userID string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM user_grants WHERE id = $1 AND user_id = $2`,
		grantID, userID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	// pgx wraps pgconn.PgError; check SQLSTATE 23505
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return false
}

func joinClauses(clauses []string) string {
	return strings.Join(clauses, ", ")
}

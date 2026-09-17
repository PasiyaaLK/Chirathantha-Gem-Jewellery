package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"gemstore/internal/middleware"
	"gemstore/internal/models"
	"gemstore/internal/repository"
)

// ErrEmailTaken re-exports repository.ErrEmailTaken for the same reason
// ErrOrderNotPending does in approval.go: handlers only import service.
var ErrEmailTaken = repository.ErrEmailTaken

// ErrInvalidCredentials covers both "no such email" and "wrong
// password" with one message — see the comment in Login for why those
// two cases are deliberately indistinguishable to the caller.
var ErrInvalidCredentials = errors.New("service: invalid email or password")

// bcryptCost is intentionally above bcrypt.DefaultCost (10). 12 costs
// roughly 4x the hashing time — still well under human-perceptible
// delay for a single login request, but meaningfully more expensive for
// an attacker running offline brute-force attempts against a leaked
// hash table.
const bcryptCost = 12

// dummyHash is compared against on a login attempt for an email that
// doesn't exist, so that path takes roughly the same time as "email
// exists, password wrong." Without this, an attacker could tell the two
// apart by response latency alone (a real bcrypt compare vs. an
// immediate DB miss) and enumerate registered emails without ever
// seeing a different error message.
var dummyHash = mustHash("not-a-real-password-used-only-for-timing")

func mustHash(pw string) []byte {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcryptCost)
	if err != nil {
		// Only ever runs once, at package init, on a fixed literal input
		// — if this fails, bcrypt itself is broken and nothing else in
		// the service would work either.
		panic(err)
	}
	return h
}

type AuthService struct {
	repo      *repository.UserRepository
	jwtSecret []byte
	tokenTTL  time.Duration
}

func NewAuthService(repo *repository.UserRepository, jwtSecret []byte, tokenTTL time.Duration) *AuthService {
	return &AuthService{repo: repo, jwtSecret: jwtSecret, tokenTTL: tokenTTL}
}

// AuthResult is what SignUp and Login both produce: the account plus a
// ready-to-use bearer token, so a handler never has to make a second
// call just to issue a token after creating/verifying the user.
type AuthResult struct {
	User  models.User
	Token string
}

// SignUp registers a new customer account. Role is hardcoded to
// 'customer' here — never read from the request — so public signup can
// never mint an admin no matter what a client sends. See cmd/seedadmin
// for creating the store owner's admin account out of band.
func (s *AuthService) SignUp(
	ctx context.Context,
	email, password, fullName string,
	phone *string,
) (*AuthResult, error) {
	email = normalizeEmail(email)

	if _, err := mail.ParseAddress(email); err != nil {
		return nil, ErrInvalidInput{Field: "email", Reason: "is not a valid email address"}
	}
	if len(password) < 8 {
		return nil, ErrInvalidInput{Field: "password", Reason: "must be at least 8 characters"}
	}
	fullName = strings.TrimSpace(fullName)
	if fullName == "" {
		return nil, ErrInvalidInput{Field: "full_name", Reason: "is required"}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("service: hash password: %w", err)
	}

	created, err := s.repo.Create(ctx, models.User{
		Email:        email,
		PasswordHash: string(hash),
		FullName:     fullName,
		Phone:        phone,
		Role:         models.RoleCustomer,
	})
	if err != nil {
		if errors.Is(err, repository.ErrEmailTaken) {
			return nil, ErrEmailTaken
		}
		return nil, err
	}

	return s.issueResult(*created)
}

// Login verifies email/password and issues a token on success.
//
// Deliberately returns the same ErrInvalidCredentials whether the email
// doesn't exist or the password is wrong, and burns equivalent time in
// the "doesn't exist" branch via a dummy bcrypt compare — see dummyHash.
// A login endpoint that returns "no such user" vs. "wrong password", or
// that responds faster for nonexistent emails, hands an attacker a free
// account-enumeration oracle.
func (s *AuthService) Login(ctx context.Context, email, password string) (*AuthResult, error) {
	email = normalizeEmail(email)

	user, err := s.repo.GetByEmail(ctx, email)
	if errors.Is(err, repository.ErrNotFound) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return s.issueResult(*user)
}

// GetProfile backs GET /api/v1/auth/me.
func (s *AuthService) GetProfile(ctx context.Context, userID uuid.UUID) (*models.User, error) {
	return s.repo.GetByID(ctx, userID)
}

func (s *AuthService) issueResult(user models.User) (*AuthResult, error) {
	token, err := middleware.GenerateToken(s.jwtSecret, user.ID, user.Role, s.tokenTTL)
	if err != nil {
		return nil, fmt.Errorf("service: issue token: %w", err)
	}
	return &AuthResult{User: user, Token: token}, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

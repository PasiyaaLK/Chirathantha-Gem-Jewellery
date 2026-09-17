// cmd/seedadmin is a one-off bootstrap CLI: it inserts an admin account
// directly into the database with a properly bcrypt-hashed password,
// bypassing POST /api/v1/auth/signup — which always assigns 'customer'
// and can never mint an admin (see AuthService.SignUp). Run this once
// per environment to create the store owner's login. After that, more
// admins can be added the same way, or via a direct SQL UPDATE on an
// existing customer row — a proper "invite admin" endpoint restricted to
// existing admins is a reasonable future addition once there's a UI for it.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"gemstore/internal/config"
	"gemstore/internal/database"
)

func main() {
	email := flag.String("email", "", "admin email (required)")
	password := flag.String("password", "", "admin password, min 8 chars (required)")
	fullName := flag.String("name", "Store Owner", "admin full name")
	flag.Parse()

	if *email == "" || len(*password) < 8 {
		fmt.Fprintln(os.Stderr, "error: -email and -password (min 8 chars) are required")
		flag.Usage()
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.Connect(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Same cost as AuthService.SignUp (kept as a literal here rather than
	// importing the service package just for one constant — this binary
	// has no other reason to depend on it).
	hash, err := bcrypt.GenerateFromPassword([]byte(*password), 12)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error hashing password:", err)
		os.Exit(1)
	}

	const query = `
		INSERT INTO users (email, password_hash, full_name, role)
		VALUES ($1, $2, $3, 'admin')
		RETURNING id`

	normalizedEmail := strings.ToLower(strings.TrimSpace(*email))

	var id string
	err = pool.QueryRow(ctx, query, normalizedEmail, string(hash), *fullName).Scan(&id)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating admin (does that email already exist?):", err)
		os.Exit(1)
	}

	fmt.Printf("admin created: id=%s email=%s\n", id, normalizedEmail)
}

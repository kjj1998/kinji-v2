package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kjj1998/kinji-v2/lambdas/internal/db"
	"github.com/kjj1998/kinji-v2/lambdas/internal/token"
)

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) < 2 {
		usage()
		return 2
	}

	switch os.Args[1] {
	case "create":
		fs := flag.NewFlagSet("create", flag.ExitOnError)
		email := fs.String("email", "", "user email (required)")
		name := fs.String("name", "", "user display name (required)")
		tokenName := fs.String("token-name", "", "label for the token, e.g. iphone (required)")

		fs.Parse(os.Args[2:])

		if *email == "" || *name == "" || *tokenName == "" {
			fs.Usage()
			return 2
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		conn, err := connect(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mint-token: %v\n", err)
			return 1
		}
		defer conn.Close(ctx)
		q := db.New(conn)

		if err := create(ctx, q, *email, *name, *tokenName); err != nil {
			fmt.Fprintf(os.Stderr, "mint-token: %v\n", err)
			return 1
		}
	case "revoke":
		fs := flag.NewFlagSet("revoke", flag.ExitOnError)
		id := fs.String("id", "", "token ID (required)")

		fs.Parse(os.Args[2:])

		if *id == "" {
			fs.Usage()
			return 2
		}

		decode, err := uuid.Parse(*id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid token id %q\n", *id)
			return 2
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		conn, err := connect(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mint-token: %v\n", err)
			return 1
		}
		defer conn.Close(ctx)
		q := db.New(conn)

		if err := revoke(ctx, q, decode); err != nil {
			fmt.Fprintf(os.Stderr, "mint-token: %v\n", err)
			return 1
		}
	default:
		usage()
		return 2
	}

	return 0
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: mint-token create --email E --name N --token-name T")
	fmt.Fprintln(os.Stderr, "       mint-token revoke --id UUID")
}

func connect(ctx context.Context) (*pgx.Conn, error) {
	dbURL := os.Getenv("MINT_TOKEN_DATABASE_URL")
	if dbURL == "" {
		return nil, errors.New("MINT_TOKEN_DATABASE_URL is not set")
	}

	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
	}
	return conn, nil
}

func create(ctx context.Context, q *db.Queries, email, name, tokenName string) error {
	email = strings.ToLower(strings.TrimSpace(email))

	userID, err := q.GetUserByEmail(ctx, email)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		userID, err = q.CreateUser(ctx, db.CreateUserParams{Name: name, Email: email})
		if err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		fmt.Fprintf(os.Stderr, "created user %s\n", userID)
	case err != nil:
		return fmt.Errorf("look up user: %w", err)
	default:
		fmt.Fprintf(os.Stderr, "found user %s\n", userID)
	}

	tok := token.Generate()

	row, err := q.CreateAPIToken(ctx, db.CreateAPITokenParams{
		UserID:    userID,
		TokenHash: token.Hash(tok),
		Scope:     "ingest",
		Name:      tokenName,
	})
	if err != nil {
		return fmt.Errorf("create token: %w", err)
	}

	fmt.Fprintf(os.Stderr, "created token %s (%s)\n", row.ID, tokenName)
	fmt.Println(tok)

	return nil
}

func revoke(ctx context.Context, q *db.Queries, id uuid.UUID) error {
	n, err := q.RevokeAPIToken(ctx, id)

	if err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("no active token with id %s", id)
	}

	fmt.Fprintf(os.Stderr, "revoked token %s\n", id)
	return nil
}

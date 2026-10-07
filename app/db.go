package main

import (
	"context"
	"net"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	// CNPG's Postgres listens on the standard port. It is not a secret and
	// is not passed in from the chart.
	dbPort = "5432"
	// Upper bound for one connect-and-query check, so /db cannot hang.
	dbCheckTimeout = 3 * time.Second
)

// newPostgresChecker returns a function that opens one connection, asks the
// server for its database name, and closes the connection. Connecting on
// each call (instead of at startup) means the app starts normally even if
// the database does not exist yet, and a database outage is only ever
// visible through /db.
func newPostgresChecker(cfg Config) func(ctx context.Context) (string, error) {
	// Build the connection URL with net/url so special characters in the
	// password or user name are escaped correctly.
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.DBUser, cfg.DBPassword),
		Host:   net.JoinHostPort(cfg.DBHost, dbPort),
		Path:   "/" + cfg.DBName,
	}
	dsn := u.String()

	return func(ctx context.Context) (string, error) {
		ctx, cancel := context.WithTimeout(ctx, dbCheckTimeout)
		defer cancel()

		conn, err := pgx.Connect(ctx, dsn)
		if err != nil {
			return "", err
		}
		defer conn.Close(context.Background())

		var name string
		if err := conn.QueryRow(ctx, "SELECT current_database()").Scan(&name); err != nil {
			return "", err
		}
		return name, nil
	}
}

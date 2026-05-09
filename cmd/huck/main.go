// huck is a small, self-hosted Go web server.
//
// See docs/DESIGN.md for the design and docs/sprint-1.md for the
// sprint-1 scope this main.go was written against.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffhelp"
	"golang.org/x/term"

	"github.com/mdhender/huck/internal/auth"
	"github.com/mdhender/huck/internal/config"
	"github.com/mdhender/huck/internal/db"
	"github.com/mdhender/huck/internal/dotenv"
	"github.com/mdhender/huck/internal/server"
	"github.com/mdhender/huck/internal/users"
)

func main() {
	// load any environment files
	env, ok := os.LookupEnv("HUCK_ENV")
	if !ok {
		env = "development"
	}
	err := dotenv.Load(env)
	if err != nil {
		fmt.Fprintf(os.Stderr, "huck: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		switch {
		case errors.Is(err, ff.ErrHelp), errors.Is(err, ff.ErrNoExec):
			// help or "no subcommand chosen" — already handled.
			return
		}
		fmt.Fprintf(os.Stderr, "huck: %v\n", err)
		os.Exit(1)
	}
}

// run wires the command tree and dispatches.
func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cfg := &config.Config{}

	root := newRootCmd(cfg, stdin, stdout, stderr)

	if err := root.Parse(args,
		ff.WithEnvVarPrefix("HUCK"),
		ff.WithConfigFileFlag("config"),
		ff.WithConfigFileParser(ff.PlainParser),
		ff.WithConfigAllowMissingFile(),
	); err != nil {
		fmt.Fprintf(stderr, "%s\n", ffhelp.Command(root))
		return err
	}

	// Set up slog now, before any subcommand runs, so its output uses the
	// requested level.
	slog.SetDefault(newLogger(cfg.LogLevel, stderr))

	if err := root.Run(ctx); err != nil {
		return err
	}
	return nil
}

// newRootCmd constructs the entire command tree.
func newRootCmd(cfg *config.Config, stdin io.Reader, stdout, stderr io.Writer) *ff.Command {
	rootFlags := ff.NewFlagSet("huck")
	// Globals visible to every subcommand.
	rootFlags.StringVar(&cfg.LogLevel, 0, "log-level", "info", "log level: debug|info|warn|error")
	rootFlags.StringVar(&cfg.ConfigFile, 0, "config", "", "optional path to a plain-format config file")

	root := &ff.Command{
		Name:      "huck",
		Usage:     "huck <subcommand> [flags]",
		ShortHelp: "huck — small self-hosted web server",
		Flags:     rootFlags,
	}
	root.Subcommands = []*ff.Command{
		newServeCmd(cfg, stderr, rootFlags),
		newDBCmd(cfg, stdout, rootFlags),
		newAdminCmd(cfg, stdin, stdout, stderr, rootFlags),
	}
	return root
}

// ---------- serve ----------

func newServeCmd(cfg *config.Config, stderr io.Writer, parent *ff.FlagSet) *ff.Command {
	fs := ff.NewFlagSet("serve").SetParent(parent)
	fs.StringVar(&cfg.Addr, 0, "addr", ":8080", "listen address")
	fs.StringVar(&cfg.DB, 0, "db", "", "path to the SQLite file (must already exist)")
	fs.StringVar(&cfg.BaseURL, 0, "base-url", "", "public base URL (parsed; unused in Sprint 1)")
	fs.StringVar(&cfg.JWTSecret, 0, "jwt-secret", "", "HS256 signing key, ≥32 bytes")
	fs.BoolVarDefault(&cfg.CookieSecure, 0, "cookie-secure", true, "set Secure on the auth cookie")
	fs.StringVar(&cfg.CookieDomain, 0, "cookie-domain", "", "optional cookie Domain attribute")
	fs.StringVar(&cfg.MailgunDomain, 0, "mailgun-domain", "", "Mailgun sending domain (parsed; unused in Sprint 1)")
	fs.StringVar(&cfg.MailgunAPIKey, 0, "mailgun-api-key", "", "Mailgun API key (parsed; unused in Sprint 1)")
	fs.StringVar(&cfg.MailgunFrom, 0, "mailgun-from", "", "From: address for invite mail (parsed; unused in Sprint 1)")

	cmd := &ff.Command{
		Name:      "serve",
		Usage:     "huck serve [flags]",
		ShortHelp: "start the web server",
		Flags:     fs,
	}
	cmd.Exec = func(ctx context.Context, _ []string) error {
		if err := cfg.ValidateServe(); err != nil {
			return err
		}
		return runServe(ctx, cfg, stderr)
	}
	return cmd
}

func runServe(ctx context.Context, cfg *config.Config, _ io.Writer) error {
	logger := slog.Default()

	pool, err := db.Open(cfg.DB)
	if err != nil {
		if errors.Is(err, db.ErrMissing) {
			return fmt.Errorf("%w\n  → run `huck db create --db %s` first", err, cfg.DB)
		}
		return err
	}
	defer pool.Close()

	if err := db.Migrate(pool); err != nil {
		return fmt.Errorf("migrate on startup: %w", err)
	}

	store := users.NewStore(pool)
	srv, err := server.New(cfg, store, logger)
	if err != nil {
		return err
	}

	sc := echo.StartConfig{
		Address:         cfg.Addr,
		HideBanner:      true,
		GracefulTimeout: 10 * time.Second,
		BeforeServeFunc: func(s *http.Server) error {
			s.ReadHeaderTimeout = 5 * time.Second
			s.ReadTimeout = 15 * time.Second
			s.WriteTimeout = 30 * time.Second
			s.IdleTimeout = 120 * time.Second
			return nil
		},
	}

	logger.Info("huck.serve.starting", "addr", cfg.Addr, "db", cfg.DB)
	if err := sc.Start(ctx, srv.Echo()); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	logger.Info("huck.serve.stopped")
	return nil
}

// ---------- db ----------

func newDBCmd(cfg *config.Config, stdout io.Writer, parent *ff.FlagSet) *ff.Command {
	dbFlags := ff.NewFlagSet("db").SetParent(parent)
	cmd := &ff.Command{
		Name:      "db",
		Usage:     "huck db <create|migrate> [flags]",
		ShortHelp: "database lifecycle commands",
		Flags:     dbFlags,
	}
	cmd.Subcommands = []*ff.Command{
		newDBCreateCmd(cfg, stdout, dbFlags),
		newDBMigrateCmd(cfg, stdout, dbFlags),
	}
	return cmd
}

func newDBCreateCmd(cfg *config.Config, stdout io.Writer, parent *ff.FlagSet) *ff.Command {
	fs := ff.NewFlagSet("create").SetParent(parent)
	fs.StringVar(&cfg.DB, 0, "db", "", "path to the new SQLite file (must NOT exist)")
	cmd := &ff.Command{
		Name:      "create",
		Usage:     "huck db create --db <path>",
		ShortHelp: "create a new SQLite file and apply migrations",
		Flags:     fs,
	}
	cmd.Exec = func(ctx context.Context, _ []string) error {
		if err := cfg.ValidateDB(); err != nil {
			return err
		}
		if err := db.Create(cfg.DB); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "created %s\n", cfg.DB)
		return nil
	}
	return cmd
}

func newDBMigrateCmd(cfg *config.Config, stdout io.Writer, parent *ff.FlagSet) *ff.Command {
	fs := ff.NewFlagSet("migrate").SetParent(parent)
	fs.StringVar(&cfg.DB, 0, "db", "", "path to an existing SQLite file")
	cmd := &ff.Command{
		Name:      "migrate",
		Usage:     "huck db migrate --db <path>",
		ShortHelp: "apply any pending migrations",
		Flags:     fs,
	}
	cmd.Exec = func(ctx context.Context, _ []string) error {
		if err := cfg.ValidateDB(); err != nil {
			return err
		}
		pool, err := db.Open(cfg.DB)
		if err != nil {
			if errors.Is(err, db.ErrMissing) {
				return fmt.Errorf("%w\n  → run `huck db create --db %s` first", err, cfg.DB)
			}
			return err
		}
		defer pool.Close()
		if err := db.Migrate(pool); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "migrations up to date")
		return nil
	}
	return cmd
}

// ---------- admin ----------

func newAdminCmd(cfg *config.Config, stdin io.Reader, stdout, stderr io.Writer, parent *ff.FlagSet) *ff.Command {
	adminFlags := ff.NewFlagSet("admin").SetParent(parent)
	cmd := &ff.Command{
		Name:      "admin",
		Usage:     "huck admin <create> [flags]",
		ShortHelp: "admin user management",
		Flags:     adminFlags,
	}
	cmd.Subcommands = []*ff.Command{newAdminCreateCmd(cfg, stdin, stdout, stderr, adminFlags)}
	return cmd
}

func newAdminCreateCmd(cfg *config.Config, stdin io.Reader, stdout, stderr io.Writer, parent *ff.FlagSet) *ff.Command {
	fs := ff.NewFlagSet("create").SetParent(parent)
	fs.StringVar(&cfg.DB, 0, "db", "", "path to the SQLite file")
	fs.StringVar(&cfg.AdminHandle, 0, "handle", "", "admin handle")
	fs.StringVar(&cfg.AdminEmail, 0, "email", "", "admin email")
	cmd := &ff.Command{
		Name:      "create",
		Usage:     "huck admin create --db <path> --handle <h> --email <e>",
		ShortHelp: "create the bootstrap admin user",
		Flags:     fs,
	}
	cmd.Exec = func(ctx context.Context, _ []string) error {
		if err := cfg.ValidateAdminCreate(); err != nil {
			return err
		}
		return runAdminCreate(ctx, cfg, stdin, stdout, stderr)
	}
	return cmd
}

func runAdminCreate(ctx context.Context, cfg *config.Config, stdin io.Reader, stdout, stderr io.Writer) error {
	pool, err := db.Open(cfg.DB)
	if err != nil {
		if errors.Is(err, db.ErrMissing) {
			return fmt.Errorf("%w\n  → run `huck db create --db %s` first", err, cfg.DB)
		}
		return err
	}
	defer pool.Close()

	count, err := db.AppliedCount(pool)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("database has no applied migrations\n  → run `huck db migrate --db %s` first", cfg.DB)
	}

	store := users.NewStore(pool)
	exists, err := store.AdminExists(ctx)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("an admin user already exists; refusing to create another")
	}

	if err := auth.ValidateHandle(cfg.AdminHandle); err != nil {
		return err
	}

	password, err := readAdminPassword(stdin, stderr)
	if err != nil {
		return err
	}
	hash, err := auth.Hash(password)
	if err != nil {
		return err
	}

	u, err := store.Create(ctx, users.NewUser{
		Handle:       cfg.AdminHandle,
		Email:        cfg.AdminEmail,
		PasswordHash: hash,
		IsAdmin:      true,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "created admin: handle=%s email=%s id=%d\n", u.Handle, u.Email, u.ID)
	return nil
}

// readAdminPassword honours $HUCK_ADMIN_PASSWORD; otherwise prompts on the
// TTY without echo and asks for confirmation. If stdin isn't a TTY (e.g.
// in CI) but the env var is unset, we fall back to a single line read.
//
// Whatever path produces the password, it must satisfy
// [auth.ValidatePassword] (DESIGN.md §8.7) before being returned.
func readAdminPassword(stdin io.Reader, stderr io.Writer) (string, error) {
	if pw := os.Getenv("HUCK_ADMIN_PASSWORD"); pw != "" {
		if err := auth.ValidatePassword(pw); err != nil {
			return "", err
		}
		return pw, nil
	}

	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprint(stderr, "password: ")
		pw1, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(stderr)
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		fmt.Fprint(stderr, "confirm:  ")
		pw2, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(stderr)
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		if string(pw1) != string(pw2) {
			return "", errors.New("passwords did not match")
		}
		if err := auth.ValidatePassword(string(pw1)); err != nil {
			return "", err
		}
		return string(pw1), nil
	}

	// Non-TTY fallback (e.g. piped stdin in CI).
	fmt.Fprint(stderr, "password: ")
	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", errors.New("no password supplied on stdin")
	}
	pw := strings.TrimRight(scanner.Text(), "\r\n")
	if err := auth.ValidatePassword(pw); err != nil {
		return "", err
	}
	return pw, nil
}

// newLogger builds a slog.Logger for the requested text level.
func newLogger(level string, w io.Writer) *slog.Logger {
	var lv slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lv = slog.LevelDebug
	case "warn", "warning":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: lv}))
}

package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"

	"school-journal/internal/behavior"
	"school-journal/internal/platform/postgres"
)

func main() {
	email := flag.String("email", "", "administrator email")
	name := flag.String("name", "School Administrator", "administrator full name")
	school := flag.String("school", "Demo School", "school name")
	flag.Parse()
	if strings.TrimSpace(*email) == "" || strings.TrimSpace(*name) == "" || strings.TrimSpace(*school) == "" {
		fatal("--email, --name, and --school must not be empty")
	}
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		fatal("DATABASE_URL is required")
	}
	password := readPassword("Пароль администратора (не менее 12 символов): ")
	confirmation := readPassword("Повторите пароль: ")
	if len(password) < 12 || string(password) != string(confirmation) {
		fatal("Пароли не совпадают или короче 12 символов")
	}
	passwordHash, err := bcrypt.GenerateFromPassword(password, 12)
	if err != nil {
		fatal("could not hash password: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := postgres.Open(ctx, dsn, 2, 1)
	if err != nil {
		fatal("database unavailable: %v", err)
	}
	defer db.Close()
	if err := seed(ctx, db, strings.TrimSpace(*email), strings.TrimSpace(*name), strings.TrimSpace(*school), string(passwordHash)); err != nil {
		fatal("administrator setup failed: %v", err)
	}
	fmt.Println("Administrator and behavior criteria are ready.")
}

func seed(ctx context.Context, db *sql.DB, email, fullName, schoolName, passwordHash string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var schoolID string
	err = tx.QueryRowContext(ctx, `SELECT id::text FROM schools WHERE name=$1 ORDER BY created_at LIMIT 1`, schoolName).Scan(&schoolID)
	if err == sql.ErrNoRows {
		err = tx.QueryRowContext(ctx, `INSERT INTO schools(name) VALUES($1) RETURNING id::text`, schoolName).Scan(&schoolID)
	}
	if err != nil {
		return err
	}
	var duplicateEmail bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE lower(email::text)=lower($1) AND school_id<>$2)`, email, schoolID).Scan(&duplicateEmail); err != nil {
		return err
	}
	if duplicateEmail {
		return fmt.Errorf("email is already used by another school; the sign-in email must identify one account")
	}
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT id::text FROM users WHERE school_id=$1 AND email=$2`, schoolID, email).Scan(&existing)
	if err == nil {
		return fmt.Errorf("an account already exists; seedadmin will not reset its password")
	}
	if err != sql.ErrNoRows {
		return err
	}
	var userID string
	err = tx.QueryRowContext(ctx, `INSERT INTO users(school_id,email,password_hash,full_name,status)
		VALUES($1,$2,$3,$4,'active') RETURNING id::text`, schoolID, email, passwordHash, fullName).Scan(&userID)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_roles(user_id,role_id,school_id)
		SELECT $1,id,$2 FROM roles WHERE code='admin' ON CONFLICT DO NOTHING`, userID, schoolID); err != nil {
		return err
	}
	for _, criterion := range behavior.DefaultCriteria() {
		if _, err := tx.ExecContext(ctx, `INSERT INTO behavior_criteria(school_id,code,title,max_score,sort_order)
			VALUES($1,$2,$3,3,$4) ON CONFLICT(school_id,code) DO NOTHING`, schoolID, criterion.Code, criterion.Title, criterion.SortOrder); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_log(actor_user_id,action,entity,entity_id,new_value)
		VALUES($1,'seed','users',$1,'{"role":"admin"}'::jsonb)`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func readPassword(prompt string) []byte {
	fmt.Print(prompt)
	password, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		fatal("could not read password: %v", err)
	}
	return password
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

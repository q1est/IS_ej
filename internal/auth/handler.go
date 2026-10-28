package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"school-journal/internal/audit"
	"school-journal/internal/config"
	"school-journal/internal/identity"
	"school-journal/internal/platform/httpx"
	"school-journal/internal/platform/ratelimit"
)

const issuer = "school-journal"
const passwordCost = 12

type Handler struct {
	db         *sql.DB
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

type claims struct {
	UserID   string `json:"uid"`
	SchoolID string `json:"sid"`
	jwt.RegisteredClaims
}

func New(db *sql.DB, cfg config.Config) *Handler {
	return &Handler{db: db, secret: cfg.JWTSecret, accessTTL: cfg.AccessTokenTTL, refreshTTL: cfg.RefreshTokenTTL}
}

func (h *Handler) Register(mux *http.ServeMux, limiter *ratelimit.Limiter) {
	mux.Handle("POST /auth/login", limiter.Wrap(http.HandlerFunc(h.login)))
	mux.Handle("POST /auth/refresh", limiter.Wrap(http.HandlerFunc(h.refresh)))
	mux.Handle("POST /auth/logout", h.Protect(http.HandlerFunc(h.logout)))
	mux.Handle("POST /invites/accept", limiter.Wrap(http.HandlerFunc(h.acceptInvite)))
	mux.Handle("GET /me", h.Protect(http.HandlerFunc(h.me)))
}

func (h *Handler) Protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization := strings.TrimSpace(r.Header.Get("Authorization"))
		prefix := "Bearer "
		if !strings.HasPrefix(authorization, prefix) || strings.TrimSpace(strings.TrimPrefix(authorization, prefix)) == "" {
			httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
			return
		}
		tokenString := strings.TrimSpace(strings.TrimPrefix(authorization, prefix))
		parsed, err := jwt.ParseWithClaims(tokenString, &claims{}, func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("unexpected signing method")
			}
			return h.secret, nil
		}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(issuer), jwt.WithLeeway(30*time.Second))
		if err != nil || !parsed.Valid {
			httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Invalid or expired access token")
			return
		}
		c, ok := parsed.Claims.(*claims)
		if !ok || c.UserID == "" || c.SchoolID == "" || c.Subject != c.UserID {
			httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Invalid access token")
			return
		}
		principal, err := h.loadPrincipal(r.Context(), h.db, c.UserID)
		if err != nil || principal.SchoolID != c.SchoolID {
			httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		next.ServeHTTP(w, r.WithContext(identity.WithPrincipal(r.Context(), principal)))
	})
}

func (h *Handler) loadPrincipal(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, userID string) (identity.Principal, error) {
	p := identity.Principal{UserID: userID, Roles: []string{}}
	var status string
	err := q.QueryRowContext(ctx, `SELECT school_id::text,email::text,full_name,status FROM users WHERE id=$1`, userID).
		Scan(&p.SchoolID, &p.Email, &p.FullName, &status)
	if err != nil {
		return p, err
	}
	if status != "active" {
		return p, errors.New("account is inactive")
	}
	rows, err := q.QueryContext(ctx, `SELECT r.code FROM user_roles ur JOIN roles r ON r.id=ur.role_id
		WHERE ur.user_id=$1 AND ur.school_id=$2 ORDER BY r.code`, userID, p.SchoolID)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return p, err
		}
		p.Roles = append(p.Roles, role)
	}
	if err := rows.Err(); err != nil {
		return p, err
	}
	if identity.HasRole(p, "student") {
		_ = q.QueryRowContext(ctx, `SELECT id::text FROM students WHERE user_id=$1 AND school_id=$2`, userID, p.SchoolID).Scan(&p.StudentID)
	}
	return p, nil
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		SchoolID string `json:"school_id"`
	}
	if httpx.Decode(w, r, &input) != nil || strings.TrimSpace(input.Email) == "" || input.Password == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "Email and password are required")
		return
	}
	input.Email = strings.TrimSpace(strings.ToLower(input.Email))
	rows, err := h.db.QueryContext(r.Context(), `SELECT id::text,school_id::text,password_hash,status FROM users
		WHERE email=$1 AND ($2='' OR school_id::text=$2) ORDER BY created_at LIMIT 2`, input.Email, strings.TrimSpace(input.SchoolID))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not sign in")
		return
	}
	type account struct{ userID, schoolID, passwordHash, status string }
	accounts := make([]account, 0, 2)
	for rows.Next() {
		var item account
		if rows.Scan(&item.userID, &item.schoolID, &item.passwordHash, &item.status) == nil {
			accounts = append(accounts, item)
		}
	}
	rowsErr := rows.Err()
	_ = rows.Close()
	if rowsErr != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not sign in")
		return
	}
	if len(accounts) != 1 || accounts[0].status != "active" || accounts[0].passwordHash == "" ||
		bcrypt.CompareHashAndPassword([]byte(accounts[0].passwordHash), []byte(input.Password)) != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
		return
	}
	currentAccount := accounts[0]
	principal, err := h.loadPrincipal(r.Context(), h.db, currentAccount.userID)
	if err != nil || principal.SchoolID != currentAccount.schoolID {
		httpx.Error(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
		return
	}
	if cost, err := bcrypt.Cost([]byte(currentAccount.passwordHash)); err == nil && cost < passwordCost {
		upgraded, err := bcrypt.GenerateFromPassword([]byte(input.Password), passwordCost)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not secure account")
			return
		}
		if _, err := h.db.ExecContext(r.Context(), `UPDATE users SET password_hash=$2 WHERE id=$1 AND password_hash=$3`, currentAccount.userID, string(upgraded), currentAccount.passwordHash); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not secure account")
			return
		}
	}
	access, err := h.issueAccess(principal)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not issue access token")
		return
	}
	refresh, err := randomToken(32)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create session")
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create session")
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(r.Context(), `INSERT INTO refresh_sessions(user_id,token_hash,expires_at,ip,user_agent)
		VALUES($1,$2,$3,$4,$5)`, principal.UserID, tokenHash(refresh), time.Now().Add(h.refreshTTL), httpx.ClientIP(r), r.UserAgent())
	if err == nil {
		err = audit.Write(r.Context(), tx, r, principal, "login", "users", principal.UserID, nil, map[string]any{"event": "login"})
	}
	if err != nil || tx.Commit() != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create session")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"access_token": access, "refresh_token": refresh, "token_type": "Bearer",
		"expires_in": int(h.accessTTL.Seconds()), "user": principal,
	})
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RefreshToken string `json:"refresh_token"`
	}
	if httpx.Decode(w, r, &input) != nil || input.RefreshToken == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "refresh_token is required")
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not rotate session")
		return
	}
	defer tx.Rollback()
	var sessionID, userID string
	var expiresAt time.Time
	var revokedAt sql.NullTime
	err = tx.QueryRowContext(r.Context(), `SELECT id::text,user_id::text,expires_at,revoked_at FROM refresh_sessions
		WHERE token_hash=$1 FOR UPDATE`, tokenHash(input.RefreshToken)).Scan(&sessionID, &userID, &expiresAt, &revokedAt)
	if err != nil || revokedAt.Valid || !time.Now().Before(expiresAt) {
		httpx.Error(w, http.StatusUnauthorized, "invalid_refresh", "Refresh token is invalid or expired")
		return
	}
	principal, err := h.loadPrincipal(r.Context(), tx, userID)
	if err != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid_refresh", "Refresh token is invalid or expired")
		return
	}
	access, err := h.issueAccess(principal)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not rotate session")
		return
	}
	refresh, err := randomToken(32)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not rotate session")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE refresh_sessions SET revoked_at=now() WHERE id=$1`, sessionID); err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO refresh_sessions(user_id,token_hash,expires_at,ip,user_agent)
			VALUES($1,$2,$3,$4,$5)`, userID, tokenHash(refresh), time.Now().Add(h.refreshTTL), httpx.ClientIP(r), r.UserAgent())
	}
	if err != nil || tx.Commit() != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not rotate session")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"access_token": access, "refresh_token": refresh, "expires_in": int(h.accessTTL.Seconds())})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	principal, _ := identity.FromContext(r.Context())
	var input struct {
		RefreshToken string `json:"refresh_token"`
	}
	if httpx.Decode(w, r, &input) != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not sign out")
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(r.Context(), `UPDATE refresh_sessions SET revoked_at=now() WHERE user_id=$1 AND token_hash=$2 AND revoked_at IS NULL`, principal.UserID, tokenHash(input.RefreshToken))
	if err == nil {
		err = audit.Write(r.Context(), tx, r, principal, "logout", "users", principal.UserID, nil, nil)
	}
	if err != nil || tx.Commit() != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not sign out")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) acceptInvite(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Token    string `json:"token"`
		Password string `json:"password"`
		FullName string `json:"full_name"`
	}
	if httpx.Decode(w, r, &input) != nil || len(input.Password) < 12 || strings.TrimSpace(input.FullName) == "" || input.Token == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "Token, full name, and a password of at least 12 characters are required")
		return
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), passwordCost)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not secure account")
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not accept invitation")
		return
	}
	defer tx.Rollback()
	var inviteID, schoolID, email, role string
	var targetType string
	var targetID, yearID sql.NullString
	var expiresAt time.Time
	var usedAt sql.NullTime
	err = tx.QueryRowContext(r.Context(), `SELECT i.id::text,i.school_id::text,i.email::text,r.code,i.target_type,i.target_id::text,
		i.academic_year_id::text,i.expires_at,i.used_at FROM invites i JOIN roles r ON r.id=i.role_id
		WHERE i.token_hash=$1 FOR UPDATE OF i`, tokenHash(input.Token)).
		Scan(&inviteID, &schoolID, &email, &role, &targetType, &targetID, &yearID, &expiresAt, &usedAt)
	if err != nil || usedAt.Valid || !time.Now().Before(expiresAt) {
		httpx.Error(w, http.StatusBadRequest, "invalid_invite", "Invitation is invalid, expired, or already used")
		return
	}
	if role == "student" && (targetType != "student" || !targetID.Valid) {
		httpx.Error(w, http.StatusBadRequest, "invalid_invite", "Student invitations must identify a student record")
		return
	}
	if role != "student" && !validInviteTarget(role, targetType) {
		httpx.Error(w, http.StatusBadRequest, "invalid_invite", "Invitation target does not match the role")
		return
	}
	studentName := strings.TrimSpace(input.FullName)
	if role == "student" {
		var linkedUser sql.NullString
		err = tx.QueryRowContext(r.Context(), `SELECT full_name,user_id::text FROM students WHERE id=$1 AND school_id=$2 FOR UPDATE`, targetID.String, schoolID).Scan(&studentName, &linkedUser)
		if err != nil || linkedUser.Valid {
			httpx.Error(w, http.StatusConflict, "student_link_conflict", "Student record is unavailable or already linked")
			return
		}
	}
	var existingID, existingStatus string
	err = tx.QueryRowContext(r.Context(), `SELECT id::text,status FROM users WHERE school_id=$1 AND email=$2 FOR UPDATE`, schoolID, email).Scan(&existingID, &existingStatus)
	userID := existingID
	if err == nil {
		if existingStatus != "invited" {
			httpx.Error(w, http.StatusConflict, "account_exists", "An active account already exists for this invitation")
			return
		}
		_, err = tx.ExecContext(r.Context(), `UPDATE users SET password_hash=$2,full_name=$3,status='active' WHERE id=$1`, userID, string(passwordHash), studentName)
	} else if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(r.Context(), `INSERT INTO users(school_id,email,password_hash,full_name,status)
			VALUES($1,$2,$3,$4,'active') RETURNING id::text`, schoolID, email, string(passwordHash), studentName).Scan(&userID)
	}
	if err != nil {
		httpx.Error(w, http.StatusConflict, "account_create_failed", "Could not activate account")
		return
	}
	if role == "student" {
		_, err = tx.ExecContext(r.Context(), `UPDATE students SET user_id=$1 WHERE id=$2 AND school_id=$3 AND user_id IS NULL`, userID, targetID.String, schoolID)
		if err != nil {
			httpx.Error(w, http.StatusConflict, "student_link_conflict", "Could not link student record")
			return
		}
	}
	_, err = tx.ExecContext(r.Context(), `INSERT INTO user_roles(user_id,role_id,school_id,academic_year_id,scope_id)
		SELECT $1,id,$2,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid FROM roles WHERE code=$5 ON CONFLICT DO NOTHING`,
		userID, schoolID, yearID.String, targetID.String, role)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "role_assign_failed", "Could not assign account role")
		return
	}
	principal, err := h.loadPrincipal(r.Context(), tx, userID)
	if err == nil {
		err = audit.Write(r.Context(), tx, r, principal, "accept_invite", "users", userID, nil, map[string]any{"role": role})
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE invites SET used_at=now() WHERE id=$1`, inviteID)
	}
	if err != nil || tx.Commit() != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not complete invitation")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	principal, _ := identity.FromContext(r.Context())
	httpx.JSON(w, http.StatusOK, principal)
}

func (h *Handler) issueAccess(p identity.Principal) (string, error) {
	now := time.Now()
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		UserID: p.UserID, SchoolID: p.SchoolID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: issuer, Subject: p.UserID,
			IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(h.accessTTL)),
		},
	}).SignedString(h.secret)
}

func randomToken(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func tokenHash(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func validInviteTarget(role, targetType string) bool {
	if targetType == "" {
		return role == "admin" || role == "teacher" || role == "class_teacher"
	}
	switch role {
	case "teacher":
		return targetType == "teacher" || targetType == "class"
	case "class_teacher":
		return targetType == "class_teacher" || targetType == "class"
	case "admin":
		return false
	default:
		return false
	}
}

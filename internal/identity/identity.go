package identity

import "context"

type Principal struct {
	UserID    string   `json:"id"`
	SchoolID  string   `json:"school_id"`
	StudentID string   `json:"student_id,omitempty"`
	Email     string   `json:"email"`
	FullName  string   `json:"full_name"`
	Roles     []string `json:"roles"`
}

type contextKey struct{}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, principal)
}

func FromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(contextKey{}).(Principal)
	return principal, ok
}

func IsAdmin(principal Principal) bool {
	for _, role := range principal.Roles {
		if role == "admin" {
			return true
		}
	}
	return false
}

func HasRole(principal Principal, role string) bool {
	if IsAdmin(principal) {
		return true
	}
	for _, candidate := range principal.Roles {
		if candidate == role {
			return true
		}
	}
	return false
}

package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"

	"school-journal/internal/identity"
	"school-journal/internal/platform/httpx"
)

type Executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func Write(ctx context.Context, executor Executor, request *http.Request, actor identity.Principal, action, entity, entityID string, oldValue, newValue any) error {
	oldJSON, err := json.Marshal(oldValue)
	if err != nil {
		return err
	}
	newJSON, err := json.Marshal(newValue)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `INSERT INTO audit_log(actor_user_id,action,entity,entity_id,old_value,new_value,ip,user_agent)
		VALUES($1,$2,$3,$4,NULLIF($5::jsonb,'null'::jsonb),NULLIF($6::jsonb,'null'::jsonb),$7,$8)`,
		actor.UserID, action, entity, entityID, string(oldJSON), string(newJSON), httpx.ClientIP(request), request.UserAgent())
	return err
}

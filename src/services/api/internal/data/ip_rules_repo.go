package data

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
"arkloop/services/shared/database"
)

type IPRuleType string

const (
	IPRuleAllowlist IPRuleType = "allowlist"
	IPRuleBlocklist IPRuleType = "blocklist"
)

type IPRule struct {
	ID        uuid.UUID
	OrgID     uuid.UUID
	Type      IPRuleType
	CIDR      string
	Note      *string
	CreatedAt time.Time
}

type IPRulesRepository struct {
	db Querier
}

func NewIPRulesRepository(db Querier) (*IPRulesRepository, error) {
	if db == nil {
		return nil, errors.New("db must not be nil")
	}
	return &IPRulesRepository{db: db}, nil
}

func (r *IPRulesRepository) Create(
	ctx context.Context,
	orgID uuid.UUID,
	ruleType IPRuleType,
	cidr string,
	note *string,
) (IPRule, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if orgID == uuid.Nil {
		return IPRule{}, fmt.Errorf("org_id must not be nil")
	}
	if ruleType != IPRuleAllowlist && ruleType != IPRuleBlocklist {
		return IPRule{}, fmt.Errorf("type must be allowlist or blocklist")
	}

	cidr = strings.TrimSpace(cidr)
	if _, _, err := net.ParseCIDR(cidr); err != nil {
		return IPRule{}, fmt.Errorf("invalid cidr: %w", err)
	}

	var rule IPRule
	err := r.db.QueryRow(
		ctx,
		`INSERT INTO ip_rules (org_id, type, cidr, note)
		 VALUES ($1, $2, $3::cidr, $4)
		 RETURNING id, org_id, type, host(cidr) || '/' || masklen(cidr), note, created_at`,
		orgID, string(ruleType), cidr, note,
	).Scan(&rule.ID, &rule.OrgID, &rule.Type, &rule.CIDR, &rule.Note, &rule.CreatedAt)
	if err != nil {
		return IPRule{}, err
	}
	return rule, nil
}

func (r *IPRulesRepository) ListByOrg(ctx context.Context, orgID uuid.UUID) ([]IPRule, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	rows, err := r.db.Query(
		ctx,
		`SELECT id, org_id, type, host(cidr) || '/' || masklen(cidr), note, created_at
		 FROM ip_rules
		 WHERE org_id = $1
		 ORDER BY created_at ASC`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := []IPRule{}
	for rows.Next() {
		var rule IPRule
		if err := rows.Scan(&rule.ID, &rule.OrgID, &rule.Type, &rule.CIDR, &rule.Note, &rule.CreatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (r *IPRulesRepository) GetByID(ctx context.Context, orgID, id uuid.UUID) (*IPRule, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var rule IPRule
	err := r.db.QueryRow(
		ctx,
		`SELECT id, org_id, type, host(cidr) || '/' || masklen(cidr), note, created_at
		 FROM ip_rules
		 WHERE id = $1 AND org_id = $2`,
		id, orgID,
	).Scan(&rule.ID, &rule.OrgID, &rule.Type, &rule.CIDR, &rule.Note, &rule.CreatedAt)
	if err != nil {
		if errors.Is(err, database.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &rule, nil
}

func (r *IPRulesRepository) Delete(ctx context.Context, orgID, id uuid.UUID) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	tag, err := r.db.Exec(
		ctx,
		`DELETE FROM ip_rules WHERE id = $1 AND org_id = $2`,
		id, orgID,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

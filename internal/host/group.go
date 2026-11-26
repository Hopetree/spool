package host

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Hopetree/spool/internal/store"
)

type groupRow struct {
	ID          string `db:"id"`
	Name        string `db:"name"`
	Description string `db:"description"`
	HostCount   int    `db:"host_count"`
	CreatedAt   int64  `db:"created_at"`
	UpdatedAt   int64  `db:"updated_at"`
}

func (r groupRow) toGroup() Group {
	return Group{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		HostCount:   r.HostCount,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

const groupSelect = `SELECT g.id, g.name, g.description, g.created_at, g.updated_at,
	(SELECT count(*) FROM hosts h WHERE h.group_id = g.id) AS host_count
	FROM host_groups g`

func validateGroup(in GroupInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return invalid("分组名称不能为空")
	}
	if len(in.Name) > 64 {
		return invalid("分组名称不能超过 64 个字符")
	}
	if len(in.Description) > 255 {
		return invalid("分组描述不能超过 255 个字符")
	}
	return nil
}

// ListGroups returns all groups with host counts.
func (s *Service) ListGroups(ctx context.Context) ([]Group, error) {
	rows := []groupRow{}
	if err := s.db.SelectContext(ctx, &rows, groupSelect+` ORDER BY g.name`); err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	out := make([]Group, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.toGroup())
	}
	return out, nil
}

// GetGroup returns one group.
func (s *Service) GetGroup(ctx context.Context, id string) (*Group, error) {
	var r groupRow
	err := s.db.GetContext(ctx, &r, groupSelect+` WHERE g.id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get group: %w", err)
	}
	g := r.toGroup()
	return &g, nil
}

// CreateGroup inserts a group.
func (s *Service) CreateGroup(ctx context.Context, in GroupInput) (*Group, error) {
	if err := validateGroup(in); err != nil {
		return nil, err
	}
	id := store.NewULID()
	now := s.nowMs()
	_, err := s.db.ExecContext(ctx, `INSERT INTO host_groups (id, name, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`, id, strings.TrimSpace(in.Name), in.Description, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, invalid("分组名称已存在")
		}
		return nil, fmt.Errorf("insert group: %w", err)
	}
	return s.GetGroup(ctx, id)
}

// UpdateGroup modifies a group.
func (s *Service) UpdateGroup(ctx context.Context, id string, in GroupInput) (*Group, error) {
	if _, err := s.GetGroup(ctx, id); err != nil {
		return nil, err
	}
	if err := validateGroup(in); err != nil {
		return nil, err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE host_groups SET name = ?, description = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(in.Name), in.Description, s.nowMs(), id)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, invalid("分组名称已存在")
		}
		return nil, fmt.Errorf("update group: %w", err)
	}
	return s.GetGroup(ctx, id)
}

// DeleteGroup removes a group; member hosts become ungrouped.
func (s *Service) DeleteGroup(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM host_groups WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete group: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

package user

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/user_service/user_models"
	"gorm.io/gorm"
)

const (
	searchDefaultPageSize int64 = 20
	searchMaxPageSize     int64 = 100
)

type searchUserRow struct {
	ID          uint
	Email       string
	Username    string
	PhoneNumber string
	Name        string
}

// SearchUser implements [user_ifaceconnect.V2UserServiceHandler]. It looks up
// users either by a keyword (optionally scoped to a team) or by an explicit set
// of ids, with manual page/page_size pagination.
func (s *v2UserServiceImpl) SearchUser(
	ctx context.Context,
	req *connect.Request[user_iface.SearchUserRequest],
) (*connect.Response[user_iface.SearchUserResponse], error) {
	pay := req.Msg
	db := s.db.WithContext(ctx)

	q := db.
		Model(&user_models.User{}).
		Select("users.id, users.email, users.username, users.phone_number, users.name")

	switch {
	case pay.GetKeyword() != nil:
		kw := pay.GetKeyword()
		if kw.TeamId != 0 {
			q = q.Joins(
				"JOIN user_team_roles ON user_team_roles.user_id = users.id AND user_team_roles.team_id = ?",
				kw.TeamId,
			)
		}
		if term := strings.TrimSpace(kw.Q); term != "" {
			like := "%" + strings.ToLower(term) + "%"
			q = q.Where(
				"(lower(users.name) LIKE ? OR lower(users.email) LIKE ? OR lower(users.username) LIKE ? OR lower(users.phone_number) LIKE ?)",
				like, like, like, like,
			)
		}

	case pay.GetIds() != nil:
		ids := pay.GetIds().Ids
		if len(ids) == 0 {
			return connect.NewResponse(&user_iface.SearchUserResponse{Users: []*user_iface.SearchUserItem{}}), nil
		}
		q = q.Where("users.id IN ?", ids)

	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("filter is required"))
	}

	page := pay.Page
	if page < 1 {
		page = 1
	}
	size := pay.PageSize
	if size <= 0 {
		size = searchDefaultPageSize
	}
	if size > searchMaxPageSize {
		size = searchMaxPageSize
	}

	var rows []searchUserRow
	if err := q.
		Order("users.id ASC").
		Limit(int(size)).
		Offset(int((page - 1) * size)).
		Scan(&rows).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	resp := &user_iface.SearchUserResponse{Users: make([]*user_iface.SearchUserItem, 0, len(rows))}
	for _, r := range rows {
		resp.Users = append(resp.Users, &user_iface.SearchUserItem{
			Id:          uint64(r.ID),
			Email:       r.Email,
			Username:    r.Username,
			PhoneNumber: r.PhoneNumber,
			Name:        r.Name,
		})
	}

	return connect.NewResponse(resp), nil
}

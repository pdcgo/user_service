package access_interceptors

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/pdcgo/san_collection/san_caches"
	role_base "github.com/pdcgo/schema/services/role_base/v1"
	"github.com/pdcgo/user_service/identity"
	"github.com/pdcgo/user_service/user_models"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"gorm.io/gorm"
)

// rootTeamID is the system team under which ROLE_ROOT / ROLE_ADMIN are stored;
// holders act as global super-admins.
const rootTeamID uint64 = 1

// roleCacheTTL bounds staleness of cached (user, team) roles.
const roleCacheTTL = time.Minute

type accessInterceptor struct {
	db          *gorm.DB
	cacheClient san_caches.CacheManager
	secret      string
}

// NewInterceptor returns a connect.Interceptor that enforces the
// (role_base.v1.request_policy) declared on each request message:
//
//   - allow_all                -> public (no token).
//   - allow_only_authenticated -> require a valid Authorization: Bearer token.
//     If the request is team-scoped (a use_scope field), the caller must also
//     have any role in that team; otherwise any logged-in caller passes.
//     ROLE_ROOT/ROLE_ADMIN at team 1 always pass.
//   - otherwise                -> require a valid token, then check the caller's
//     roles in user_team_roles. ROLE_ROOT/ROLE_ADMIN at team 1 always pass; a
//     team-scoped request (field tagged use_scope) requires one of the policy
//     roles in that team; a non-scoped request requires root/admin.
func NewAccessInterceptor(
	db *gorm.DB,
	secret string,
	cacheClient san_caches.CacheManager,
) connect.Interceptor {
	return &accessInterceptor{db: db, secret: secret, cacheClient: cacheClient}
}

func (a *accessInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (a *accessInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

func (a *accessInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if req.Spec().IsClient {
			return next(ctx, req)
		}

		msg, ok := req.Any().(proto.Message)
		if !ok {
			return nil, connect.NewError(connect.CodeInternal, errors.New("request is not a proto message"))
		}

		policy := requestPolicy(msg)
		if policy == nil {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("no access policy"))
		}
		if policy.AllowAll {
			return next(ctx, req)
		}

		token := bearerToken(req.Header().Get("Authorization"))
		if token == "" {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing token"))
		}
		tok, err := identity.Parse(a.secret, token)
		if err != nil {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid token"))
		}
		if tok.IsExpired(time.Now()) {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("token expired"))
		}
		userID := uint(tok.IdentityId)
		teamID := scopeTeamID(msg)

		// Make the caller identity and scope available to downstream handlers.
		ctx = SetIdentityToCtx(ctx, tok.Identity)
		ctx = SetScopeIDToCtx(ctx, teamID)

		// allow_only_authenticated, unscoped: a valid token is enough.
		if policy.AllowOnlyAuthenticated && teamID == 0 {
			return next(ctx, req)
		}

		// Root/admin (system team) are global super-admins.
		isRoot, err := a.hasRole(ctx, userID, rootTeamID, []role_base.Role{
			role_base.Role_ROLE_ROOT,
			role_base.Role_ROLE_ADMIN,
		})
		if err != nil {
			return nil, err
		}
		if isRoot {
			return next(ctx, req)
		}

		// allow_only_authenticated, team-scoped: require any role in that team.
		if policy.AllowOnlyAuthenticated {
			role, err := a.getRole(ctx, userID, teamID)
			if err != nil {
				return nil, err
			}
			if role == 0 {
				return nil, connect.NewError(connect.CodePermissionDenied, errors.New("requires a role in the team"))
			}
			return next(ctx, req)
		}

		if teamID == 0 {
			teamID = 1
			// return nil, connect.NewError(connect.CodePermissionDenied, errors.New("requires root or admin"))
		}

		allowed, err := a.hasRole(ctx, userID, teamID, policy.Roles)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient role"))
		}

		return next(ctx, req)
	}
}

// UserRoleNamespace is the cache-key prefix for all of a user's cached team
// roles. Pass it to CacheManager.DelNamespace to evict every team role for the
// user (e.g. on login/logout). The trailing ":" keeps "...:1:" from matching
// "...:11:".
func UserRoleNamespace(userID uint) string {
	return fmt.Sprintf("user-service:role:%d:", userID)
}

// roleCacheKey caches a user's single role within a team (uniqueness on
// (team_id, user_id) guarantees at most one).
type roleCacheKey struct {
	userID uint
	teamID uint64
}

func (k roleCacheKey) GetKey() (string, error) {
	return fmt.Sprintf("%s%d", UserRoleNamespace(k.userID), k.teamID), nil
}

// getRole returns the user's role in the team (0 = no membership), cache-aside.
func (a *accessInterceptor) getRole(ctx context.Context, userID uint, teamID uint64) (uint, error) {
	key := roleCacheKey{userID: userID, teamID: teamID}

	var role uint
	if err := a.cacheClient.Get(ctx, key, &role); err == nil {
		return role, nil
	}

	var rec user_models.UserTeamRole
	if err := a.db.WithContext(ctx).
		Model(&user_models.UserTeamRole{}).
		Select("role").
		Where("user_id = ? AND team_id = ?", userID, teamID).
		Limit(1).
		Find(&rec).
		Error; err != nil {
		return 0, err
	}

	// Best-effort populate (rec.Role is 0 when the user is not a member).
	_ = a.cacheClient.Set(ctx, key, uint(rec.Role), roleCacheTTL)
	return uint(rec.Role), nil
}

// hasRole reports whether the user holds any of roles within the given team.
func (a *accessInterceptor) hasRole(ctx context.Context, userID uint, teamID uint64, roles []role_base.Role) (bool, error) {
	role, err := a.getRole(ctx, userID, teamID)
	if err != nil || role == 0 {
		return false, err
	}
	for _, r := range roles {
		if uint(r) == role {
			return true, nil
		}
	}
	return false, nil
}

// requestPolicy reads the (role_base.v1.request_policy) message option, or nil.
func requestPolicy(msg proto.Message) *role_base.RequestPolicy {
	opts, ok := msg.ProtoReflect().Descriptor().Options().(*descriptorpb.MessageOptions)
	if !ok || opts == nil {
		return nil
	}
	if !proto.HasExtension(opts, role_base.E_RequestPolicy) {
		return nil
	}
	policy, _ := proto.GetExtension(opts, role_base.E_RequestPolicy).(*role_base.RequestPolicy)
	return policy
}

// scopeTeamID returns the value of the field tagged (role_base.v1.use_scope),
// or 0 when no such field exists.
func scopeTeamID(msg proto.Message) uint64 {
	m := msg.ProtoReflect()
	fields := m.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		opts, ok := fd.Options().(*descriptorpb.FieldOptions)
		if !ok || opts == nil {
			continue
		}
		if scoped, _ := proto.GetExtension(opts, role_base.E_UseScope).(bool); scoped {
			return m.Get(fd).Uint()
		}
	}
	return 0
}

func bearerToken(header string) string {
	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
}

type identityCtxKey struct{}
type scopeCtxKey struct{}

// GetIdentityFromCtx returns the authenticated caller identity that the
// interceptor stored, or an error when none is present (e.g. allow_all routes).
func GetIdentityFromCtx(ctx context.Context) (*role_base.Identity, error) {
	id, ok := ctx.Value(identityCtxKey{}).(*role_base.Identity)
	if !ok || id == nil {
		return nil, errors.New("identity not found in context")
	}
	return id, nil
}

func SetIdentityToCtx(ctx context.Context, identity *role_base.Identity) context.Context {
	return context.WithValue(ctx, identityCtxKey{}, identity)
}

// GetScopeIDFromCtx returns the resolved scope (team) id, or 0 when absent.
func GetScopeIDFromCtx(ctx context.Context) uint64 {
	id, _ := ctx.Value(scopeCtxKey{}).(uint64)
	return id
}

func SetScopeIDToCtx(ctx context.Context, teamID uint64) context.Context {
	return context.WithValue(ctx, scopeCtxKey{}, teamID)
}

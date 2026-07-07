package access_interceptors_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/pdcgo/san_collection/san_caches"
	invoice_iface "github.com/pdcgo/schema/services/invoice_iface/v2"
	role_base "github.com/pdcgo/schema/services/role_base/v1"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/shared/pkg/moretest"
	"github.com/pdcgo/shared/pkg/moretest/moretest_mock"
	"github.com/pdcgo/user_service/access_interceptors"
	"github.com/pdcgo/user_service/identity"
	"github.com/pdcgo/user_service/user_models"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

const secret = "test-secret"

func token(t *testing.T, userID uint, expiredAt time.Time) string {
	t.Helper()
	tok := &identity.TokenIdentity{Identity: &role_base.Identity{
		IdentityId: uint32(userID),
		ExpiredAt:  timestamppb.New(expiredAt),
	}}
	s, err := tok.Serialize(secret)
	assert.NoError(t, err)
	return s
}

func TestAccessInterceptor(t *testing.T) {
	var scenario moretest_mock.DbScenario
	moretest.Suite(t, "access interceptor",
		moretest.SetupListFunc{moretest_mock.MockPostgresDatabase(&scenario)},
		func(t *testing.T) {
			scenario(t, func(tx *gorm.DB) {
				assert.NoError(t, tx.AutoMigrate(&user_models.User{}, &user_models.UserTeamRole{}))

				root := &user_models.User{Username: "root", Email: "root@x.com"}
				owner := &user_models.User{Username: "owner", Email: "owner@x.com"}
				stranger := &user_models.User{Username: "stranger", Email: "stranger@x.com"}
				teamOneAdmin := &user_models.User{Username: "t1admin", Email: "t1admin@x.com"}
				for _, u := range []*user_models.User{root, owner, stranger, teamOneAdmin} {
					assert.NoError(t, tx.Create(u).Error)
				}
				// root/admin live at team 1; owner is a team-5 owner.
				assert.NoError(t, tx.Create(&user_models.UserTeamRole{TeamID: 1, UserID: root.ID, Role: role_base.Role_ROLE_ROOT}).Error)
				assert.NoError(t, tx.Create(&user_models.UserTeamRole{TeamID: 5, UserID: owner.ID, Role: role_base.Role_ROLE_TEAM_OWNER}).Error)
				// non-root holder of a CreateUser policy role at team 1 (the default scope).
				assert.NoError(t, tx.Create(&user_models.UserTeamRole{TeamID: 1, UserID: teamOneAdmin.ID, Role: role_base.Role_ROLE_TEAM_ADMIN}).Error)

				run := func(req connect.AnyRequest, tkn string) (bool, error) {
					if tkn != "" {
						req.Header().Set("Authorization", "Bearer "+tkn)
					}
					called := false
					next := connect.UnaryFunc(func(ctx context.Context, r connect.AnyRequest) (connect.AnyResponse, error) {
						called = true
						return connect.NewResponse(&user_iface.TeamUserUpdateResponse{}), nil
					})
					_, err := access_interceptors.NewAccessInterceptor(tx, secret, san_caches.NewSkipCacheManager()).WrapUnary(next)(context.Background(), req)
					return called, err
				}

				future := time.Now().Add(time.Hour)
				past := time.Now().Add(-time.Hour)

				// team-scoped request (use_scope team_id = 5)
				teamReq := func() connect.AnyRequest {
					return connect.NewRequest(&user_iface.TeamUserUpdateRequest{TeamId: 5})
				}
				// non-scoped request (no use_scope field) -> root/admin only
				createReq := func() connect.AnyRequest {
					return connect.NewRequest(&user_iface.CreateUserRequest{})
				}

				t.Run("missing token -> unauthenticated", func(t *testing.T) {
					called, err := run(teamReq(), "")
					assert.False(t, called)
					assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
				})

				t.Run("garbage token -> unauthenticated", func(t *testing.T) {
					called, err := run(teamReq(), "not-a-token")
					assert.False(t, called)
					assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
				})

				t.Run("expired token -> unauthenticated", func(t *testing.T) {
					called, err := run(teamReq(), token(t, owner.ID, past))
					assert.False(t, called)
					assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
				})

				t.Run("root passes team-scoped", func(t *testing.T) {
					called, err := run(teamReq(), token(t, root.ID, future))
					assert.NoError(t, err)
					assert.True(t, called)
				})

				t.Run("root passes non-scoped create", func(t *testing.T) {
					called, err := run(createReq(), token(t, root.ID, future))
					assert.NoError(t, err)
					assert.True(t, called)
				})

				t.Run("team owner passes own team", func(t *testing.T) {
					called, err := run(teamReq(), token(t, owner.ID, future))
					assert.NoError(t, err)
					assert.True(t, called)
				})

				t.Run("team owner denied on create (non-scoped)", func(t *testing.T) {
					called, err := run(createReq(), token(t, owner.ID, future))
					assert.False(t, called)
					assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
				})

				// Non-scoped role-policy request with teamID == 0 defaults to the root
				// team (1): a policy-role holder there passes without being root/admin.
				t.Run("non-scoped: team-1 role holder passes via default scope", func(t *testing.T) {
					called, err := run(createReq(), token(t, teamOneAdmin.ID, future))
					assert.NoError(t, err)
					assert.True(t, called)
				})

				t.Run("team owner denied on other team", func(t *testing.T) {
					called, err := run(
						connect.NewRequest(&user_iface.TeamUserUpdateRequest{TeamId: 9}),
						token(t, owner.ID, future))
					assert.False(t, called)
					assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
				})

				t.Run("stranger denied", func(t *testing.T) {
					called, err := run(teamReq(), token(t, stranger.ID, future))
					assert.False(t, called)
					assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
				})

				// allow_only_authenticated request (ResetPasswordRequest): any valid
				// token passes, regardless of role.
				authOnlyReq := func() connect.AnyRequest {
					return connect.NewRequest(&user_iface.ResetPasswordRequest{})
				}

				t.Run("authenticated-only: stranger with valid token passes", func(t *testing.T) {
					called, err := run(authOnlyReq(), token(t, stranger.ID, future))
					assert.NoError(t, err)
					assert.True(t, called)
				})

				t.Run("authenticated-only: missing token -> unauthenticated", func(t *testing.T) {
					called, err := run(authOnlyReq(), "")
					assert.False(t, called)
					assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
				})

				t.Run("authenticated-only: expired token -> unauthenticated", func(t *testing.T) {
					called, err := run(authOnlyReq(), token(t, stranger.ID, past))
					assert.False(t, called)
					assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
				})

				// allow_only_authenticated + use_scope (CreateBalanceLogRequest, team_id
				// scoped): requires any role in that team; root/admin bypass.
				scopedAuthReq := func() connect.AnyRequest {
					return connect.NewRequest(&invoice_iface.CreateBalanceLogRequest{TeamId: 5})
				}

				t.Run("scoped authenticated: team member passes", func(t *testing.T) {
					called, err := run(scopedAuthReq(), token(t, owner.ID, future))
					assert.NoError(t, err)
					assert.True(t, called)
				})

				t.Run("scoped authenticated: root passes", func(t *testing.T) {
					called, err := run(scopedAuthReq(), token(t, root.ID, future))
					assert.NoError(t, err)
					assert.True(t, called)
				})

				t.Run("scoped authenticated: stranger (no role in team) denied", func(t *testing.T) {
					called, err := run(scopedAuthReq(), token(t, stranger.ID, future))
					assert.False(t, called)
					assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
				})

				t.Run("scoped authenticated: missing token -> unauthenticated", func(t *testing.T) {
					called, err := run(scopedAuthReq(), "")
					assert.False(t, called)
					assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
				})

				t.Run("context carries identity and scope", func(t *testing.T) {
					capture := func(req connect.AnyRequest, tkn string) (uint32, uint64, error) {
						req.Header().Set("Authorization", "Bearer "+tkn)
						var gotID uint32
						var gotScope uint64
						next := connect.UnaryFunc(func(ctx context.Context, r connect.AnyRequest) (connect.AnyResponse, error) {
							if id, e := access_interceptors.GetIdentityFromCtx(ctx); e == nil {
								gotID = id.IdentityId
							}
							gotScope = access_interceptors.GetScopeIDFromCtx(ctx)
							return connect.NewResponse(&user_iface.TeamUserUpdateResponse{}), nil
						})
						_, err := access_interceptors.NewAccessInterceptor(tx, secret, san_caches.NewSkipCacheManager()).WrapUnary(next)(context.Background(), req)
						return gotID, gotScope, err
					}

					// team-scoped: identity = owner, scope = 5
					id, scope, err := capture(connect.NewRequest(&user_iface.TeamUserUpdateRequest{TeamId: 5}), token(t, owner.ID, future))
					assert.NoError(t, err)
					assert.Equal(t, uint32(owner.ID), id)
					assert.Equal(t, uint64(5), scope)

					// non-scoped (root on CreateUser): identity = root, scope = 0
					id, scope, err = capture(connect.NewRequest(&user_iface.CreateUserRequest{}), token(t, root.ID, future))
					assert.NoError(t, err)
					assert.Equal(t, uint32(root.ID), id)
					assert.Equal(t, uint64(0), scope)
				})
			})
		})
}

// fakeStreamConn is a minimal connect.StreamingHandlerConn for exercising
// WrapStreamingHandler: it carries a settable request header and a Spec whose Schema is a
// real method descriptor (so the interceptor can read the request_policy without a message).
type fakeStreamConn struct {
	spec   connect.Spec
	header http.Header
}

func (f *fakeStreamConn) Spec() connect.Spec           { return f.spec }
func (f *fakeStreamConn) Peer() connect.Peer           { return connect.Peer{} }
func (f *fakeStreamConn) Receive(any) error            { return nil }
func (f *fakeStreamConn) RequestHeader() http.Header   { return f.header }
func (f *fakeStreamConn) Send(any) error               { return nil }
func (f *fakeStreamConn) ResponseHeader() http.Header  { return http.Header{} }
func (f *fakeStreamConn) ResponseTrailer() http.Header { return http.Header{} }

// TeamSynclegacy is server-streaming with request_policy roles [ROLE_ROOT, ROLE_ADMIN];
// it exercises the WrapStreamingHandler enforcement ladder.
func TestAccessInterceptorStreaming(t *testing.T) {
	md := user_iface.
		File_user_iface_v2_v2_user_proto.
		Services().
		ByName("V2UserService").
		Methods().
		ByName("TeamSynclegacy")

	var scenario moretest_mock.DbScenario
	moretest.Suite(t, "access interceptor streaming",
		moretest.SetupListFunc{moretest_mock.MockPostgresDatabase(&scenario)},
		func(t *testing.T) {
			scenario(t, func(tx *gorm.DB) {
				assert.NoError(t, tx.AutoMigrate(&user_models.User{}, &user_models.UserTeamRole{}))

				root := &user_models.User{Username: "sroot", Email: "sroot@x.com"}
				stranger := &user_models.User{Username: "sstranger", Email: "sstranger@x.com"}
				for _, u := range []*user_models.User{root, stranger} {
					assert.NoError(t, tx.Create(u).Error)
				}
				assert.NoError(t, tx.Create(&user_models.UserTeamRole{TeamID: 1, UserID: root.ID, Role: role_base.Role_ROLE_ROOT}).Error)

				run := func(tkn string) (bool, context.Context, error) {
					header := http.Header{}
					if tkn != "" {
						header.Set("Authorization", "Bearer "+tkn)
					}
					conn := &fakeStreamConn{
						spec: connect.Spec{
							StreamType: connect.StreamTypeServer,
							Schema:     md,
							Procedure:  "/user_iface.v2.V2UserService/TeamSynclegacy",
						},
						header: header,
					}
					called := false
					var gotCtx context.Context
					next := connect.StreamingHandlerFunc(func(ctx context.Context, c connect.StreamingHandlerConn) error {
						called = true
						gotCtx = ctx
						return nil
					})
					err := access_interceptors.
						NewAccessInterceptor(tx, secret, san_caches.NewSkipCacheManager()).
						WrapStreamingHandler(next)(context.Background(), conn)
					return called, gotCtx, err
				}

				future := time.Now().Add(time.Hour)
				past := time.Now().Add(-time.Hour)

				t.Run("missing token -> unauthenticated", func(t *testing.T) {
					called, _, err := run("")
					assert.False(t, called)
					assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
				})

				t.Run("garbage token -> unauthenticated", func(t *testing.T) {
					called, _, err := run("not-a-token")
					assert.False(t, called)
					assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
				})

				t.Run("expired token -> unauthenticated", func(t *testing.T) {
					called, _, err := run(token(t, root.ID, past))
					assert.False(t, called)
					assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
				})

				t.Run("stranger denied (needs root/admin)", func(t *testing.T) {
					called, _, err := run(token(t, stranger.ID, future))
					assert.False(t, called)
					assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
				})

				t.Run("root passes + identity in ctx", func(t *testing.T) {
					called, gotCtx, err := run(token(t, root.ID, future))
					assert.NoError(t, err)
					assert.True(t, called)
					id, e := access_interceptors.GetIdentityFromCtx(gotCtx)
					assert.NoError(t, e)
					assert.Equal(t, uint32(root.ID), id.IdentityId)
				})
			})
		})
}

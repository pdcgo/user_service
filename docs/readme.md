# User Service

This Is part of Submodule of [Warehouse Infra](https://github.com/pdcgo/warehouse_infra). In Warehouse Infra this is live in folder `./user_service`.<br>
This service owns **identity** for the whole platform: authentication (JWT login/token check) and
**user + team-role management** (who exists, which teams they belong to, and with what role/alias). It is
built on **Connect-RPC** + **GORM/Postgres** and deployed to **Google Cloud Run** (`make deploy-user-service`).

It is also the repo's **shared authorization**: its `access_interceptors` package (JWT parsing, identity/scope
extraction, role checks) is imported by nearly every other service to guard their own RPCs.

1. for database schema related, read this [Database Schema](database-schema.md).

## Authentication & Authorization
1. Use the **v2 roling system** (`role_base.Role` + `user_team_roles`), not the legacy system.
2. **Token / JWT** — `Login` issues an **HS256** JWT signed with `JwtSecret`; its payload is a proto-marshaled
   `role_base.v1.Identity` (identity id, type, username, agent, expiry). Default expiry **24h**. `CheckAccess`
   validates the token and **refreshes** it when expired (so an active session keeps working); `Logout` is
   stateless (client discards the token) and just evicts the caller's cached roles. See
   [identity/identity.go](../identity/identity.go). Passwords are **bcrypt** hashed.
3. **Access control** ([access_interceptors/interceptor.go](../access_interceptors/interceptor.go)) is enforced by a
   Connect interceptor reading each RPC's proto `request_policy`:
    - `allow_all` — public (no token).
    - `allow_only_authenticated` — any valid Bearer token. If the request carries a `use_scope` team field, the
      caller must hold **some** role in that team.
    - explicit `roles: [...]` — the caller must hold one of those roles (in the scoped team for `use_scope`
      requests, otherwise globally).
    - **Root team (`team_id = 1`)**: `ROLE_ROOT` / `ROLE_ADMIN` there always pass (platform super-admins).
4. **Role cache** — role lookups are cached in **Redis** (1-minute TTL, key `user-service:role:{userID}:{teamID}`);
   login/logout evict the caller's namespace so role changes take effect promptly.
5. **Exported for other services** — `GetIdentityFromCtx`, `SetIdentityToCtx`, `GetScopeIDFromCtx`,
   `SetScopeIDToCtx`, `UserRoleNamespace`, and `NewAccessInterceptor`. Other services mount
   `NewAccessInterceptor` and read the caller via `GetIdentityFromCtx(ctx)`.

## Connect RPC Spec
`user_service` heavily depends on `connect-rpc` to serve APIs (it works as both pure gRPC and grpc-web over
HTTP/2). It registers two services in [register.go](../register.go): **`V2AuthService`** (default interceptor only)
and **`V2UserService`** (default interceptor **+** the role-enforcing access interceptor). Protos:
[v2_auth.proto](../../schema/user_iface/v2/v2_auth.proto) and
[v2_user.proto](../../schema/user_iface/v2/v2_user.proto).

1. Auth RPC (`V2AuthService`)
    - `Login` — authenticate by email / username / phone + password; returns the JWT, the `User`, and the caller `Identity`.
    - `CheckAccess` — verify the token (refresh if expired) and return the caller's role in an optional `team_id` (`ROLE_UNSPECIFIED` if not a member). The frontend calls this on each page load.
    - `Logout` — evict the caller's cached roles (stateless; the client discards its token).

2. User Account RPC (`V2UserService`)
    - `CreateUser` — create a standalone user (no team). *(root/admin/team-owner/team-admin)*
    - `UpdateUser` — edit a user's email / name (password changes go through `ResetPassword`). *(root/admin/team-owner/team-admin)*
    - `DeleteUser` — hard-delete a user. *(root/admin)*
    - `SuspendUser` — suspend / unsuspend (toggles `is_suspended`). *(root/admin)*
    - `ResetPassword` — self-service password change (prove old password, set new). *(authenticated)*
    - `UserChangePhoneNumber` — two-step phone change: `otp` sends a code to the new phone, `update` verifies the code and persists (OTP via Twilio). *(authenticated)*

3. User Lookup / List RPC (`V2UserService`)
    - `SearchUser` — find users by keyword (name / email / username / phone) **or** by explicit ids; optional `team_id` scope; paginated (`page` / `page_size`). *(authenticated)*
    - `UserByIDs` — fetch users by id list, with their team aliases (per `team_id`, or all teams when `0`). *(authenticated)*
    - `UserList` — list a team's members (or all users when `team_id = 0`, root/admin only); returns a `UserMapItem` per user with per-team aliases; optional role + keyword filter; paginated. *(root/admin/team-owner/team-admin)*

4. Team Membership RPC (`V2UserService`)
    - `TeamUserUpdate` — one `oneof action`: `add` (upsert a user's role + alias in the team), `remove`, or `create` (make a brand-new user and add them). *(root/admin/team-owner/team-admin)*
    - `TeamAccessList` — list every team a user belongs to (team name, type, alias, role); `user_id = 0` means the caller's own teams. *(authenticated)*
    - `TeamSynclegacy` — **server-streaming**; migrate a team's legacy role assignments into the v2 role system, streaming progress. *(root/admin)*

## Run & Deploy
- Bootstrap: [cmd/app/](../cmd/app/) (Google Wire DI + urfave/cli). Binds `{HOST}:{PORT}` (default **`PORT=8080`**), serves h2c (HTTP/2 without TLS) wrapped in `custom_connect.WithCORS`, and inits OTel tracing as `user-service`.
- Dependencies wired: Postgres (`NewDatabase`), Redis-backed cache manager (roles), the default Connect interceptor, and Twilio OTP.
- Deploy: `make deploy-user-service` (Cloud Run). In local development it is also served by the omnibus
  [cmd/app_development](../../cmd/app_development/) on `:8086`.

## Testing
DB-backed tests use the **`moretest`** harness with Postgres (`moretest_mock.MockPostgresDatabase`) — see the
`user/*_test.go` files. Build the service with `NewV2UserService(tx, ...)` (mock OTP via
`san_verification.NewMockOtpVerification()`), `AutoMigrate` the `user_models`, then arrange/act/assert.

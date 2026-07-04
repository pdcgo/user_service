# Database Schema & Model

The service owns two tables (in [user_models/](../user_models/)): the user account and the user↔team
membership. Team rows and the `role_base.Role` enum themselves live in `shared/db_models` — this service
references them but does not own the `teams` table. Migrations are **goose** SQL under
[db_migrations/](../db_migrations/).

Schema and Model that have:
1. User
2. UserTeamRole

## User Schema  ·  table `users`
`user_models.User` — the account.

1. Fields:
    - `name`
    - `profile_picture`
    - `username`              <-- unique
    - `password`             <-- bcrypt hash
    - `email`                 <-- unique
    - `phone_number`
    - `is_suspended`          <-- suspend/unsuspend flag (SuspendUser)
    - `last_password_reset`
    - `created_at`
2. `username` and `email` each have a **unique index**.
3. Passwords are stored **bcrypt-hashed** (never plaintext); phone changes are OTP-verified.

## UserTeamRole Schema  ·  table `user_team_roles`
`user_models.UserTeamRole` — the v2 membership: which role (and per-team alias) a user has in a team.

1. Fields:
    - `team_id`               <-- for scoped
    - `user_id`
    - `role`                  <-- `role_base.v1.Role` enum (ROLE_ROOT, ROLE_ADMIN, ROLE_TEAM_OWNER, ROLE_TEAM_ADMIN, …)
    - `alias`                 <-- the user's team-specific display name
    - `created_at`
2. (`team_id`, `user_id`) is **unique** — one role row per user per team; `TeamUserUpdate{add}` upserts it,
   `TeamUserUpdate{remove}` deletes it.
3. This is the source of truth for authorization (see the Access Control section in the [readme](readme.md)) — the
   access interceptor reads roles from here (cached in Redis).

## Relationships & legacy
- A user belongs to many teams via `user_team_roles` (v2). This **replaces** the legacy
  `shared/db_models.UserTeam` (user↔team by alias only, no role) — that model still exists in `shared/db_models`
  for legacy compatibility but the v2 flows use `UserTeamRole`.
- `shared/db_models.Team` holds team identity (name, `team_code`, `type`: root/admin/warehouse/selling). The
  **root team is `id = 1`** — `ROLE_ROOT`/`ROLE_ADMIN` there are platform super-admins.

## Migrations
Goose SQL in [db_migrations/](../db_migrations/):
- `00001_create_users.sql` — `users` (+ unique username/email).
- `00002_create_user_team_roles.sql` — `user_team_roles` (+ unique `(team_id, user_id)`).
- `00003_add_alias_to_user_team_roles.sql` — adds the per-team `alias` column.

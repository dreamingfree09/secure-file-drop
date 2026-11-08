#!/bin/sh
set -eu

# The official entrypoint supplies PGPASSWORD for the temporary server.
psql --set ON_ERROR_STOP=1 --username postgres --dbname sfd <<'SQL'
\getenv app_password SFD_PG_APP_SECRET
CREATE ROLE sfd LOGIN NOSUPERUSER NOCREATEROLE NOCREATEDB NOREPLICATION PASSWORD :'app_password';
ALTER DATABASE sfd OWNER TO sfd;
SQL

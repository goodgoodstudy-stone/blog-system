#!/usr/bin/env bash
set -euo pipefail

required=(
  DEPLOY_HOST DEPLOY_USER DEPLOY_SSH_KEY DEPLOY_KNOWN_HOSTS
  DEPLOY_MYSQL_ROOT_PASSWORD DEPLOY_MYSQL_PASSWORD DEPLOY_DEMO_PASSWORD
  DEPLOY_PUBLIC_ORIGIN GHCR_TOKEN GITHUB_ACTOR GITHUB_REPOSITORY GITHUB_REF_NAME
)
for name in "${required[@]}"; do
  if [[ -z "${!name:-}" ]]; then
    printf 'Missing required deployment setting: %s\n' "$name" >&2
    exit 1
  fi
done

ssh_port="${DEPLOY_SSH_PORT:-22}"
public_bind="${DEPLOY_PUBLIC_BIND:-127.0.0.1}"
public_port="${DEPLOY_PUBLIC_PORT:-8080}"
for name in DEPLOY_MYSQL_ROOT_PASSWORD DEPLOY_MYSQL_PASSWORD DEPLOY_DEMO_PASSWORD; do
  if [[ ! "${!name}" =~ ^[A-Za-z0-9_-]{16,}$ ]]; then
    printf '%s must be at least 16 characters and use only letters, digits, _ or -\n' "$name" >&2
    exit 1
  fi
done
if [[ ! "$DEPLOY_HOST" =~ ^[A-Za-z0-9.-]+$ ]] ||
   [[ ! "$DEPLOY_USER" =~ ^[A-Za-z_][A-Za-z0-9_-]*$ ]] ||
   [[ ! "$GITHUB_ACTOR" =~ ^[A-Za-z0-9-]+$ ]] ||
   [[ ! "$ssh_port" =~ ^[0-9]{1,5}$ ]] ||
   [[ ! "$public_port" =~ ^[0-9]{1,5}$ ]] ||
   [[ ! "$public_bind" =~ ^(127\.0\.0\.1|0\.0\.0\.0)$ ]] ||
   [[ ! "$DEPLOY_PUBLIC_ORIGIN" =~ ^https?://[A-Za-z0-9.:-]+$ ]]; then
  echo 'Invalid deployment host, user, port, bind address or public origin' >&2
  exit 1
fi

temporary_dir="$(mktemp -d)"
trap 'rm -rf "$temporary_dir"' EXIT
umask 077
printf '%s\n' "$DEPLOY_SSH_KEY" > "$temporary_dir/id_deploy"
printf '%s\n' "$DEPLOY_KNOWN_HOSTS" > "$temporary_dir/known_hosts"

image_prefix="ghcr.io/${GITHUB_REPOSITORY,,}"
cat > "$temporary_dir/.env" <<EOF
MYSQL_ROOT_PASSWORD=$DEPLOY_MYSQL_ROOT_PASSWORD
MYSQL_PASSWORD=$DEPLOY_MYSQL_PASSWORD
DEMO_PASSWORD=$DEPLOY_DEMO_PASSWORD
PUBLIC_ORIGIN=$DEPLOY_PUBLIC_ORIGIN
PUBLIC_BIND=$public_bind
PUBLIC_PORT=$public_port
IMAGE_PREFIX=$image_prefix
IMAGE_TAG=$GITHUB_REF_NAME
EOF

ssh_options=(
  -i "$temporary_dir/id_deploy"
  -o BatchMode=yes
  -o IdentitiesOnly=yes
  -o StrictHostKeyChecking=yes
  -o "UserKnownHostsFile=$temporary_dir/known_hosts"
)
target="$DEPLOY_USER@$DEPLOY_HOST"
ssh "${ssh_options[@]}" -p "$ssh_port" "$target" 'mkdir -p "$HOME/blog-system" && chmod 700 "$HOME/blog-system"'
scp "${ssh_options[@]}" -P "$ssh_port" \
  compose.release.yaml "$temporary_dir/.env" "$target:blog-system/"
ssh "${ssh_options[@]}" -p "$ssh_port" "$target" 'chmod 600 "$HOME/blog-system/.env"'

printf '%s' "$GHCR_TOKEN" | ssh "${ssh_options[@]}" -p "$ssh_port" "$target" \
  "set -eu; cd \"\$HOME/blog-system\"; trap 'docker logout ghcr.io >/dev/null 2>&1 || true' EXIT; docker login ghcr.io -u '$GITHUB_ACTOR' --password-stdin >/dev/null; docker compose -f compose.release.yaml pull; docker compose -f compose.release.yaml up -d --wait"

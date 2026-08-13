#!/usr/bin/env bash
set -Eeuo pipefail

SERVICE_DIR="/home/kterna/services"
BASE_COMPOSE="$SERVICE_DIR/docker-compose.yml"
STATE_DIR="$SERVICE_DIR/newapi-deploy"
OVERRIDE_COMPOSE="$SERVICE_DIR/docker-compose.override.yml"
STATUS_FILE="$STATE_DIR/status.json"
LOCK_FILE="$STATE_DIR/deploy.lock"
LOG_DIR="$STATE_DIR/logs"
CONTAINER_NAME="new-api"
HEALTH_URL="http://127.0.0.1:3000/api/status"
HEALTH_TIMEOUT_SECONDS=120
DRAIN_SECONDS=15

mkdir -p "$STATE_DIR" "$LOG_DIR"

usage() {
  cat <<'EOF'
Usage:
  deploy-newapi-detached.sh deploy <image>
  deploy-newapi-detached.sh status
  deploy-newapi-detached.sh logs [lines]

The deploy command starts a detached worker and returns immediately. The worker
pulls the image, recreates only the new-api service, verifies health, and rolls
back automatically if the new container does not become healthy.
EOF
}

write_status() {
  local run_id="$1"
  local state="$2"
  local image="$3"
  local previous_image="$4"
  local message="$5"
  local updated_at
  updated_at="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
  local tmp="$STATUS_FILE.tmp.$$"
  jq -n \
    --arg run_id "$run_id" \
    --arg state "$state" \
    --arg image "$image" \
    --arg previous_image "$previous_image" \
    --arg message "$message" \
    --arg updated_at "$updated_at" \
    '{run_id:$run_id,state:$state,image:$image,previous_image:$previous_image,message:$message,updated_at:$updated_at}' \
    >"$tmp"
  mv -f "$tmp" "$STATUS_FILE"
}

write_override() {
  local image="$1"
  local tmp="$OVERRIDE_COMPOSE.tmp.$$"
  printf 'services:\n  new-api:\n    image: %s\n' "$image" >"$tmp"
  mv -f "$tmp" "$OVERRIDE_COMPOSE"
}

compose() {
  docker compose -f "$BASE_COMPOSE" -f "$OVERRIDE_COMPOSE" "$@"
}

is_healthy() {
  curl -fsS --max-time 5 "$HEALTH_URL" | jq -e '.success == true' >/dev/null
}

wait_healthy() {
  local deadline=$((SECONDS + HEALTH_TIMEOUT_SECONDS))
  while (( SECONDS < deadline )); do
    if is_healthy; then
      return 0
    fi
    sleep 2
  done
  return 1
}

worker() {
  local image="$1"
  local run_id="$2"
  local log_file="$LOG_DIR/$run_id.log"

  exec >>"$log_file" 2>&1
  exec 9>"$LOCK_FILE"
  if ! flock -n 9; then
    write_status "$run_id" "rejected" "$image" "" "another deployment is already running"
    echo "[$(date -Is)] another deployment is already running"
    exit 1
  fi

  local previous_image
  previous_image="$(docker inspect "$CONTAINER_NAME" --format '{{.Config.Image}}' 2>/dev/null || true)"
  if [[ -z "$previous_image" ]]; then
    previous_image="$(cd "$SERVICE_DIR" && docker compose -f "$BASE_COMPOSE" config --format json | jq -r '.services["new-api"].image')"
  fi

  echo "[$(date -Is)] deployment $run_id started"
  echo "[$(date -Is)] previous image: $previous_image"
  echo "[$(date -Is)] target image:   $image"
  write_status "$run_id" "pulling" "$image" "$previous_image" "pulling target image while current service remains online"

  write_override "$image"
  if ! compose pull new-api; then
    write_override "$previous_image"
    write_status "$run_id" "failed" "$image" "$previous_image" "image pull failed; current container was not changed"
    echo "[$(date -Is)] image pull failed; current container was not changed"
    exit 1
  fi

  write_status "$run_id" "ready_to_recreate" "$image" "$previous_image" "image pulled; waiting briefly before detached recreate"
  echo "[$(date -Is)] image pulled; waiting ${DRAIN_SECONDS}s before recreate"
  sleep "$DRAIN_SECONDS"

  write_status "$run_id" "recreating" "$image" "$previous_image" "recreating new-api container"
  if ! compose up -d --no-deps --force-recreate --no-build new-api; then
    echo "[$(date -Is)] recreate command failed; rolling back"
    write_override "$previous_image"
    compose up -d --no-deps --force-recreate --no-build new-api || true
    if wait_healthy; then
      write_status "$run_id" "rolled_back" "$image" "$previous_image" "recreate failed; previous image restored and healthy"
    else
      write_status "$run_id" "rollback_failed" "$image" "$previous_image" "recreate failed and previous image did not recover health"
    fi
    exit 1
  fi

  write_status "$run_id" "health_check" "$image" "$previous_image" "waiting for new-api health endpoint"
  if wait_healthy; then
    local running_image
    running_image="$(docker inspect "$CONTAINER_NAME" --format '{{.Config.Image}}')"
    write_status "$run_id" "succeeded" "$image" "$previous_image" "healthy; running image: $running_image"
    echo "[$(date -Is)] deployment succeeded; running image: $running_image"
    exit 0
  fi

  echo "[$(date -Is)] health check failed; rolling back to $previous_image"
  write_status "$run_id" "rolling_back" "$image" "$previous_image" "new image failed health check; restoring previous image"
  docker logs --tail 200 "$CONTAINER_NAME" || true
  write_override "$previous_image"
  compose up -d --no-deps --force-recreate --no-build new-api || true

  if wait_healthy; then
    write_status "$run_id" "rolled_back" "$image" "$previous_image" "new image failed health check; previous image restored and healthy"
    echo "[$(date -Is)] rollback succeeded"
  else
    write_status "$run_id" "rollback_failed" "$image" "$previous_image" "new image failed and previous image did not recover health"
    echo "[$(date -Is)] rollback failed"
  fi
  exit 1
}

case "${1:-}" in
  deploy)
    image="${2:-}"
    if [[ -z "$image" ]]; then
      usage
      exit 2
    fi
    if [[ ! "$image" =~ ^[A-Za-z0-9._/:@-]+$ ]]; then
      echo "invalid image reference: $image" >&2
      exit 2
    fi
    if ! command -v docker >/dev/null || ! command -v jq >/dev/null || ! command -v curl >/dev/null || ! command -v flock >/dev/null; then
      echo "required command missing (docker, jq, curl, or flock)" >&2
      exit 1
    fi
    run_id="$(date -u +'%Y%m%dT%H%M%SZ')-$$"
    log_file="$LOG_DIR/$run_id.log"
    write_status "$run_id" "queued" "$image" "" "detached deployment worker queued"
    nohup setsid "$0" _worker "$image" "$run_id" </dev/null >>"$log_file.launcher" 2>&1 &
    worker_pid=$!
    echo "deployment queued"
    echo "run_id=$run_id"
    echo "worker_pid=$worker_pid"
    echo "status: $0 status"
    echo "logs:   $0 logs"
    ;;
  status)
    if [[ -f "$STATUS_FILE" ]]; then
      cat "$STATUS_FILE"
    else
      echo '{"state":"never_run"}'
    fi
    echo
    if docker inspect "$CONTAINER_NAME" >/dev/null 2>&1; then
      docker inspect "$CONTAINER_NAME" --format 'container={{.Name}} image={{.Config.Image}} status={{.State.Status}} started={{.State.StartedAt}}'
      if is_healthy; then
        echo "health=healthy"
      else
        echo "health=unhealthy"
      fi
    fi
    ;;
  logs)
    lines="${2:-100}"
    if [[ ! "$lines" =~ ^[0-9]+$ ]]; then
      echo "lines must be a positive integer" >&2
      exit 2
    fi
    latest_log="$(ls -1t "$LOG_DIR"/*.log 2>/dev/null | head -1 || true)"
    if [[ -z "$latest_log" ]]; then
      echo "no deployment log found"
      exit 0
    fi
    echo "==> $latest_log <=="
    tail -n "$lines" "$latest_log"
    ;;
  _worker)
    worker "${2:?missing image}" "${3:?missing run id}"
    ;;
  *)
    usage
    exit 2
    ;;
esac

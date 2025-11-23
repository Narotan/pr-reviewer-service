#!/usr/bin/env bash
# Скрипт для проверки основных endpoint'ов сервиса PR-reviewer

set -euo pipefail
IFS=$'\n\t'

BASE=${1:-http://localhost:8080}

# Три тестовых пользователя и два PR
A="00000000-0000-0000-0000-00000000000a"
B="00000000-0000-0000-0000-00000000000b"
C="00000000-0000-0000-0000-00000000000c"
PR1="11111111-1111-1111-1111-111111111111"
PR2="22222222-2222-2222-2222-222222222222"
TEAM="test_team_api"

hdr() {
  echo
  echo "================================================================"
  echo "$1"
  echo "----------------------------------------------------------------"
}

req() {
  local method=$1
  local path=$2
  local data=${3-}

  local url="$BASE$path"
  hdr "$method $url"

  if [[ -n "$data" ]]; then
    response=$(curl -sS -w "\n%{http_code}" -X "$method" -H 'Content-Type: application/json' -d "$data" "$url" ) || true
  else
    response=$(curl -sS -w "\n%{http_code}" -X "$method" "$url" ) || true
  fi

  http_code=$(echo "$response" | tail -n1)
  body=$(echo "$response" | sed '$d')

  echo "HTTP $http_code"
  if command -v jq >/dev/null 2>&1; then
    if [[ -n "$body" ]]; then
      echo "$body" | jq || echo "$body"
    else
      echo "<empty body>"
    fi
  else
    echo "$body"
  fi
}

set +e

# 1) Create team
req POST "/team/add" '{"team_name":"'$TEAM'","members":[{"user_id":"'$A'","username":"alice","is_active":true},{"user_id":"'$B'","username":"bob","is_active":true},{"user_id":"'$C'","username":"charlie","is_active":true}]}'

# 2) Get team
req GET "/team/get?team_name=$TEAM"

# 3) Create PR1 by alice
req POST "/pullRequest/create" '{"pull_request_id":"'$PR1'","pull_request_name":"feat/one","author_id":"'$A'"}'

# 4) Get PRs for bob
req GET "/users/getReview?user_id=$B"

# 5) Get PRs for charlie
req GET "/users/getReview?user_id=$C"

# 6) Stats
req GET "/stats/assignments"

# 7) Deactivate charlie
req POST "/users/setIsActive" '{"user_id":"'$C'","is_active":false}'

# 8) Create PR2 by alice
req POST "/pullRequest/create" '{"pull_request_id":"'$PR2'","pull_request_name":"feat/two","author_id":"'$A'"}'

# 9) Merge PR1
req POST "/pullRequest/merge" '{"pull_request_id":"'$PR1'"}'

# 10) Try reassign on PR2 old reviewer = bob
req POST "/pullRequest/reassign" '{"pull_request_id":"'$PR2'","old_user_id":"'$B'"}'

req GET "/stats/assignments"

set -e

echo
echo "Done."


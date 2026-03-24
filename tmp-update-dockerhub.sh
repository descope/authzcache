#!/usr/bin/env bash
set -euo pipefail

# --- CONFIG ---
# Docker Hub
DOCKERHUB_NAMESPACE="descope"          # e.g. your username or org
DOCKERHUB_REPO="authzcache"                    # e.g. my-image
DOCKERHUB_TOKEN="${DOCKERHUB_TOKEN:?missing}" # store as env/secret

# GitHub
GH_OWNER="descope"     # e.g. ariansvi
GH_REPO="authzcache"       # e.g. my-image
README_PATH="README.md"      # change if your readme is elsewhere
# optionally: GH_TOKEN for private repos (GitHub PAT) -> export GH_TOKEN=...

API_GH="https://api.github.com/repos/${GH_OWNER}/${GH_REPO}"

# --- FETCH DEFAULT BRANCH NAME ---
if [[ -z "${GH_TOKEN:-}" ]]; then
  DEFAULT_BRANCH="$(curl -fsSL "$API_GH" | jq -r .default_branch)"
else
  DEFAULT_BRANCH="$(curl -fsSL -H "Authorization: Bearer ${GH_TOKEN}" "$API_GH" | jq -r .default_branch)"
fi

# --- DOWNLOAD RAW README CONTENT ---
RAW_URL="https://raw.githubusercontent.com/${GH_OWNER}/${GH_REPO}/${DEFAULT_BRANCH}/${README_PATH}"
if [[ -z "${GH_TOKEN:-}" ]]; then
  curl -fsSL "$RAW_URL" > /tmp/README.md
else
  curl -fsSL -H "Authorization: Bearer ${GH_TOKEN}" "$RAW_URL" > /tmp/README.md
fi

# --- JSON ENCODE + PATCH DOCKER HUB FULL DESCRIPTION ---
JSON_PAYLOAD="$(jq -Rs '{full_description: .}' /tmp/README.md)"


curl -fsS -X PATCH \
  -H "Authorization: JWT ${DOCKERHUB_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "${JSON_PAYLOAD}" \
  "https://hub.docker.com/v2/repositories/${DOCKERHUB_NAMESPACE}/${DOCKERHUB_REPO}/"

echo "✅ Docker Hub README updated for ${DOCKERHUB_NAMESPACE}/${DOCKERHUB_REPO}"

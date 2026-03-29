#!/usr/bin/env bash
set -euo pipefail

# ============================================================
# Kube Foundry Agent Entrypoint (OpenCode)
#
# Required environment variables:
#   TASK_DESCRIPTION  - Natural language task for the agent
#   REPO_URL          - HTTPS URL of the git repository
#   BASE_BRANCH       - Branch to clone and work from
#   WORK_BRANCH       - Branch name to create for the work
#   TASK_NAME         - Name of the SoftwareTask CR
#   ANTHROPIC_API_KEY - API key for Anthropic (used by OpenCode)
#   GITHUB_TOKEN      - GitHub token for git push and PR creation
# ============================================================

TERMINATION_LOG="/tmp/termination-log"

terminate() {
    local exit_code=$1
    local message=$2
    echo "${message}" > "${TERMINATION_LOG}"
    exit "${exit_code}"
}

echo "=== Kube Foundry Agent Starting (OpenCode) ==="
echo "Task:   ${TASK_NAME}"
echo "Repo:   ${REPO_URL}"
echo "Branch: ${BASE_BRANCH} -> ${WORK_BRANCH}"
echo "======================================="

# --------------------------------------------------
# Step 1: Configure git credentials
# --------------------------------------------------
echo "[1/6] Configuring git credentials..."

git config --global url."https://x-access-token:${GITHUB_TOKEN}@github.com/".insteadOf "https://github.com/"
git config --global user.name "${GIT_AUTHOR_NAME:-Bot}"
git config --global user.email "${GIT_AUTHOR_EMAIL:-bot@users.noreply.github.com}"

echo "${GITHUB_TOKEN}" | gh auth login --with-token 2>/dev/null || true
gh auth setup-git 2>/dev/null || true

# --------------------------------------------------
# Step 2: Clone the repository
# --------------------------------------------------
echo "[2/6] Cloning repository..."
cd /workspace
git clone --branch "${BASE_BRANCH}" --single-branch --depth=50 "${REPO_URL}" repo
cd repo

# --------------------------------------------------
# Step 3: Create work branch
# --------------------------------------------------
echo "[3/6] Creating work branch: ${WORK_BRANCH}..."
git checkout -b "${WORK_BRANCH}"

# --------------------------------------------------
# Step 3.5: Apply skill files and init commands
# --------------------------------------------------
if [ -n "${SKILL_FILES:-}" ]; then
    echo "[skills] Injecting skill files..."
    echo "${SKILL_FILES}" | jq -c '.[]' | while read -r entry; do
        fpath=$(echo "${entry}" | jq -r '.path')
        fcontent=$(echo "${entry}" | jq -r '.content')
        mkdir -p "$(dirname "${fpath}")"
        echo "${fcontent}" > "${fpath}"
        echo "  -> ${fpath}"
    done
fi

if [ -n "${SKILL_MCP_SERVERS:-}" ]; then
    echo "[skills] Configuring MCP servers..."

    # OpenCode uses config.json with mcpServers in its own format:
    # { "mcpServers": { "name": { "command": "...", "args": [...], "env": {...} } } }
    # or for remote: { "mcpServers": { "name": { "url": "...", "headers": {...} } } }
    MCP_CONFIG=$(echo "${SKILL_MCP_SERVERS}" | jq -c '
        reduce .[] as $srv ({};
            if $srv.url then
                .[$srv.name] = {
                    url: $srv.url
                } + (if $srv.headers then {headers: $srv.headers} else {} end)
            else
                .[$srv.name] = {
                    command: $srv.command
                } + (if $srv.args then {args: $srv.args} else {} end)
                  + (if $srv.env then {env: $srv.env} else {} end)
            end
        )
    ')

    CONFIG_DIR="${HOME}/.config/opencode"
    CONFIG_FILE="${CONFIG_DIR}/config.json"
    mkdir -p "${CONFIG_DIR}"

    if [ -f "${CONFIG_FILE}" ]; then
        EXISTING=$(cat "${CONFIG_FILE}")
        echo "${EXISTING}" | jq --argjson mcp "${MCP_CONFIG}" '.mcpServers = (.mcpServers // {} | . * $mcp)' > "${CONFIG_FILE}"
    else
        jq -n --argjson mcp "${MCP_CONFIG}" '{mcpServers: $mcp}' > "${CONFIG_FILE}"
    fi
    echo "  -> ${CONFIG_FILE} ($(echo "${MCP_CONFIG}" | jq 'length') servers)"
fi

if [ -n "${SKILL_INIT_COMMANDS:-}" ]; then
    echo "[skills] Running init commands..."
    echo "${SKILL_INIT_COMMANDS}" | jq -r '.[]' | while read -r cmd; do
        echo "  \$ ${cmd}"
        eval "${cmd}"
    done
fi

# --------------------------------------------------
# Step 4: Run OpenCode agent
# --------------------------------------------------
echo "[4/6] Running OpenCode agent..."
echo "Task description: ${TASK_DESCRIPTION}"

opencode run "${TASK_DESCRIPTION}" \
    2>&1 | tee /tmp/opencode-output.log

OPENCODE_EXIT=${PIPESTATUS[0]}
if [ "${OPENCODE_EXIT}" -ne 0 ]; then
    terminate 1 "OpenCode agent failed with exit code ${OPENCODE_EXIT}"
fi

# --------------------------------------------------
# Step 5: Commit and push changes
# --------------------------------------------------
echo "[5/6] Committing and pushing changes..."

if git diff --quiet && git diff --cached --quiet && [ -z "$(git ls-files --others --exclude-standard)" ]; then
    terminate 1 "Agent produced no changes"
fi

git add -A

git diff --cached --quiet || git commit -m "$(cat <<EOF
factory(${TASK_NAME}): ${TASK_DESCRIPTION:0:72}

Automated by Kube Foundry
Task: ${TASK_NAME}
EOF
)"

git push origin "${WORK_BRANCH}"

# --------------------------------------------------
# Step 6: Create Pull Request
# --------------------------------------------------
echo "[6/6] Creating pull request..."

PR_TITLE="[Factory] ${TASK_DESCRIPTION:0:72}"
PR_BODY="## Automated by Kube Foundry

**Task:** \`${TASK_NAME}\`

### Description
${TASK_DESCRIPTION}

---
*This PR was created automatically by [Kube Foundry](https://github.com/kube-foundry/kube-foundry).*"

PR_URL=$(gh pr create \
    --title "${PR_TITLE}" \
    --body "${PR_BODY}" \
    --base "${BASE_BRANCH}" \
    --head "${WORK_BRANCH}" \
    2>&1) || true

if [ -n "${PR_URL}" ] && echo "${PR_URL}" | grep -q "https://"; then
    echo "=== PR Created: ${PR_URL} ==="
    terminate 0 "${PR_URL}"
else
    echo "Warning: PR creation may have failed, but changes were pushed to ${WORK_BRANCH}"
    terminate 0 "pushed:${WORK_BRANCH}"
fi

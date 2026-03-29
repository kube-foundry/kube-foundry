#!/usr/bin/env bash
set -euo pipefail

# ============================================================
# Kube Foundry Agent Entrypoint
#
# Required environment variables:
#   TASK_DESCRIPTION  - Natural language task for the agent
#   REPO_URL          - HTTPS URL of the git repository
#   BASE_BRANCH       - Branch to clone and work from
#   WORK_BRANCH       - Branch name to create for the work
#   TASK_NAME         - Name of the SoftwareTask CR
#   ANTHROPIC_API_KEY - API key for Claude
#   GITHUB_TOKEN      - GitHub token for git push and PR creation
# ============================================================

TERMINATION_LOG="/tmp/termination-log"

terminate() {
    local exit_code=$1
    local message=$2
    echo "${message}" > "${TERMINATION_LOG}"
    exit "${exit_code}"
}

echo "=== Kube Foundry Agent Starting ==="
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
# Step 3.5: Apply skill prompts, files, and init commands
# --------------------------------------------------
if [ -n "${SKILL_PROMPTS:-}" ]; then
    echo "[skills] Installing skill prompts..."
    mkdir -p .claude/skills
    echo "${SKILL_PROMPTS}" | jq -c '.[]' | while read -r entry; do
        sname=$(echo "${entry}" | jq -r '.name')
        scontent=$(echo "${entry}" | jq -r '.content')
        echo "${scontent}" > ".claude/skills/${sname}.md"
        echo "  -> .claude/skills/${sname}.md"
    done
fi

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
    mkdir -p .claude

    # Build the mcpServers object for Claude Code settings.json
    # Each server is either remote (url) or stdio (command)
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

    # Merge into existing settings.json or create new one
    if [ -f .claude/settings.json ]; then
        EXISTING=$(cat .claude/settings.json)
        echo "${EXISTING}" | jq --argjson mcp "${MCP_CONFIG}" '.mcpServers = (.mcpServers // {} | . * $mcp)' > .claude/settings.json
    else
        jq -n --argjson mcp "${MCP_CONFIG}" '{mcpServers: $mcp}' > .claude/settings.json
    fi
    echo "  -> .claude/settings.json ($(echo "${MCP_CONFIG}" | jq 'length') servers)"
fi

if [ -n "${SKILL_INIT_COMMANDS:-}" ]; then
    echo "[skills] Running init commands..."
    echo "${SKILL_INIT_COMMANDS}" | jq -r '.[]' | while read -r cmd; do
        echo "  \$ ${cmd}"
        eval "${cmd}"
    done
fi

# --------------------------------------------------
# Step 4: Run Claude Code agent
# --------------------------------------------------
echo "[4/6] Running Claude Code agent..."
echo "Task description: ${TASK_DESCRIPTION}"

claude -p "${TASK_DESCRIPTION}" \
    --allowedTools "Bash,Read,Write,Edit,Glob,Grep" \
    --output-format text \
    2>&1 | tee /tmp/claude-output.log

CLAUDE_EXIT=${PIPESTATUS[0]}
if [ "${CLAUDE_EXIT}" -ne 0 ]; then
    terminate 1 "Claude Code agent failed with exit code ${CLAUDE_EXIT}"
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

#!/usr/bin/env bash
set -euo pipefail

required_variables=(
  IOS_SIMULATOR_UDID
  MAESTRO_EXPO_DEV_CLIENT_URL
  MAESTRO_E2E_API_URL
  MAESTRO_E2E_LLM_BASE_URL
  MAESTRO_E2E_USERNAME
  MAESTRO_E2E_EMAIL
  MAESTRO_E2E_PASSWORD
)

for variable_name in "${required_variables[@]}"; do
  if [[ -z "${!variable_name:-}" ]]; then
    echo "Missing required environment variable: ${variable_name}" >&2
    exit 2
  fi
done

for command_name in maestro node xcrun; do
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "Required command not found: ${command_name}" >&2
    exit 2
  fi
done

mobile_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
workspace_root="$(cd "${mobile_dir}/../../.." && pwd)"
fixture_path="${mobile_dir}/e2e/maestro/fixtures/knowledge-upload.md"
run_id="${E2E_RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)-knowledge-upload}"
export MAESTRO_E2E_PROCESSING_TIMEOUT_MS="${MAESTRO_E2E_PROCESSING_TIMEOUT_MS:-120000}"

if [[ ! "${run_id}" =~ ^[A-Za-z0-9._-]+$ ]]; then
  echo "E2E_RUN_ID may contain only letters, numbers, dots, underscores, and hyphens." >&2
  exit 2
fi
if [[ ! "${MAESTRO_E2E_PROCESSING_TIMEOUT_MS}" =~ ^[0-9]+$ ]] \
  || (( MAESTRO_E2E_PROCESSING_TIMEOUT_MS < 1000 || MAESTRO_E2E_PROCESSING_TIMEOUT_MS > 600000 )); then
  echo "MAESTRO_E2E_PROCESSING_TIMEOUT_MS must be an integer from 1000 through 600000." >&2
  exit 2
fi
if [[ ! -f "${fixture_path}" ]]; then
  echo "Controlled fixture not found: ${fixture_path}" >&2
  exit 2
fi

artifact_dir="${workspace_root}/output/ios-simulator/runs/${run_id}"
recording_path="${artifact_dir}/evidence/knowledge-upload.mp4"
recording_pid=""
automated_fixture_staged="0"
simulator_fixture_path=""
mkdir -p "${artifact_dir}/evidence" "${artifact_dir}/maestro-debug"

cleanup() {
  local exit_status=$?
  trap - EXIT
  if [[ -n "${recording_pid}" ]] && kill -0 "${recording_pid}" 2>/dev/null; then
    kill -INT "${recording_pid}" 2>/dev/null || true
    wait "${recording_pid}" 2>/dev/null || true
  fi
  if [[ "${automated_fixture_staged}" == "1" ]]; then
    if ! xcrun simctl spawn "${IOS_SIMULATOR_UDID}" /bin/rm -f "${simulator_fixture_path}"; then
      echo "Failed to remove the Simulator temporary fixture: ${simulator_fixture_path}" >&2
      exit_status=1
    fi
    if ! xcrun simctl launch "${IOS_SIMULATOR_UDID}" com.apple.DocumentsApp >/dev/null; then
      echo "Failed to launch Files for fixture cleanup." >&2
      exit_status=1
    elif ! maestro test \
      --udid "${IOS_SIMULATOR_UDID}" \
      --test-output-dir "${artifact_dir}/evidence" \
      --debug-output "${artifact_dir}/maestro-debug/cleanup" \
      "${mobile_dir}/e2e/maestro/flows/cleanup-staged-knowledge-upload.yaml"; then
      echo "Failed to remove the controlled fixture from Files." >&2
      exit_status=1
    fi
  fi
  if ! node "${mobile_dir}/e2e/maestro/sanitize-artifacts.mjs" "${artifact_dir}"; then
    echo "Failed to sanitize Maestro artifacts: ${artifact_dir}" >&2
    exit_status=1
  fi
  exit "${exit_status}"
}

trap cleanup EXIT

export MAESTRO_CLI_NO_ANALYTICS="${MAESTRO_CLI_NO_ANALYTICS:-true}"
export MAESTRO_CLI_ANALYSIS_NOTIFICATION_DISABLED="${MAESTRO_CLI_ANALYSIS_NOTIFICATION_DISABLED:-true}"

if [[ "${MAESTRO_E2E_FILES_READY:-}" == "1" ]]; then
  export MAESTRO_E2E_UPLOAD_FILE_STEM="${MAESTRO_E2E_UPLOAD_FILE_STEM:-knowledge-upload}"
else
  generated_fixture_stem="$(node -e 'const crypto=require("node:crypto");const hash=crypto.createHash("sha256").update(process.argv[1]).digest("hex").slice(0,12);process.stdout.write(`cove-ku-${hash}`);' "${run_id}")"
  export MAESTRO_E2E_UPLOAD_FILE_STEM="${MAESTRO_E2E_UPLOAD_FILE_STEM:-${generated_fixture_stem}}"
fi
if [[ ! "${MAESTRO_E2E_UPLOAD_FILE_STEM}" =~ ^[A-Za-z0-9._-]+$ ]]; then
  echo "MAESTRO_E2E_UPLOAD_FILE_STEM may contain only letters, numbers, dots, underscores, and hyphens." >&2
  exit 2
fi
if (( ${#MAESTRO_E2E_UPLOAD_FILE_STEM} > 40 )); then
  echo "MAESTRO_E2E_UPLOAD_FILE_STEM must not exceed 40 characters so Files cleanup remains addressable." >&2
  exit 2
fi

if [[ "${MAESTRO_E2E_FILES_READY:-}" != "1" ]]; then
  simulator_fixture_path="/tmp/${MAESTRO_E2E_UPLOAD_FILE_STEM}.md"

  xcrun simctl spawn "${IOS_SIMULATOR_UDID}" /bin/sh -c \
    "/bin/cat > '${simulator_fixture_path}'" <"${fixture_path}"
  automated_fixture_staged="1"
  xcrun simctl openurl "${IOS_SIMULATOR_UDID}" "file://${simulator_fixture_path}"

  echo "Importing the controlled fixture through the public iOS Files save sheet"
  maestro test \
    --udid "${IOS_SIMULATOR_UDID}" \
    --test-output-dir "${artifact_dir}/evidence" \
    --debug-output "${artifact_dir}/maestro-debug/staging" \
    "${mobile_dir}/e2e/maestro/flows/stage-knowledge-upload.yaml"
fi

node "${mobile_dir}/e2e/maestro/setup-knowledge-upload-fixture.mjs"

xcrun simctl io "${IOS_SIMULATOR_UDID}" recordVideo \
  --codec=h264 \
  --force \
  "${recording_path}" \
  >"${artifact_dir}/recording.log" 2>&1 &
recording_pid=$!

if ! kill -0 "${recording_pid}" 2>/dev/null; then
  wait "${recording_pid}" || true
  echo "Failed to start Simulator recording. See ${artifact_dir}/recording.log." >&2
  exit 1
fi

echo "Running Cove native knowledge upload E2E on Simulator ${IOS_SIMULATOR_UDID}"
echo "API boundary: run-owned loopback URL validated by fixture setup"
echo "Artifacts: ${artifact_dir}"

maestro test \
  --udid "${IOS_SIMULATOR_UDID}" \
  --config "${mobile_dir}/e2e/maestro/config.yaml" \
  --test-output-dir "${artifact_dir}/evidence" \
  --debug-output "${artifact_dir}/maestro-debug" \
  --format JUNIT \
  --output "${artifact_dir}/maestro-junit.xml" \
  "${mobile_dir}/e2e/maestro/flows/knowledge-upload.yaml"

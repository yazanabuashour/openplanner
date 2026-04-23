#!/bin/sh
set -eu

repo="yazanabuashour/openplanner"
default_version="__OPENPLANNER_VERSION__"

log() {
  printf '%s\n' "$*"
}

fail() {
  printf 'openplanner install: %s\n' "$*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

detect_os() {
  case "$(uname -s)" in
    Darwin) printf 'darwin' ;;
    Linux) printf 'linux' ;;
    *) fail "unsupported operating system: $(uname -s)" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64 | amd64) printf 'amd64' ;;
    arm64 | aarch64) printf 'arm64' ;;
    *) fail "unsupported CPU architecture: $(uname -m)" ;;
  esac
}

resolve_latest_version() {
  latest_json="$(curl -fsSL "https://api.github.com/repos/${repo}/releases/latest")" ||
    fail "could not resolve latest GitHub Release"
  latest_tag="$(printf '%s\n' "$latest_json" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
  [ -n "$latest_tag" ] || fail "could not read latest release tag"
  printf '%s' "$latest_tag"
}

select_version() {
  requested="${OPENPLANNER_VERSION:-$default_version}"
  case "$requested" in
    "" | "__OPENPLANNER_VERSION__" | latest)
      resolve_latest_version
      ;;
    v*)
      printf '%s' "$requested"
      ;;
    *)
      printf 'v%s' "$requested"
      ;;
  esac
}

download() {
  url="$1"
  output="$2"
  curl -fsSL "$url" -o "$output" || fail "download failed: $url"
}

verify_archive() {
  checksum_file="$1"
  archive="$2"
  expected_line="expected-${archive}.sha256"

  awk -v file="$archive" '$2 == file { print; found = 1 } END { exit found ? 0 : 1 }' "$checksum_file" > "$expected_line" ||
    fail "checksum entry not found for ${archive}"

  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 -c "$expected_line" >/dev/null ||
      fail "checksum verification failed for ${archive}"
    return
  fi

  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -c "$expected_line" >/dev/null ||
      fail "checksum verification failed for ${archive}"
    return
  fi

  fail "missing required command: shasum or sha256sum"
}

first_writable_path_dir() {
  old_ifs="$IFS"
  IFS=:
  for dir in ${PATH:-}; do
    IFS="$old_ifs"
    [ -n "$dir" ] || dir="."
    [ "$dir" = "." ] && continue
    if [ -d "$dir" ] && [ -w "$dir" ]; then
      printf '%s' "$dir"
      return 0
    fi
    IFS=:
  done
  IFS="$old_ifs"
  return 1
}

select_install_dir() {
  if [ -n "${OPENPLANNER_INSTALL_DIR:-}" ]; then
    printf '%s' "$OPENPLANNER_INSTALL_DIR"
    return
  fi

  if dir="$(first_writable_path_dir)"; then
    printf '%s' "$dir"
    return
  fi

  [ -n "${HOME:-}" ] || fail "HOME is not set and no writable PATH directory was found"
  printf '%s/.local/bin' "$HOME"
}

path_contains_dir() {
  needle="$1"
  old_ifs="$IFS"
  IFS=:
  for dir in ${PATH:-}; do
    IFS="$old_ifs"
    [ "$dir" = "$needle" ] && return 0
    IFS=:
  done
  IFS="$old_ifs"
  return 1
}

verify_runner() {
  runner="$1"
  printf '%s\n' '{"action":"validate"}' | "$runner" planning >/dev/null ||
    fail "runner validation failed"
}

need_cmd curl
need_cmd tar

os="$(detect_os)"
arch="$(detect_arch)"
tag="$(select_version)"
asset_version="${tag#v}"
archive="openplanner_${asset_version}_${os}_${arch}.tar.gz"
checksum="openplanner_${asset_version}_checksums.txt"
release_url="https://github.com/${repo}/releases/download/${tag}"
tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/openplanner-install.XXXXXX")"
install_dir="$(select_install_dir)"

cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

log "Installing OpenPlanner ${tag} for ${os}/${arch}"

cd "$tmp_dir"
download "${release_url}/${archive}" "$archive"
download "${release_url}/${checksum}" "$checksum"
verify_archive "$checksum" "$archive"

tar -xzf "$archive"
mkdir -p "$install_dir"
cp "openplanner_${asset_version}_${os}_${arch}/openplanner" "${install_dir}/openplanner"
chmod 755 "${install_dir}/openplanner"

log "Runner installed to ${install_dir}/openplanner"
installed_version="$("${install_dir}/openplanner" --version)"
log "Runner version: ${installed_version}"

active_path="$(command -v openplanner 2>/dev/null || true)"
if path_contains_dir "$install_dir"; then
  [ -n "$active_path" ] || fail "openplanner is not callable even though ${install_dir} is on PATH"
  active_version="$(openplanner --version 2>/dev/null || true)"
  if [ "$active_version" != "$installed_version" ]; then
    log ""
    log "Warning: active openplanner resolves to ${active_path}, not ${install_dir}/openplanner."
    log "Your current shell may still invoke another openplanner binary."
    fail "active openplanner reports ${active_version:-unavailable}; expected ${installed_version}"
  fi
fi

verify_runner "${install_dir}/openplanner"
log "Runner verified with openplanner planning"
log ""
log "To complete OpenPlanner installation, register the OpenPlanner skill with your agent:"
log "  Source: https://github.com/${repo}/tree/${tag}/skills/openplanner"
log "  Archive: ${release_url}/openplanner_${asset_version}_skill.tar.gz"
log "Use your agent's native skill location or installer."
log "Do not report OpenPlanner installed until both the runner and skill are installed."

if ! path_contains_dir "$install_dir"; then
  log ""
  log "Add this directory to PATH before using the skill:"
  log "  export PATH=\"${install_dir}:\$PATH\""
fi

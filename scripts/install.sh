#!/usr/bin/env bash
# Bootstrap installer: clones Dendrite and hands over to `dendrite install`, which clones
# every sub-project next to it and keeps each on its tracked branch.
#
#   curl -sSL https://raw.githubusercontent.com/yasuomaidana/dendrite/main/scripts/install.sh | bash
#   curl -sSL https://raw.githubusercontent.com/yasuomaidana/dendrite/develop/scripts/install.sh | bash -s -- --dev
#
# Options:
#   --dev                 Track develop in every repository (default: main)
#   --root <dir>          Where the repositories live (default: $DENDRITE_ROOT or ~/dendrite)
#   --ref <alias>=<ref>   Track another branch or tag for one sub-project (repeatable)
#   --no-build            Clone and check out only
# Environment:
#   DENDRITE_REPO         Dendrite git URL (default: https://github.com/yasuomaidana/dendrite.git)
set -euo pipefail

repo="${DENDRITE_REPO:-https://github.com/yasuomaidana/dendrite.git}"
root="${DENDRITE_ROOT:-$HOME/dendrite}"
branch="main"
channel="--stable"
args=()

usage() { sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'; }

while [ $# -gt 0 ]; do
	case "$1" in
	--dev) branch="develop"; channel="--dev" ;;
	--root) root="${2:?--root requires a directory}"; shift ;;
	--root=*) root="${1#--root=}" ;;
	--ref) args+=(--ref "${2:?--ref requires <alias>=<ref>}"); shift ;;
	--ref=*) args+=(--ref "${1#--ref=}") ;;
	--no-build) args+=(--no-build) ;;
	-h|--help) usage; exit 0 ;;
	*) echo "[-] unknown option: $1" >&2; usage >&2; exit 2 ;;
	esac
	shift
done

for tool in git go; do
	command -v "$tool" >/dev/null 2>&1 || { echo "[-] $tool is required" >&2; exit 1; }
done
command -v cargo >/dev/null 2>&1 || echo "[!] cargo not found: the Rust sub-projects will be cloned but not built" >&2

mkdir -p "$root"
root="$(cd "$root" && pwd)"
src="$root/Dendrite"
if [ -d "$src/.git" ] || [ -f "$src/.git" ]; then
	echo "[+] Using the Dendrite checkout at $src"
	# Build the installer from the tracked branch when the checkout can move there.
	if [ -z "$(git -C "$src" status --porcelain --untracked-files=no)" ]; then
		git -C "$src" fetch --quiet origin
		git -C "$src" switch --quiet "$branch" 2>/dev/null || git -C "$src" switch --quiet -c "$branch" --track "origin/$branch"
		git -C "$src" merge --quiet --ff-only "origin/$branch"
	fi
else
	echo "[+] Cloning Dendrite ($branch) into $src"
	git clone --quiet --branch "$branch" "$repo" "$src"
fi

cd "$src"
exec go run ./cmd/dendrite install "$channel" --root "$root" ${args[@]+"${args[@]}"}

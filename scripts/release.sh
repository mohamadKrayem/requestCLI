#!/usr/bin/env bash
#
# Build and package every release artifact into dist/.
#
#   VERSION=1.3.0 ./scripts/release.sh      # explicit
#   ./scripts/release.sh                    # derives it from the current tag
#
# For each target this writes dist/rq_<version>_<os>_<arch>.tar.gz (unix) or
# .zip (windows): one top-level directory holding the binary, LICENSE and
# README.md. It then writes dist/SHA256SUMS.
#
# The archives are byte-reproducible: ownership, order and timestamps are all
# pinned, so anyone can rebuild a tag and check the published checksum rather
# than trusting it. SOURCE_DATE_EPOCH overrides the timestamp; it defaults to
# the commit date of HEAD.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODULE="github.com/mohamadkrayem/requestCLI"
DIST="$ROOT/dist"

# Asset names carry a bare version, the git tag carries a leading v. Strip it
# so `VERSION=v1.3.0` and `VERSION=1.3.0` both produce rq_1.3.0_linux_amd64.
VERSION="${VERSION:-$(git -C "$ROOT" describe --tags --exact-match 2>/dev/null || true)}"
VERSION="${VERSION#v}"
if [[ -z "$VERSION" ]]; then
  echo "error: no VERSION given and HEAD is not tagged" >&2
  exit 1
fi

# Reproducible archives need GNU tar's --sort/--owner/--mtime. bsdtar (macOS)
# has none of them, and silently producing a differently-ordered archive would
# defeat the point of publishing checksums at all.
if ! tar --version 2>/dev/null | head -1 | grep -q GNU; then
  echo "error: GNU tar is required (macOS: brew install gnu-tar, then PATH=\$(brew --prefix)/opt/gnu-tar/libexec/gnubin:\$PATH)" >&2
  exit 1
fi

SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-$(git -C "$ROOT" log -1 --pretty=%ct)}"
TOUCH_DATE="@$SOURCE_DATE_EPOCH"

targets=(
  "linux amd64"
  "linux arm64"
  "darwin amd64"
  "darwin arm64"
  "windows amd64"
  "windows arm64"
)

rm -rf "$DIST"
mkdir -p "$DIST"

echo "==> Packaging rq $VERSION"

for target in "${targets[@]}"; do
  read -r os arch <<<"$target"

  stem="rq_${VERSION}_${os}_${arch}"
  stage="$DIST/$stem"
  binary="rq"
  [[ "$os" == "windows" ]] && binary="rq.exe"

  mkdir -p "$stage"

  # -trimpath keeps build paths out of the binary, -s -w drop the symbol and
  # DWARF tables, and -X stamps the version core.Version reports and puts in
  # the default User-Agent.
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build \
    -trimpath \
    -ldflags "-s -w -X ${MODULE}/core.Version=${VERSION}" \
    -o "$stage/$binary" \
    "$ROOT/cmd/rq"

  cp "$ROOT/LICENSE" "$ROOT/README.md" "$stage/"

  # Pin every mtime before archiving, including the directory itself.
  find "$stage" -exec touch --no-dereference --date="$TOUCH_DATE" {} +

  if [[ "$os" == "windows" ]]; then
    # -X drops extra attributes (uid/gid, mtime precision) that would differ
    # between machines. Sorted input keeps the central directory stable.
    ( cd "$DIST" && find "$stem" -print | sort | zip -q -X -9 "$stem.zip" -@ )
  else
    # gzip is invoked separately with -n: tar --gzip would let gzip stamp the
    # current time into its own header, which alone would change the checksum
    # on every rebuild.
    tar --create \
        --sort=name \
        --owner=root:0 --group=root:0 --numeric-owner \
        --mtime="$TOUCH_DATE" \
        --directory "$DIST" \
        "$stem" \
      | gzip -n -9 > "$DIST/$stem.tar.gz"
  fi

  rm -rf "$stage"
  echo "    $stem"
done

# sha256sum's own format, so `sha256sum -c SHA256SUMS` verifies a download
# with no extra tooling.
( cd "$DIST" && sha256sum rq_* | sort -k2 > SHA256SUMS )

echo "==> dist/"
ls -1 "$DIST"

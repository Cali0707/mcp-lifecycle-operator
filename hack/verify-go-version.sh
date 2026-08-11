#!/usr/bin/env bash
# Verifies that the Go version in the Dockerfile is not older than the version
# go.mod asks for.
#
# A newer toolchain builds an older `go` directive, so the builder image running
# ahead of go.mod is fine. The reverse is not: if go.mod asks for a newer Go than
# the image ships, `go build` downloads a toolchain over the network mid-build,
# and fails outright when offline or under GOTOOLCHAIN=local.

set -o errexit
set -o nounset
set -o pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOCKERFILE="${REPO_ROOT}/Dockerfile"
GO_MOD="${REPO_ROOT}/go.mod"

# Every `FROM golang:<version>` in the Dockerfile must agree, otherwise "the"
# image version is ambiguous and the stages would build against different Gos.
mapfile -t image_versions < <(grep -oE '^FROM.* golang:[0-9]+(\.[0-9]+)*' "${DOCKERFILE}" |
	sed -E 's/.*golang://' | sort -u)

if [[ "${#image_versions[@]}" -eq 0 ]]; then
	echo "ERROR: found no 'FROM golang:<version>' lines in ${DOCKERFILE}." >&2
	exit 1
fi

if [[ "${#image_versions[@]}" -gt 1 ]]; then
	echo "ERROR: Dockerfile pins more than one golang version: ${image_versions[*]}" >&2
	echo "       All 'FROM golang:<version>' stages must use the same version." >&2
	exit 1
fi

image_version="${image_versions[0]}"

# `go 1.26.0` -> 1.26.0. Required; a bare `go 1.26` is normalised to 1.26.0 so
# that it compares correctly against a three-component image tag.
go_directive="$(sed -nE 's/^go[[:space:]]+([0-9]+(\.[0-9]+)*).*/\1/p' "${GO_MOD}" | head -n1)"
if [[ -z "${go_directive}" ]]; then
	echo "ERROR: could not find a 'go' directive in ${GO_MOD}." >&2
	exit 1
fi

# `toolchain go1.26.5` -> 1.26.5. Optional.
toolchain="$(sed -nE 's/^toolchain[[:space:]]+go([0-9]+(\.[0-9]+)*).*/\1/p' "${GO_MOD}" | head -n1)"

normalise() {
	local v="$1"
	while [[ "$(tr -dc '.' <<<"${v}" | wc -c)" -lt 2 ]]; do
		v="${v}.0"
	done
	printf '%s' "${v}"
}

# Highest of the two is what go.mod effectively demands of the toolchain.
required="${go_directive}"
required_from="go directive"
if [[ -n "${toolchain}" ]]; then
	highest="$(printf '%s\n%s\n' "$(normalise "${go_directive}")" "$(normalise "${toolchain}")" |
		sort -V | tail -n1)"
	if [[ "${highest}" == "$(normalise "${toolchain}")" && "${toolchain}" != "${go_directive}" ]]; then
		required="${toolchain}"
		required_from="toolchain directive"
	fi
fi

# Ordering holds when the lower of {required, image} is `required`.
lowest="$(printf '%s\n%s\n' "$(normalise "${required}")" "$(normalise "${image_version}")" |
	sort -V | head -n1)"

if [[ "${lowest}" != "$(normalise "${required}")" ]]; then
	cat >&2 <<-EOF
		ERROR: the Dockerfile's Go version is older than go.mod requires.

		  Dockerfile  FROM golang:${image_version}
		  go.mod      ${required} (${required_from})

		Building this image would download a Go toolchain over the network, and
		would fail offline or under GOTOOLCHAIN=local.

		Fix by bumping the 'FROM golang:' stages in Dockerfile to at least
		${required}, or by lowering the go.mod ${required_from}.
	EOF
	exit 1
fi

echo "Dockerfile Go version (${image_version}) satisfies go.mod (${required} from ${required_from})."

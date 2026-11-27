#!/bin/sh
# Copyright (C) 2026 ItsWempo
# SPDX-License-Identifier: AGPL-3.0-or-later
set -eu

: "${R2_ACCOUNT_ID:?R2_ACCOUNT_ID must be configured}"
: "${R2_BUCKET_NAME:?R2_BUCKET_NAME must be configured}"
: "${AWS_ACCESS_KEY_ID:?AWS_ACCESS_KEY_ID must be configured}"
: "${AWS_SECRET_ACCESS_KEY:?AWS_SECRET_ACCESS_KEY must be configured}"

mkdir -p /mnt/r2
/usr/local/bin/tigrisfs \
  --endpoint "https://${R2_ACCOUNT_ID}.r2.cloudflarestorage.com" \
  --region auto \
  -f "${R2_BUCKET_NAME}" /mnt/r2 &
fuse_pid=$!

app_pid=""
cleanup() {
  if [ -n "$app_pid" ]; then
    kill -TERM "$app_pid" 2>/dev/null || true
    wait "$app_pid" 2>/dev/null || true
  fi
  fusermount -u /mnt/r2 2>/dev/null || true
  kill "$fuse_pid" 2>/dev/null || true
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

attempt=0
until mountpoint -q /mnt/r2; do
  if ! kill -0 "$fuse_pid" 2>/dev/null; then
    echo "VERTEXIA storage mount exited before becoming ready" >&2
    exit 1
  fi
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 45 ]; then
    echo "Timed out waiting for the VERTEXIA R2 mount" >&2
    exit 1
  fi
  sleep 1
done

mkdir -p /mnt/r2/uploads/assets /mnt/r2/renders/meshes
if [ -d /app/uploads ] && [ ! -L /app/uploads ]; then
  rmdir /app/uploads 2>/dev/null || {
    echo "/app/uploads must be empty before the R2 mount is linked" >&2
    exit 1
  }
fi
if [ -d /app/static/renders ] && [ ! -L /app/static/renders ]; then
  rmdir /app/static/renders 2>/dev/null || {
    echo "/app/static/renders must be empty before the R2 mount is linked" >&2
    exit 1
  }
fi
ln -sfn /mnt/r2/uploads /app/uploads
ln -sfn /mnt/r2/renders /app/static/renders

echo "VERTEXIA persistent asset storage mounted"
/usr/local/bin/vertexia &
app_pid=$!
set +e
wait "$app_pid"
status=$?
set -e
app_pid=""
exit "$status"

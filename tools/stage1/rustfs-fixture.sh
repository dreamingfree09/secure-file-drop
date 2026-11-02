#!/bin/sh
set -eu
[ "$#" -eq 2 ] && [ "$1" = server ] && [ "$2" = /data ] || exit 2
export RUSTFS_ACCESS_KEY="${MINIO_ROOT_USER:?owned fixture key}"
export RUSTFS_SECRET_KEY="${MINIO_ROOT_PASSWORD:?owned fixture secret}"
export RUSTFS_ADDRESS=0.0.0.0:9000 RUSTFS_CONSOLE_ENABLE=false RUSTFS_OBS_LOG_DIRECTORY=
unset MINIO_ROOT_USER MINIO_ROOT_PASSWORD
exec /usr/bin/rustfs /data

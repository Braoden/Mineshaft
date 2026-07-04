#!/bin/sh
set -e

# Re-apply git/dolt config on every start so env var changes take effect
# even when the home volume already exists from a previous run.
if [ -n "$GIT_USER" ] && [ -n "$GIT_EMAIL" ]; then
    git config --global user.name "$GIT_USER"
    git config --global user.email "$GIT_EMAIL"
    git config --global credential.helper store
    dolt config --global --add user.name "$GIT_USER"
    dolt config --global --add user.email "$GIT_EMAIL"
fi

if [ ! -f /gt/overseer/town.json ]; then
    echo "Initializing Excavation Site workspace at /gt..."
    /app/excavation/gt install /gt --git
else
    echo "Refreshing Excavation Site workspace at /gt..."
    /app/excavation/gt install /gt --git --force
fi

exec "$@"

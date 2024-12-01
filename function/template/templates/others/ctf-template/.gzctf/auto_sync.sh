#!/bin/sh

while true; do
    git fetch origin

    LOCAL=$(git rev-parse @)
    REMOTE=$(git rev-parse @{u})
    BASE=$(git merge-base @ @{u})

    if [ "$LOCAL" = "$REMOTE" ]; then
        echo "Up to date!"
    elif [ "$LOCAL" = "$BASE" ]; then
        git pull
        echo "Updates available. Syncing..."
        (make sync & make start)
    else
        echo "You have local changes. Please push them first."
    fi

    sleep 5

done

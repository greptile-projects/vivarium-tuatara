#!/bin/sh
case "$1" in
  *Username*) printf '%s\n' git ;;
  *Password*) printf '%s\n' "$VIVARIUM_GIT_TOKEN" ;;
  *) exit 1 ;;
esac

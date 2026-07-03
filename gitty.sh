#!/usr/bin/env sh
set -eu

ask_yes_no() {
  prompt=$1
  while true; do
    printf "%s [ja/nein]: " "$prompt"
    read -r answer
    case "$answer" in
      ja|j|Ja|JA|yes|y|Yes|YES) return 0 ;;
      nein|n|Nein|NEIN|no|No|NO) return 1 ;;
      *) printf "Bitte ja oder nein eingeben.\n" ;;
    esac
  done
}

git status -uall --short

if ! ask_yes_no "Weiter?"; then
  exit 0
fi

git add .

git status -uall --short

if ! ask_yes_no "Weiter?"; then
  exit 0
fi

printf "Beschreibung fuer den Commit: "
read -r description

printf "Commit-Beschreibung: %s\n" "$description"
if ! ask_yes_no "Weiter machen?"; then
  exit 0
fi

git commit -m "$description"

git log -1 --oneline --name-status

if ! ask_yes_no "Weiter?"; then
  exit 0
fi

if ask_yes_no "Push?"; then
  git push
fi

#!/bin/bash
# The CODE TREE of a worktree: HEAD's tree with every .go file, go.mod and
# go.sum replaced by the working-tree bytes (tracked and untracked), written
# through a throwaway index. Evidence and docs are excluded on purpose, so the
# hash does not move while captures are being written.
set -e
cd "$1"
T=$(mktemp)
GIT_INDEX_FILE=$T git read-tree HEAD
GIT_INDEX_FILE=$T git add -A -- ':(glob)**/*.go' go.mod go.sum
GIT_INDEX_FILE=$T git write-tree
rm -f "$T"

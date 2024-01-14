# gitscd - Gits Change Directory to selected repository.
gits-cd() {
	cd "$(gits cd "$@")" || echo "Unable to cd"
}

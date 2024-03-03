# cdgit - change directory to selected repository with gits.
cdgit() {
	cd -- "$(gits cd "$@")" || echo "Unable to cd"
}

# cdgit - change directory to selected repository with gits.
function cdgit --wraps='gits cd'
	cd -- "$(gits cd $argv[1])" || echo "Unable to cd"
end

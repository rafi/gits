# gits demo - one command across many git repositories.
#
# Recorded against the throwaway sandbox that examples/demo-fixture.sh builds:
# fake repositories, fake remotes and a config file, all under $DEMO_SANDBOX.
# Run it through `just demo`, which builds the binary, seeds the sandbox and
# removes it afterwards. Running `vhs examples/demo.tape` by hand records
# against whatever $DEMO_SANDBOX points at, and fails when it is unset.

Output examples/demo.gif

Set Shell fish

Require gits
Require git
Require jq

Set Width 920
Set Height 680
Set Framerate 60
Set TypingSpeed 50ms

# Theme and frame
Set Theme "Catppuccin Mocha"
Set BorderRadius 14
Set WindowBar Colorful
Set WindowBarSize 40

Set Margin 20
Set Padding 30
Set MarginFill "#674EFF"

# Typography
Set FontFamily "PragmataPro"
Set FontSize 19
Set LetterSpacing 1
Set LineHeight 1.1

# ============================================
# Setup - everything stays inside the sandbox
# ============================================

Hide
Type@0ms `[ -n "$DEMO_SANDBOX" ] || { echo "DEMO_SANDBOX unset; run: just demo"; exit 1; }`
Enter
Type@0ms `source ~/.config/fish/conf.d/theme.fish`
Enter
Type@0ms `set -U fish_color_comment $fish_color_comment --italics`
Enter
# A known prompt that `Wait /❯/` can match, so every command is waited out
# rather than guessed at with a Sleep.
Type@0ms `function fish_prompt; set_color DA3A9C; echo -n '❯ '; set_color normal; end`
Enter
Type@0ms `export HOME="$DEMO_SANDBOX/home"`
Enter
Type@0ms `export XDG_CONFIG_HOME="$HOME/.config" XDG_CACHE_HOME="$HOME/.cache"`
Enter
Type@0ms `export GIT_CONFIG_GLOBAL="$HOME/.gitconfig" GIT_CONFIG_SYSTEM=/dev/null`
Enter
Type@0ms `export PATH="$DEMO_SANDBOX/bin:$PATH"`
Enter
Type@0ms `cd "$HOME" && clear`
Enter
Show

Type@2ms "# gits - one command across many git repositories"
Enter
Sleep 2s

# ============================================
# Part 1: Status, with no config at all
# ============================================

Enter@800ms
Type@2ms "# Point it at any directory - no config needed"
Enter 2
Sleep 500ms
Type "gits status ~/code/oss"
Sleep 1s
Enter
Wait /❯/
Sleep 3s

Screenshot examples/screenshots/status-path.png

Enter@1500ms

# ============================================
# Part 2: Projects from the config file
# ============================================

Hide Type "clear" Enter Show Sleep 500ms
Type@2ms "# Or group repositories into projects in ~/.gits.yaml"
Enter 2
Sleep 500ms
Type "gits list"
Sleep 1s
Enter
Wait /❯/
Sleep 3s

Screenshot examples/screenshots/list.png

Enter@1500ms

Hide Type "clear" Enter Show Sleep 500ms
Type@2ms "# Sub-projects keep their shape - list them as a tree"
Enter 1
Sleep 500ms
Type "gits list -o tree"
Sleep 1s
Enter
Wait /❯/
Sleep 3s

Screenshot examples/screenshots/list-tree.png

Enter@1500ms

# ============================================
# Part 3: Making a project, in one command
#
# After the two list views on purpose: these runs add projects, so a full
# `gits list` recorded after them would no longer fit the frame.
# ============================================

Hide Type "clear" Enter Show Sleep 500ms
Type@2ms "# Already have clones on disk? One command writes the config"
Enter 2
Sleep 500ms
Type "gits discover ~/work"
Sleep 1s
Enter
Wait /❯/
Sleep 3s

Enter@1500ms

Hide Type "clear" Enter Show Sleep 500ms
Type@2ms "# A project per directory, with every repo and its remote"
Enter 2
Sleep 500ms
Type "gits list backend platform"
Sleep 1s
Enter
Wait /❯/
Sleep 3s

Enter@1500ms

Hide Type "clear" Enter Show Sleep 500ms
Type@2ms "# Or add one repository at a time - the project is created for you"
Enter 2
Sleep 500ms
Type "cd ~/labs/prototype"
Sleep 500ms
Enter
Wait /❯/
Type "gits add labs"
Sleep 1s
Enter
Wait /❯/
Sleep 2500ms

Hide Type `cd "$HOME"` Enter Show Sleep 500ms

Enter@1500ms

# ============================================
# Part 4: Status of a whole project
# ============================================

Hide Type "clear" Enter Show Sleep 500ms
Type@2ms "# One line per repo: branch, changes, upstream, version, commit"
Enter 2
Sleep 500ms
Type "gits status acme"
Sleep 1s
Enter
Wait /❯/
Sleep 4s

Screenshot examples/screenshots/status.png

Enter@1500ms

Hide Type "clear" Enter Show Sleep 500ms
Type@2ms "# Narrow it down: only what has uncommitted work"
Enter 2
Sleep 500ms
Type "gits status acme --dirty --stat"
Sleep 1s
Enter
Wait /❯/
Sleep 3s

Screenshot examples/screenshots/status-dirty.png

Enter@1500ms

# ============================================
# Part 5: Clone what is missing
# ============================================

Hide Type "clear" Enter Show Sleep 500ms
Type@2ms "# Clone the repositories that are not on disk yet"
Enter 2
Sleep 500ms
Type "gits clone acme"
Sleep 1s
Enter
Wait /❯/
Sleep 3s

Screenshot examples/screenshots/clone.png

Enter@1500ms

# ============================================
# Part 6: Fetch and pull, everywhere at once
# ============================================

Hide Type "clear" Enter Show Sleep 500ms
Type@2ms "# Fetch every remote, in parallel"
Enter 2
Sleep 500ms
Type "gits fetch acme"
Sleep 1s
Enter
Wait /❯/
Sleep 2s

Enter@800ms
Type@2ms "# ...then bring them all up to date"
Enter 2
Sleep 500ms
Type "gits pull acme"
Sleep 1s
Enter
Wait /❯/
Sleep 3s

Screenshot examples/screenshots/pull.png

Enter@1500ms

# ============================================
# Part 7: Push, safely
# ============================================

Hide Type "clear" Enter Show Sleep 500ms
Type@2ms "# Push each current branch to its upstream - here as a dry run"
Enter 2
Sleep 500ms
Type "gits push acme --dry-run"
Sleep 1s
Enter
Wait /❯/
Sleep 3s

Screenshot examples/screenshots/push.png

Enter@1500ms

# ============================================
# Part 8: Run anything, everywhere
# ============================================

Hide Type "clear" Enter Show Sleep 500ms
Type@2ms "# Run any command in every repository"
Enter 2
Sleep 500ms
Type `gits exec acme -- git log -1 --format='%h %s'`
Sleep 1s
Enter
Wait /❯/
Sleep 3s

Screenshot examples/screenshots/exec.png

Enter@1500ms

# ============================================
# Part 9: Scripting with JSON
# ============================================

Hide Type "clear" Enter Show Sleep 500ms
Type@2ms "# Every report speaks JSON, for jq and for CI"
Enter 2
Sleep 500ms
Type `gits status acme -o json | jq -r '.acme.repos[].dir'`
Sleep 1s
Enter
Wait /❯/
Sleep 3s

Screenshot examples/screenshots/json.png

Enter@1500ms

# ============================================
# Part 10: Orphans and doctor
# ============================================

Hide Type "clear" Enter Show Sleep 500ms
Type@2ms "# Find repositories on disk that no project declares"
Enter 2
Sleep 500ms
Type "gits orphan acme"
Sleep 1s
Enter
Wait /❯/
Sleep 2500ms

Enter@800ms
Type@2ms "# And check the config and environment before they bite"
Enter 2
Sleep 500ms
Type "gits doctor"
Sleep 1s
Enter
Wait /❯/
Sleep 3s

Screenshot examples/screenshots/doctor.png

Enter@1500ms

# ============================================
# Closing
# ============================================

Hide Type "clear" Enter Show Sleep 500ms
Type@2ms "# gits - github.com/rafi/gits"
Enter
Sleep 500ms
Type@2ms "# Install: brew install rafi/tap/gits"
Enter
Sleep 6s

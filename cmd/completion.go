package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(completionCmd)
}

// completionCmd represents the completion command
// nolint:gochecknoglobals
var completionCmd = &cobra.Command{
	Use:   "completion",
	Short: "Generates bash completion scripts",
	Long: `To load completion run

. <(gits completion)

To configure your bash shell to load completions for each session add to your bashrc

# ~/.bashrc or ~/.profile
. <(gits completion)
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return rootCmd.GenBashCompletion(os.Stdout)
	},
}

const (
	bashCompletionFunc = `__gits_get_projects()
{
    local gits_output out
    if gits_output=$(gits list 2>/dev/null); then
        out=($(echo "${gits_output}" | awk '{print $1}'))
        COMPREPLY=( $( compgen -W "${out[*]}" -- "$cur" ) )
    fi
    if [[ $? -eq 0 ]]; then
        return 0
    fi
}

__gits_custom_func() {
    case ${last_command} in
        gits_checkout | gits_clone | gits_fetch | gits_status | gits_list)
            __gits_get_projects
            return
            ;;
        *)
            ;;
    esac
}
`
)

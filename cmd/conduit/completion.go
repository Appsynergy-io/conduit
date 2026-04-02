package main

import (
	"os"

	"github.com/spf13/cobra"
)

func completionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion script",
		Long: `Generate a shell completion script for conduit.

To load completions:

  bash:
    source <(conduit completion bash)

    # To load completions for each session, execute once:
    # Linux:
    conduit completion bash > /etc/bash_completion.d/conduit
    # macOS:
    conduit completion bash > $(brew --prefix)/etc/bash_completion.d/conduit

  zsh:
    # If shell completion is not already enabled, enable it:
    echo "autoload -U compinit; compinit" >> ~/.zshrc

    # To load completions for each session, execute once:
    conduit completion zsh > "${fpath[1]}/_conduit"

    # You may need to start a new shell for this to take effect.

  fish:
    conduit completion fish | source

    # To load completions for each session, execute once:
    conduit completion fish > ~/.config/fish/completions/conduit.fish

  powershell:
    conduit completion powershell | Out-String | Invoke-Expression

    # To load completions for each session, add to your profile:
    conduit completion powershell >> $PROFILE
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletionV2(os.Stdout, true)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			}
			return nil
		},
	}

	return cmd
}

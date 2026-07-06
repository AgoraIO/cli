package cli

import "github.com/spf13/cobra"

type playgroundOptions struct {
	channel  string
	port     int
	uid      int64
	agentUID int64
	ttl      string
	noOpen   bool
}

func (a *App) buildConvoaiCommand() *cobra.Command {
	group := &cobra.Command{
		Use:   "convoai",
		Short: "Conversational AI developer tools",
		Long:  "Commands for building and testing Agora Conversational AI agents.",
	}
	group.AddCommand(a.buildConvoaiPlaygroundCommand())
	return group
}

func (a *App) buildConvoaiPlaygroundCommand() *cobra.Command {
	opts := &playgroundOptions{}
	cmd := &cobra.Command{
		Use:   "playground",
		Short: "Start a local frontend to talk to your Conversational AI agent",
		Long: `Serve a local web frontend that joins an RTC channel so you can talk to a
Conversational AI agent you start separately (e.g. a Python script on the
Agora Agents SDK). Point your agent at the same App ID and --channel.`,
		Example: example(`
  agora convoai playground --channel my-dev-room
  agora convoai playground --channel my-dev-room --port 8787 --no-open
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runConvoaiPlayground(cmd, opts)
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.channel, "channel", "", "RTC channel to join (required; use the same value for your agent)")
	f.IntVar(&opts.port, "port", 8787, "local server port (auto-increments if busy unless explicitly set)")
	f.Int64Var(&opts.uid, "uid", 0, "browser RTC uid (0 = generate a non-zero uid)")
	f.Int64Var(&opts.agentUID, "agent-uid", 0, "reserved agent uid printed in the wire block (0 = generate)")
	f.StringVar(&opts.ttl, "ttl", "1h", "token lifetime")
	f.BoolVar(&opts.noOpen, "no-open", false, "do not open the browser automatically")
	_ = cmd.MarkFlagRequired("channel")
	return cmd
}

// runConvoaiPlayground is completed in later tasks. Temporary stub so the
// tree registers and tests compile.
func (a *App) runConvoaiPlayground(cmd *cobra.Command, opts *playgroundOptions) error {
	return nil
}

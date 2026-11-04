package cli

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/happyhackingspace/jev"
	"github.com/happyhackingspace/jev/internal/mcpserver"
)

func (a *app) mcpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve Jev as MCP tools over stdio",
		Long: `mcp runs a Model Context Protocol server on stdin and stdout with two
tools: jev_ask, which asks any mix of typed questions about a state, and
jev_models. Register it with an MCP client, for example:

  claude mcp add jev -- jev mcp`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s := mcpserver.New(jev.Version, func() (*jev.Client, error) { return a.client() })
			return s.Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
}

package cli

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/governedtools"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite"
)

func newGovernedToolsCommand(ctx *commandContext) *cobra.Command {
	var workspace, encodedPolicy, dataDir, sessionID string
	cmd := &cobra.Command{Use: "governed-tools", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		raw, err := base64.RawURLEncoding.DecodeString(encodedPolicy)
		if err != nil {
			return fmt.Errorf("decode governed policy: %w", err)
		}
		var policy domain.AttemptExecutionPolicy
		if err := json.Unmarshal(raw, &policy); err != nil {
			return fmt.Errorf("decode governed policy: %w", err)
		}
		store, err := sqlite.Open(dataDir)
		if err != nil {
			return fmt.Errorf("open governed capability store: %w", err)
		}
		defer store.Close()
		return (governedtools.Server{
			Policy: policy, WorkspaceRoot: workspace, SessionID: domain.SessionID(sessionID),
			In: ctx.deps.In, Out: ctx.deps.Out, Escalations: store,
		}).Serve(cmd.Context())
	}}
	cmd.Flags().StringVar(&workspace, "workspace", "", "leased workspace root")
	cmd.Flags().StringVar(&encodedPolicy, "policy", "", "base64url frozen execution policy")
	cmd.Flags().StringVar(&dataDir, "data-dir", "", "Kennel application data directory")
	cmd.Flags().StringVar(&sessionID, "session", "", "governed session identity")
	_ = cmd.MarkFlagRequired("workspace")
	_ = cmd.MarkFlagRequired("policy")
	_ = cmd.MarkFlagRequired("data-dir")
	_ = cmd.MarkFlagRequired("session")
	return cmd
}

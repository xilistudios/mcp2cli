// oauthMaintenance implements --oauth-reset and --oauth-status. Both work
// without --oauth so a user can clear or inspect credentials for a server they
// authenticated to earlier.
package cli

import (
	"fmt"
	"time"

	"github.com/xilistudios/mcp2cli/internal/cache"
	"github.com/xilistudios/mcp2cli/internal/oauth"
	"github.com/xilistudios/mcp2cli/internal/util"
)

func oauthMaintenance(g *GlobalFlags, source, srcHash string) error {
	store := oauth.NewSecretStore(srcHash, cache.OAuthDir(srcHash), cache.LegacyOAuthDir(srcHash))

	if g.OAuthReset {
		if err := store.Forget(); err != nil {
			return err
		}
		if !g.JSONOutput {
			fmt.Fprintf(util.Err, "Cleared stored OAuth credentials for %s\n", source)
		}
		if !g.OAuthStatus {
			return nil
		}
	}

	if g.OAuthStatus {
		st := store.Status()
		if g.JSONOutput {
			util.OutputResult(st, util.OutputOptions{JSONOutput: true, Pretty: g.Pretty})
			return nil
		}
		return printOAuthStatus(source, st)
	}
	return nil
}

func printOAuthStatus(source string, st oauth.TokenStatus) error {
	fmt.Fprintf(util.Out, "OAuth credentials for %s\n", source)
	switch {
	case !st.HasToken:
		fmt.Fprintln(util.Out, "  token:      none stored (the next --oauth call will ask you to log in)")
	case st.Expired && st.HasRefresh:
		fmt.Fprintf(util.Out, "  token:      expired %s, refresh token stored (will renew silently)\n", st.ExpiresAt.Local().Format(time.RFC3339))
	case st.Expired:
		fmt.Fprintf(util.Out, "  token:      expired %s and no refresh token (will ask you to log in)\n", st.ExpiresAt.Local().Format(time.RFC3339))
	default:
		if st.ExpiresAt.IsZero() {
			fmt.Fprintln(util.Out, "  token:      stored, no expiry advertised")
		} else {
			fmt.Fprintf(util.Out, "  token:      valid until %s\n", st.ExpiresAt.Local().Format(time.RFC3339))
		}
	}
	if st.Scope != "" {
		fmt.Fprintf(util.Out, "  scope:      %s\n", st.Scope)
	}
	if st.ClientID != "" {
		fmt.Fprintf(util.Out, "  client_id:  %s (%s)\n", st.ClientID, describeLocation(st.ClientLocation))
	} else {
		fmt.Fprintln(util.Out, "  client_id:  none stored")
	}
	fmt.Fprintf(util.Out, "  stored in:  %s\n", describeLocation(st.TokenLocation))
	if st.KeyringService != "" {
		fmt.Fprintf(util.Out, "              keyring service=%q account=%q\n", st.KeyringService, st.KeyringAccount)
	}
	fmt.Fprintf(util.Out, "              fallback file=%s\n", st.FileDir)
	if st.Error != "" {
		fmt.Fprintf(util.Err, "  warning:    %s\n", st.Error)
	}
	return nil
}

func describeLocation(loc string) string {
	switch loc {
	case oauth.LocKeyring:
		return "OS keyring"
	case oauth.LocFile:
		return "config file (0600)"
	case oauth.LocLegacy:
		return "cache file (0600, will be migrated)"
	default:
		return "not stored"
	}
}

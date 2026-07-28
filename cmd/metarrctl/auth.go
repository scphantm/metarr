package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"Metarr/internal/cliclient"
)

var (
	loginUsername string
	loginPassword string
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in as the admin user",
	Long:  "Compares the submitted username/password against the stored admin credentials and issues a session API key carrying admin rights. The key is saved locally so subsequent commands don't need --api-key.",
	RunE: func(cmd *cobra.Command, args []string) error {
		username := loginUsername
		if username == "" {
			fmt.Print("Username: ")
			if _, err := fmt.Scanln(&username); err != nil {
				return fmt.Errorf("reading username: %w", err)
			}
		}

		password := loginPassword
		if password == "" {
			fmt.Print("Password: ")
			passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println()
			if err != nil {
				return fmt.Errorf("reading password: %w", err)
			}
			password = string(passwordBytes)
		}

		resp, err := newPublicClient().Login(cmd.Context(), username, password)
		if err != nil {
			return err
		}

		if err := cliclient.SaveCredentials(serverURL, resp.APIKey); err != nil {
			return fmt.Errorf("login succeeded but failed to save credentials: %w", err)
		}

		return printJSON(resp)
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out",
	Long:  "Revokes the session API key that authenticated this request and clears it from local storage.",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}

		resp, err := client.Logout(cmd.Context())
		if err != nil {
			return err
		}

		if err := cliclient.ClearCredentials(serverURL); err != nil {
			return fmt.Errorf("logged out but failed to clear local credentials: %w", err)
		}

		return printJSON(resp)
	},
}

func init() {
	loginCmd.Flags().StringVar(&loginUsername, "username", "", "Admin username (prompted if omitted)")
	loginCmd.Flags().StringVar(&loginPassword, "password", "", "Admin password (prompted, masked, if omitted)")
}

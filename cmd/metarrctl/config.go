package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"Metarr/internal/appconfig"
	"Metarr/internal/handlers"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage the application config",
}

var configGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Fetch the application config",
	Long:  "Reads the singleton application config document from MongoDB.",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		resp, err := client.GetConfig(cmd.Context())
		if err != nil {
			return err
		}
		return printJSON(resp)
	},
}

var configSetFile string

var configSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Replace the whole application config",
	Long:  `Fires a system_config_update event with the given document as its payload. Reads the new document as JSON from --file, or stdin if --file is omitted or "-".`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var config appconfig.Config
		if err := readJSONInput(configSetFile, &config); err != nil {
			return err
		}

		client, err := newClient()
		if err != nil {
			return err
		}
		resp, err := client.SetConfig(cmd.Context(), config)
		if err != nil {
			return err
		}
		return printJSON(resp)
	},
}

var (
	setAdminUsername string
	setAdminEmail    string
	setAdminPassword string
)

var configSetAdminCmd = &cobra.Command{
	Use:   "set-admin",
	Short: "Update the admin user's credentials",
	Long:  "Updates any subset of the admin user's username, email, and password. If password is set, it is re-hashed with a fresh salt.",
	RunE: func(cmd *cobra.Command, args []string) error {
		req := handlers.UpdateAdminRequest{}
		if cmd.Flags().Changed("username") {
			req.Username = &setAdminUsername
		}
		if cmd.Flags().Changed("email") {
			req.Email = &setAdminEmail
		}
		if cmd.Flags().Changed("password") {
			req.Password = &setAdminPassword
		}

		client, err := newClient()
		if err != nil {
			return err
		}
		resp, err := client.UpdateAdmin(cmd.Context(), req)
		if err != nil {
			return err
		}
		return printJSON(resp)
	},
}

var configInterfacesCmd = &cobra.Command{
	Use:   "interfaces",
	Short: "Manage interface instances",
}

var configInterfacesSonarrCmd = &cobra.Command{
	Use:   "sonarr",
	Short: "Manage Sonarr interface instances",
}

var sonarrListCmd = &cobra.Command{
	Use:   "list",
	Short: "List Sonarr interface instances",
	Long:  "Reads the application config from MongoDB and returns every configured Sonarr instance.",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		resp, err := client.ListSonarrInterfaces(cmd.Context())
		if err != nil {
			return err
		}
		return printJSON(resp)
	},
}

var sonarrGetCmd = &cobra.Command{
	Use:   "get <slug>",
	Short: "Fetch a single Sonarr interface instance",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		resp, err := client.GetSonarrInterface(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		return printJSON(resp)
	},
}

var sonarrCreateFile string

var sonarrCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a Sonarr interface instance",
	Long:  `Adds a new Sonarr instance. instance_slug is required and must be unique across every interface type. Reads the instance as JSON from --file, or stdin if --file is omitted or "-".`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var instance appconfig.SonarrInstance
		if err := readJSONInput(sonarrCreateFile, &instance); err != nil {
			return err
		}

		client, err := newClient()
		if err != nil {
			return err
		}
		resp, err := client.CreateSonarrInterface(cmd.Context(), instance)
		if err != nil {
			return err
		}
		return printJSON(resp)
	},
}

var sonarrUpdateFile string

var sonarrUpdateCmd = &cobra.Command{
	Use:   "update <slug>",
	Short: "Update a Sonarr interface instance",
	Long:  `Replaces every field of the Sonarr instance at the given instance_slug except instance_slug itself, which cannot be changed once set. Reads the instance as JSON from --file, or stdin if --file is omitted or "-".`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var instance appconfig.SonarrInstance
		if err := readJSONInput(sonarrUpdateFile, &instance); err != nil {
			return err
		}

		client, err := newClient()
		if err != nil {
			return err
		}
		resp, err := client.UpdateSonarrInterface(cmd.Context(), args[0], instance)
		if err != nil {
			return err
		}
		return printJSON(resp)
	},
}

var sonarrDeleteCmd = &cobra.Command{
	Use:   "delete <slug>",
	Short: "Delete a Sonarr interface instance",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		resp, err := client.DeleteSonarrInterface(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		return printJSON(resp)
	},
}

// readJSONInput reads JSON from path (or stdin if path is "" or "-") and
// decodes it into out.
func readJSONInput(path string, out any) error {
	var reader io.Reader
	if path == "" || path == "-" {
		reader = os.Stdin
	} else {
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("opening %s: %w", path, err)
		}
		defer f.Close()
		reader = f
	}

	if err := json.NewDecoder(reader).Decode(out); err != nil {
		return fmt.Errorf("decoding JSON input: %w", err)
	}
	return nil
}

func init() {
	configCmd.AddCommand(configGetCmd, configSetCmd, configSetAdminCmd, configInterfacesCmd)
	configInterfacesCmd.AddCommand(configInterfacesSonarrCmd)
	configInterfacesSonarrCmd.AddCommand(sonarrListCmd, sonarrGetCmd, sonarrCreateCmd, sonarrUpdateCmd, sonarrDeleteCmd)

	configSetCmd.Flags().StringVar(&configSetFile, "file", "", `JSON file to read (default: stdin, or pass "-")`)
	configSetAdminCmd.Flags().StringVar(&setAdminUsername, "username", "", "New username")
	configSetAdminCmd.Flags().StringVar(&setAdminEmail, "email", "", "New email address")
	configSetAdminCmd.Flags().StringVar(&setAdminPassword, "password", "", "New password")
	sonarrCreateCmd.Flags().StringVar(&sonarrCreateFile, "file", "", `JSON file to read (default: stdin, or pass "-")`)
	sonarrUpdateCmd.Flags().StringVar(&sonarrUpdateFile, "file", "", `JSON file to read (default: stdin, or pass "-")`)
}

package main

import "github.com/spf13/cobra"

var heartbeatCmd = &cobra.Command{
	Use:   "heartbeat",
	Short: "Blocking heartbeat check",
	Long:  "Publishes a heartbeat request on the Redis Pub/Sub queue and blocks until the heartbeat listener replies with the current time and the request's correlation ID.",
	RunE: func(cmd *cobra.Command, args []string) error {
		resp, err := newPublicClient().Heartbeat(cmd.Context())
		if err != nil {
			return err
		}
		return printJSON(resp)
	},
}

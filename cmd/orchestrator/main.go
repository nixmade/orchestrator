package main

import (
	"log"
	"os"

	"github.com/nixmade/orchestrator/core"
	"github.com/nixmade/orchestrator/server"
	"github.com/urfave/cli/v2"
)

func main() {
	appCli := &cli.App{
		Name:  "orchestrator",
		Usage: "starts orchestrator server",
		Action: func(c *cli.Context) error {
			return server.Execute(core.NewApp())
		},
	}

	err := appCli.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

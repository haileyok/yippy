package main

import (
	"os"

	"github.com/haileyok/yippy/yippy"
	_ "github.com/joho/godotenv/autoload"
	"github.com/urfave/cli/v2"
)

func main() {
	app := cli.App{
		Name: "yippy",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "addr",
				Required: true,
				EnvVars:  []string{"YIPPY_ADDR"},
			},
			&cli.StringFlag{
				Name:     "files-root",
				Required: true,
				EnvVars:  []string{"YIPPY_FILES_ROOT"},
			},
			&cli.StringFlag{
				Name:     "session-secret",
				Required: true,
				EnvVars:  []string{"YIPPY_SESSION_SECRET"},
			},
			&cli.StringFlag{
				Name:     "password",
				Required: true,
				EnvVars:  []string{"YIPPY_PASSWORD"},
			},
		},
		Action: run,
	}

	app.Run(os.Args)
}

var run = func(cmd *cli.Context) error {
	args := &yippy.Args{
		Addr:          cmd.String("addr"),
		FilesRoot:     cmd.String("files-root"),
		SessionSecret: cmd.String("session-secret"),
		Password:      cmd.String("password"),
	}

	y := yippy.NewYippy(args)

	if err := y.Start(); err != nil {
		return err
	}

	return nil
}

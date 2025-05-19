package yippy

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"

	"github.com/labstack/echo/v4"
)

func (y *Yippy) handleSubs(e echo.Context) error {
	fileName, err := url.QueryUnescape(e.FormValue("file"))
	if err != nil {
		y.logger.Warn("couldn't unescape input file name")
		return echo.NewHTTPError(http.StatusInternalServerError)
	}

	itp, err := isTraversalPath(fileName)
	if err != nil || itp {
		y.logger.Warn("attempted to use directory traversal")
		return echo.NewHTTPError(http.StatusBadRequest)
	}

	filePath := y.filesRoot + "/" + fileName
	ip := e.RealIP()

	y.logger.Info("getting subs", "ip", ip, "path", filePath)

	_, err = os.Stat(filePath)
	if os.IsNotExist(err) {
		return echo.NewHTTPError(http.StatusNotFound)
	}

	cmd := exec.Command(
		"ffmpeg",
		"-i", filePath,
		"-f", "webvtt",
		"pipe:1",
	)

	out, err := cmd.StdoutPipe()
	if err != nil {
		y.logger.Warn("error starting vtt transcode", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError)
	}

	if err := cmd.Start(); err != nil {
		y.logger.Warn("error starting vtt transcode", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError)
	}

	b, err := io.ReadAll(out)
	if err != nil {
		y.logger.Warn("error reading output for vtt transcode", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError)
	}

	if err := cmd.Wait(); err != nil {
		y.logger.Warn("error waiting for command", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError)
	}

	return e.Stream(200, "text/vtt", bytes.NewReader(b))
}

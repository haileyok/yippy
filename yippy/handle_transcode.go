package yippy

import (
	"net/http"
	"net/url"
	"os"

	"github.com/labstack/echo/v4"
)

func (y *Yippy) handleTranscode(c echo.Context) error {
	fileName, err := url.QueryUnescape(c.FormValue("file"))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError)
	}
	if ok, err := isTraversalPath(fileName); err != nil || ok {
		return echo.NewHTTPError(http.StatusBadRequest)
	}

	path := y.filesRoot + "/" + fileName
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return echo.NewHTTPError(http.StatusNotFound)
	}

	c.Response().Header().Set("Content-Type", "video/mp4")

	flusher, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError)
	}

	notify := make(chan int)
	session, err := y.sessionManager.StartSession(c.RealIP(), path, notify)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError)
	}
	defer y.sessionManager.StopSession(session.ID)

	sent := 0
	for {
		select {
		case count, ok := <-notify:
			if !ok || count < 0 {
				return nil
			}
			for ; sent < count; sent++ {
				data := session.Buffer.GetChunk(sent)
				if _, err := c.Response().Writer.Write(data); err != nil {
					return echo.NewHTTPError(http.StatusInternalServerError)
				}
				flusher.Flush()
			}
			if session.Buffer.IsFinished() && sent >= session.Buffer.Len() {
				return nil
			}
		case <-c.Request().Context().Done():
			return nil
		}
	}
}

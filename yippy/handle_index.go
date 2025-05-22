package yippy

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
)

var traversalPatterns = []string{
	"../",
	"..\\",
}

func isTraversalPath(path string) (bool, error) {
	path, err := url.QueryUnescape(path)
	if err != nil {
		return true, err
	}

	for _, p := range traversalPatterns {
		if strings.Contains(path, p) {
			return true, nil
		}
	}

	return false, nil
}

func (y *Yippy) handleIndex(e echo.Context) error {
	currPath := e.Request().URL.Path
	currPathPts := strings.Split(currPath, "/")
	if currPath != "/" {
		currPath += "/"
	}
	sysPath := y.filesRoot + e.Request().URL.Path

	itp, err := isTraversalPath(sysPath)
	if err != nil || itp {
		return echo.NewHTTPError(400)
	}

	files, err := os.ReadDir(sysPath)

	html := fmt.Sprintf("<h3>%s</h3>", currPath)

	if currPath != "/" {
		upOne := "/"
		if len(currPathPts) >= 3 {
			upOne = strings.Join(currPathPts[0:len(currPathPts)-1], "/")
		}
		html += fmt.Sprintf(`D: <a href="%s">..</a><br />`, upOne)
	}

	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		html += fmt.Sprintf(`D: <a href="%s">%s</a><br />`, currPath+f.Name(), f.Name())
	}

	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if !strings.HasSuffix(f.Name(), ".mkv") && !strings.HasSuffix(f.Name(), ".mp4") {
			continue
		}
		escaped := url.QueryEscape(currPath + f.Name())
		html += fmt.Sprintf(`F: <a href="/transcode?file=%s">%s</a> - <a href="/subtitles?file=%s">Subs</a><br />`, escaped, f.Name(), escaped)
	}

	return e.HTML(200, html)
}

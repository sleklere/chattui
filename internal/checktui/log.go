package checktui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Only known event names leave the disposable client directory. No raw log
// line, field, username, password, URL, request, or terminal input is copied.
var clientEvents = []string{
	"authenticated", "websocket connected", "websocket connected at app level",
	"websocket closed", "error reconnecting to websocket", "ws read error",
	"ws write error", "room selected", "left room", "ws received", "ws sent",
}

func saveClientLog(resultsDir, name, clientDir string) (err error) {
	file, err := os.Open(filepath.Join(clientDir, "client.log"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	counts := make(map[string]int)
	scanner := bufio.NewScanner(io.LimitReader(file, 1<<20))
	for scanner.Scan() {
		line := scanner.Text()
		for _, event := range clientEvents {
			if strings.Contains(line, "msg="+event+" ") || strings.HasSuffix(line, "msg="+event) || strings.Contains(line, "msg=\""+event+"\"") {
				counts[event]++
				break
			}
		}
	}
	if err = scanner.Err(); err != nil {
		return err
	}
	var out strings.Builder
	for _, event := range clientEvents {
		if counts[event] > 0 {
			fmt.Fprintf(&out, "%s: %d\n", event, counts[event])
		}
	}
	return os.WriteFile(filepath.Join(resultsDir, name+".log"), []byte(out.String()), 0600)
}

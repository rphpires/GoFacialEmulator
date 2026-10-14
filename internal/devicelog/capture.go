package devicelog

import "time"

type connInfo struct {
	opened time.Time
	reqs   int
}

type streamInfo struct {
	opened time.Time
	path   string
	events int
}
